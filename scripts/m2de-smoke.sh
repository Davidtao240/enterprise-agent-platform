#!/usr/bin/env bash
# M2-C 收尾 + M2-D + M2-E Smoke: 超时扫描 / 熔断 / DLQ / CredentialRef / 全链 Trace
# 前置:docker compose 全栈运行,go-backend 已重建(含 migration 019/020 与 TOOL_* 环境变量)
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
TOKEN="${INTERNAL_SERVICE_TOKEN:-dev-internal-token}"
TENANT="00000000-0000-0000-0000-000000000010" # seed finance tenant
PASS=0; FAIL=0

check() { # name expected actual_substring
  if [[ "$2" == *"$3"* ]]; then
    PASS=$((PASS+1)); echo "PASS: $1"
  else
    FAIL=$((FAIL+1)); echo "FAIL: $1 → $2"
  fi
}

check_code() { # name expected actual
  if [[ "$2" == "$3" ]]; then
    PASS=$((PASS+1)); echo "PASS: $1"
  else
    FAIL=$((FAIL+1)); echo "FAIL: $1 → $2 (expect $3)"
  fi
}

PSQL() { # sql
  docker compose exec -T postgres psql -U platform -d enterprise_agent_platform -tA -c "$1"
}

new_call() { # idem_suffix [extra_json]
  local extra="${2:-}"
  curl -s -X POST "$BASE/internal/v1/tool-calls" \
    -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
    -d "{\"tool_id\":\"parse_csv\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"data_extract_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-$1\"$extra}"
}

confirm() { # tool_call_id status [verification_json]
  local vjson="${3:-}"
  curl -s -X POST "$BASE/internal/v1/tool-calls/$1/confirm" \
    -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' \
    -d "{\"status\":\"$2\"${vjson:+,\"verification_json\":\"$vjson\"}}" > /dev/null
}

# failed 为终态:验证证据(executed=false)须随 confirm 内联提交,事后 /verify 会被拒
NOT_EXECUTED_EVIDENCE='{\"executed\":false,\"source\":\"smoke\"}'

STAMP="$(date +%s)"
IDEM="smoke-m2de-$STAMP"
RUN_ID="$(uuidgen | tr 'A-Z' 'a-z')"

echo "══ A. M2-C.8 timeout_at 超时扫描器 ══"
RES=$(new_call "tmo")
check "low-risk → executing" "$RES" '"status":"executing"'
TC_TMO=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
# 回拨 timeout_at 至过去,模拟执行超时(扫描周期默认 15s)
PSQL "UPDATE tool_calls SET timeout_at = now() - interval '10 seconds' WHERE id = '$TC_TMO'" > /dev/null
GOT=""
for i in $(seq 1 14); do
  sleep 3
  RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC_TMO" -H "X-Internal-Service-Token: $TOKEN")
  if [[ "$RES" == *'"status":"indeterminate"'* ]]; then GOT="$RES"; break; fi
done
check "scanner: executing → indeterminate" "${GOT:-TIMEOUT_WAITING}" '"status":"indeterminate"'
check "scanner: error TOOL_TIMEOUT" "${GOT:-}" 'TOOL_TIMEOUT'

echo "══ B. M2-C.10 DLQ 死信队列 ══"
# 重置 parse_csv 熔断计数,保证 B 段 4 次失败不影响 C 段(4<5 不会 open,但残留计数可能叠加)
PSQL "DELETE FROM tool_circuit_breakers WHERE tool_id = 'parse_csv'" > /dev/null
RES=$(new_call "dlq")
TC_DLQ=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
# 3 轮重试(retry_count 1→3),每轮 failed+证据(未执行) → retry 放行
for i in 1 2 3; do
  confirm "$TC_DLQ" failed "$NOT_EXECUTED_EVIDENCE"
  RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls/$TC_DLQ/retry" -H "X-Internal-Service-Token: $TOKEN")
  check "retry #$i → executing" "$RES" '"status":"executing"'
done
# 第 4 次重试:retry_count(3) ≥ max_retry(3) → 死信
confirm "$TC_DLQ" failed "$NOT_EXECUTED_EVIDENCE"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls/$TC_DLQ/retry" -H "X-Internal-Service-Token: $TOKEN")
check_code "retry over max → 409" "$CODE" '409'
RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC_DLQ" -H "X-Internal-Service-Token: $TOKEN")
check "dead letter flagged" "$RES" '"is_dead_letter":true'
check "dead letter error code" "$RES" 'TOOL_RETRY_EXHAUSTED'
RES=$(curl -s "$BASE/internal/v1/tool-calls/dead-letters" -H "X-Internal-Service-Token: $TOKEN")
check "DLQ list contains call" "$RES" "$TC_DLQ"

echo "══ C. M2-D CredentialRef 边界 ══"
SECRET_PT="smoke-secret-$STAMP"
RES=$(curl -s -X POST "$BASE/internal/v1/connector-bindings" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tenant_id\":\"$TENANT\",\"business_app_code\":\"finance\",\"connector_code\":\"mock_bank\",\"name\":\"m2de smoke\",\"config_json\":\"{\\\"base_url\\\":\\\"https://api.mock-bank.example\\\"}\",\"credential_plaintext\":\"$SECRET_PT\"}")
check "binding created w/ secret ref" "$RES" '"credential_ref":"secret:'
if [[ "$RES" == *"$SECRET_PT"* ]]; then
  FAIL=$((FAIL+1)); echo "FAIL: binding response leaks plaintext → $RES"
