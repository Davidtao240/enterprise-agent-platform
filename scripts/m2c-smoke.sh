#!/usr/bin/env bash
# M2-C Smoke: Tool Execution Lifecycle 端到端验证
# 前置:docker compose 全栈运行,go-backend 已重建(含 M2-C)
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
TOKEN="${INTERNAL_SERVICE_TOKEN:-dev-internal-token}"
PASS=0; FAIL=0

check() { # name expected actual_substring
  if [[ "$2" == *"$3"* ]]; then
    PASS=$((PASS+1)); echo "PASS: $1"
  else
    FAIL=$((FAIL+1)); echo "FAIL: $1 → $2"
  fi
}

IDEM="smoke-m2c-$(date +%s)"
RUN_ID="$(uuidgen | tr 'A-Z' 'a-z')"
TENANT="00000000-0000-0000-0000-000000000010" # seed finance tenant

# 1) 高风险工具调用 → pending_approval + 自动建审批
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"archive_report\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"report_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{\\\"report_id\\\":7}\",\"idempotency_key\":\"$IDEM\"}")
check "high-risk → pending_approval" "$RES" '"status":"pending_approval"'
TC_ID=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')

# 2) GET → approval_task_id 已绑定
RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC_ID" -H "X-Internal-Service-Token: $TOKEN")
check "approval bound (approval_task_id)" "$RES" '"approval_task_id"'

APPR_ID=$(echo "$RES" | sed -n 's/.*"approval_task_id":"\([^"]*\)".*/\1/p')

# 3) 审批 UI 决定:finance_manager 登录 → approve
LOGIN=$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"finance_manager","password":"password"}')
JWT=$(echo "$LOGIN" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
RES=$(curl -s -X POST "$BASE/api/v1/approval-tasks/$APPR_ID/approve" \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' -d '{"comment":"m2c smoke"}')
check "approval decide approved" "$RES" '"status":"approved"'

# 4) tool_call → executing(重检通过)
RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC_ID" -H "X-Internal-Service-Token: $TOKEN")
check "bound → executing" "$RES" '"status":"executing"'

# 5) confirm 超时 → indeterminate
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls/$TC_ID/confirm" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"indeterminate","error_json":"{\"code\":\"TIMEOUT\"}"}')
check "confirm timeout → indeterminate" "$RES" '"status":"indeterminate"'

# 6) 无证据 reconcile → 409
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls/$TC_ID/reconcile" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' -d '{"outcome":"executed"}')
check "reconcile w/o verification → 409" "$CODE" '409'

# 7) verify 写入证据(executed=true)
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls/$TC_ID/verify" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"verification_json":"{\"executed\":true,\"external_object_id\":\"obj-smoke\"}"}')
check "verify persisted" "$RES" '"verification_json"'

# 8) reconcile executed → succeeded
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls/$TC_ID/reconcile" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"outcome":"executed","detail_json":"{\"source\":\"smoke\"}"}')
check "reconcile → succeeded" "$RES" '"status":"succeeded"'

# 9) 终态后 confirm → 409
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls/$TC_ID/confirm" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' -d '{"status":"failed"}')
check "terminal re-confirm → 409" "$CODE" '409'

# 10) 拒绝路径:高风险 → 审批拒绝 → cancelled
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"archive_report\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"report_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-rej\"}")
TC2=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC2" -H "X-Internal-Service-Token: $TOKEN")
APPR2=$(echo "$RES" | sed -n 's/.*"approval_task_id":"\([^"]*\)".*/\1/p')
RES=$(curl -s -X POST "$BASE/api/v1/approval-tasks/$APPR2/reject" \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' -d '{"comment":"no"}')
check "approval decide rejected" "$RES" '"status":"rejected"'
RES=$(curl -s "$BASE/internal/v1/tool-calls/$TC2" -H "X-Internal-Service-Token: $TOKEN")
check "rejected → cancelled" "$RES" '"status":"cancelled"'

# 11) Retry Gate:低风险失败无证据不可重试
RES=$(curl -s -X POST "$BASE/internal/v1/tool-calls" \
  -H "X-Internal-Service-Token: $TOKEN" -H "X-Tenant-ID: $TENANT" -H 'Content-Type: application/json' \
  -d "{\"tool_id\":\"parse_csv\",\"tool_version\":\"1.0\",\"run_id\":\"$RUN_ID\",\"agent_id\":\"data_extract_agent\",\"business_app_code\":\"finance\",\"arguments_json\":\"{}\",\"idempotency_key\":\"$IDEM-rtry\"}")
TC3=$(echo "$RES" | sed -n 's/.*"tool_call_id":"\([^"]*\)".*/\1/p')
curl -s -X POST "$BASE/internal/v1/tool-calls/$TC3/confirm" \
  -H "X-Internal-Service-Token: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"failed","error_json":"{\"code\":\"ERR\"}"}' > /dev/null
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/internal/v1/tool-calls/$TC3/retry" \
  -H "X-Internal-Service-Token: $TOKEN")
check "retry w/o evidence → 409" "$CODE" '409'

echo "──"
echo "M2-C smoke: PASS=$PASS FAIL=$FAIL"
[[ $FAIL -eq 0 ]]