else
  PASS=$((PASS+1)); echo "PASS: binding response不含明文"
fi
BINDING=$(echo "$RES" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
CRED_REF=$(echo "$RES" | sed -n 's/.*"credential_ref":"\([^"]*\)".*/\1/p')

RES=$(curl -s "$BASE/internal/v1/connector-bindings/$BINDING" -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT")
check "GET binding: ref visible" "$RES" "\"$CRED_REF\""
if [[ "$RES" == *"$SECRET_PT"* ]]; then
  FAIL=$((FAIL+1)); echo "FAIL: GET binding leaks plaintext"
else
  PASS=$((PASS+1)); echo "PASS: GET binding 不含明文"
fi

# 密文落库校验:cipher_text 不含明文
SECRET_ID="${CRED_REF#secret:}"
CIPHER=$(PSQL "SELECT cipher_text FROM credential_secrets WHERE id = '$SECRET_ID'")
if [[ -n "$CIPHER" && "$CIPHER" != *"$SECRET_PT"* ]]; then
  PASS=$((PASS+1)); echo "PASS: DB 仅存密文(AES-256-GCM)"
else
  FAIL=$((FAIL+1)); echo "FAIL: cipher_text 缺失或含明文 → $CIPHER"
fi

# 执行瞬间解析:仅执行器(InternalServiceToken + 正确租户)可见明文
RES=$(curl -s -X POST "$BASE/internal/v1/connector-bindings/$BINDING/resolve-credential" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT")
check "resolve returns plaintext (executor-only)" "$RES" "$SECRET_PT"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/connector-bindings/$BINDING/resolve-credential" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: 00000000-0000-0000-0000-000000009999")
check_code "cross-tenant resolve → 404" "$CODE" '404'

# Execute 接线 binding:active → executing;disabled → 409;不存在 → 404
RES=$(new_call "cb" ",\"connector_binding_id\":\"$BINDING\"")
check "execute w/ active binding → executing" "$RES" '"status":"executing"'
RES=$(new_call "cb-miss" ",\"connector_binding_id\":\"$(uuidgen | tr 'A-Z' 'a-z')\"")
if [[ "$RES" == *'CONNECTOR_BINDING_NOT_FOUND'* ]]; then
  PASS=$((PASS+1)); echo "PASS: unknown binding → 404"
else
  FAIL=$((FAIL+1)); echo "FAIL: unknown binding → $RES"
fi
PSQL "UPDATE connector_bindings SET status = 'disabled' WHERE id = '$BINDING'" > /dev/null
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"parse_csv\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"data_extract_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-cb-dis\",\"connector_binding_id\":\"$BINDING\"}")
check_code "disabled binding → 409" "$CODE" '409'
PSQL "UPDATE connector_bindings SET status = 'active' WHERE id = '$BINDING'" > /dev/null

echo "══ D. M2-E 全链 Trace ══"
TRACE="smoke-trace-$STAMP"
RES=$(new_call "tr1" ",\"trace_id\":\"$TRACE\"")
TC_TR1=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
RES=$(new_call "tr2" ",\"trace_id\":\"$TRACE\"")
TC_TR2=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
confirm "$TC_TR1" succeeded
confirm "$TC_TR2" succeeded
RES=$(curl -s "$BASE/internal/v1/tool-calls/by-trace/$TRACE" -H "X-Internal-Service-Token: $TOKEN")
check "by-trace returns call #1" "$RES" "$TC_TR1"
check "by-trace returns call #2" "$RES" "$TC_TR2"
AUDIT_N=$(PSQL "SELECT count(*) FROM audit_logs WHERE trace_id = '$TRACE'")
if [[ "${AUDIT_N:-0}" -ge 3 ]]; then
  PASS=$((PASS+1)); echo "PASS: audit 串联 trace (n=$AUDIT_N)"
else
  FAIL=$((FAIL+1)); echo "FAIL: audit trace 记录不足 (n=$AUDIT_N)"
fi

echo "══ E. M2-C.9 熔断器 ══"
# 重置 parse_csv 熔断状态,保证计数从 0 开始(阈值默认 5)
PSQL "DELETE FROM tool_circuit_breakers WHERE tool_id = 'parse_csv'" > /dev/null
for i in 1 2 3 4 5; do
  RES=$(new_call "cb$i")
  confirm "$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')" failed
done
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"parse_csv\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"data_extract_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-cb-open\"}")
check_code "circuit open → 429" "$CODE" '429'
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"parse_csv\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"data_extract_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-cb-open2\"}")
check "circuit error code" "$RES" 'CIRCUIT_OPEN'
STATE=$(PSQL "SELECT state FROM tool_circuit_breakers WHERE tool_id = 'parse_csv'")
check "breaker state = open" "$STATE" 'open'
# 清理:恢复 closed,避免阻塞后续调用(m2c-smoke 等)
PSQL "DELETE FROM tool_circuit_breakers WHERE tool_id = 'parse_csv'" > /dev/null

echo "──"
echo "M2-C/D/E smoke: PASS=$PASS FAIL=$FAIL"
[[ $FAIL -eq 0 ]]
