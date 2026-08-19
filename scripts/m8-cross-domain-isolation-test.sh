#!/usr/bin/env bash
# =============================================================================
# M8 Cross-Domain Isolation Penetration Test
# -----------------------------------------------------------------------------
# 用途：验证 M8 的 Domain Policy 跨域隔离能力是否真的生效
#
# 场景：
#   Case 1: Procurement 身份 → 尝试调用 finance.* Tool → 预期 DENY(403)
#   Case 2: Procurement 身份 → 尝试调用 hr.* / legal.* / it.* Tool → 预期 DENY
#   Case 3: Finance 身份 → 尝试调用 procurement.* Tool → 预期 DENY
#   Case 4: Finance 身份 → 正常调用 finance.* Tool（基线，预期 ALLOW=200 或业务执行）
#   Case 5: Ops Viewer → 尝试调用 procurement.* + finance.* → 预期 DENY
#
# 用法：
#   # 1) 设置环境变量（JWT 可从登录接口获取；$BASE_URL 指向 Go 后端）
#   export BASE_URL=http://localhost:8080
#   export PROC_USER_JWT=eyJhbGciOi....
#   export FIN_USER_JWT=eyJhbGciOi....
#   export OPS_USER_JWT=eyJhbGciOi....
#   export ADMIN_JWT=eyJhbGciOi....
#
#   # 2) 执行
#   bash scripts/m8-cross-domain-isolation-test.sh
#
#   # 3) 预期输出：Summary: PASS=5 FAIL=0
#
# 失败处理：任一类返回 200 且能拿到数据 → 立即 exit 1
# =============================================================================
set -uo pipefail
IFS=$'\n\t'

BASE="${BASE_URL:-http://localhost:8080}"
TMP_BODY="${TMPDIR:-/tmp}/m8_penetrate_body.json"
TMP_HEADER="${TMPDIR:-/tmp}/m8_penetrate_headers.txt"

REQUIRED_VARS=(PROC_USER_JWT FIN_USER_JWT)
for v in "${REQUIRED_VARS[@]}"; do
  if [ -z "${!v:-}" ]; then
    echo "❌ Missing env var: $v" >&2
    echo "   请先从登录接口获取 JWT，导出环境变量后再执行。" >&2
    exit 2
  fi
done

PASS_CNT=0
FAIL_CNT=0
FAIL_LOGS=()

echo "═══════════════════════════════════════════════════════════"
echo "  M8 Cross-Domain Isolation Penetration Test"
echo "  Target: $BASE"
echo "  Time:   $(date '+%Y-%m-%d %H:%M:%S')"
echo "═══════════════════════════════════════════════════════════"
echo

# -----------------------------------------------------------------------------
# run_deny_case <label> <jwt> <json_body>
#   断言：返回 403 / 401 / 或 Body 中包含 DOMAIN_POLICY_VIOLATION / PERMISSION_DENIED
# -----------------------------------------------------------------------------
run_deny_case() {
  local label="$1"
  local jwt="$2"
  local json="$3"

  local http_code
  http_code="$(curl -sS -o "$TMP_BODY" -D "$TMP_HEADER" -w "%{http_code}" \
    -X POST "$BASE/api/v1/tools/calls" \
    -H "Authorization: Bearer $jwt" \
    -H "Content-Type: application/json" \
    -H "X-Test-Scenario: m8-cross-domain-isolation" \
    -d "$json")"

  local body
  body="$(cat "$TMP_BODY")"

  local denied=false
  if [[ "$http_code" == "403" || "$http_code" == "401" ]]; then
    denied=true
  elif echo "$body" | grep -Eq 'DOMAIN_POLICY_VIOLATION|PERMISSION_DENIED|ACCESS_DENIED|permission.?denied|not.?authorized'; then
    denied=true
  fi

  if $denied; then
    echo "  [✅ PASS] $label"
    echo "          HTTP=$http_code"
    PASS_CNT=$((PASS_CNT+1))
  else
    echo "  [❌ FAIL] $label"
    echo "          HTTP=$http_code  (expected 401/403 + DOMAIN_POLICY_VIOLATION)"
    echo "          Resp body: $(echo "$body" | tr -d '\n' | head -c 600)"
    FAIL_CNT=$((FAIL_CNT+1))
    FAIL_LOGS+=("[$label] HTTP=$http_code, body=$body")
  fi
}

# -----------------------------------------------------------------------------
# run_allow_baseline <label> <jwt> <json_body>
#   断言：返回 2xx 或业务逻辑层面的成功（不强制 403）——用于判定环境可用
# -----------------------------------------------------------------------------
run_allow_baseline() {
  local label="$1"
  local jwt="$2"
  local json="$3"

  local http_code
  http_code="$(curl -sS -o "$TMP_BODY" -D "$TMP_HEADER" -w "%{http_code}" \
    -X POST "$BASE/api/v1/tools/calls" \
    -H "Authorization: Bearer $jwt" \
    -H "Content-Type: application/json" \
    -H "X-Test-Scenario: m8-cross-domain-baseline" \
    -d "$json")"

  if [[ "$http_code" =~ ^2 ]]; then
    echo "  [✅ PASS] $label (baseline, HTTP=$http_code)"
    PASS_CNT=$((PASS_CNT+1))
  else
    echo "  [⚠️  WARN] $label (baseline non-2xx, HTTP=$http_code)"
    echo "          这不代表跨域失效，但建议先确保基线环境正常。"
  fi
}

# =============================================================================
# Case 组 A：Procurement → Finance（越权调用，必须全部拒绝）
# =============================================================================
echo "▶ Group A — Procurement 身份 → 尝试访问 Finance Tool（必须 DENY）"
echo

run_deny_case "A1: Proc → finance.report_generate (季度报表)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"finance.report_generate","input":{"period":"Q3_2026"},"run_as_business_app":"procurement"}'

run_deny_case "A2: Proc → finance.voucher_read (凭证读取)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"finance.voucher_read","input":{"voucher_id":"FV-2026-000123"}}'

run_deny_case "A3: Proc → finance.expense_report_read (报销读取)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"finance.expense_report_read","input":{"report_id":"EXP-001"}}'

run_deny_case "A4: Proc → finance.budget_snapshot (预算查询)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"finance.budget_snapshot","input":{"department_id":"FIN01"}}'

echo

# =============================================================================
# Case 组 B：Procurement → 其它非采购域（HR / Legal / IT），必须拒绝
# =============================================================================
echo "▶ Group B — Procurement 身份 → 访问 HR/Legal/IT Tool（必须 DENY）"
echo

run_deny_case "B1: Proc → hr.employee_read (员工信息读取)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"hr.employee_read","input":{"employee_id":"E-10086"}}'

run_deny_case "B2: Proc → legal.contract_search (合同搜索)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"legal.contract_search","input":{"keyword":"供应商"}}'

run_deny_case "B3: Proc → it.account_create (IT 开户)" \
  "$PROC_USER_JWT" \
  '{"tool_code":"it.account_create","input":{"username":"malicious"}}'

echo

# =============================================================================
# Case 组 C：Finance → Procurement（反向越权，必须拒绝）
# =============================================================================
echo "▶ Group C — Finance 身份 → 尝试访问 Procurement Tool（必须 DENY）"
echo

run_deny_case "C1: Fin → procurement.supplier_query (供应商查询)" \
  "$FIN_USER_JWT" \
  '{"tool_code":"procurement.supplier_query","input":{"supplier_ids":["supplier_alpha"]}}'

run_deny_case "C2: Fin → procurement.purchase_order_read (PO 读取)" \
  "$FIN_USER_JWT" \
  '{"tool_code":"procurement.purchase_order_read","input":{"po_id":"PO-2026-007"}}'

run_deny_case "C3: Fin → procurement.policy_lookup (采购制度查看)" \
  "$FIN_USER_JWT" \
  '{"tool_code":"procurement.policy_lookup","input":{"section":"payment_terms"}}'

run_deny_case "C4: Fin → procurement.budget_check (采购预算检查)" \
  "$FIN_USER_JWT" \
  '{"tool_code":"procurement.budget_check","input":{"budget_amount":600000,"estimated_amount":500000}}'

echo

# =============================================================================
# Case 组 D：基线（Finance 自己调自己应该被允许）
# =============================================================================
echo "▶ Group D — Finance → Finance（基线，应当 ALLOW 2xx）"
echo

run_allow_baseline "D1: Fin → finance.report_generate (自身权限，基线)" \
  "$FIN_USER_JWT" \
  '{"tool_code":"finance.report_generate","input":{"period":"Q2_2026"}}'

echo

# =============================================================================
# Case 组 E：Ops Viewer（只读，应被业务 Tool 写接口拒绝，若存在）
# =============================================================================
if [ -n "${OPS_USER_JWT:-}" ]; then
  echo "▶ Group E — Ops Viewer → 调用写接口/业务接口（建议 DENY）"
  echo
  run_deny_case "E1: Ops Viewer → finance.report_generate" \
    "$OPS_USER_JWT" \
    '{"tool_code":"finance.report_generate","input":{"period":"Q3_2026"}}'
  run_deny_case "E2: Ops Viewer → procurement.supplier_query" \
    "$OPS_USER_JWT" \
    '{"tool_code":"procurement.supplier_query","input":{"supplier_ids":["alpha"]}}'
  echo
else
  echo "▶ Group E — Skipped (OPS_USER_JWT not provided)"
  echo
fi

# =============================================================================
# Case 组 F：Admin 通过 run_as_business_app 冒充采购 → 调 finance
#           （如果 Domain Policy 是强模式，应当拒绝；非强模式可作为告警记录）
# =============================================================================
if [ -n "${ADMIN_JWT:-}" ]; then
  echo "▶ Group F — Admin 伪装 procurement 调 finance（强模式下必须 DENY）"
  echo
  run_deny_case "F1: Admin(run_as=procurement) → finance.voucher_read" \
    "$ADMIN_JWT" \
    '{"tool_code":"finance.voucher_read","input":{"voucher_id":"FV-000001"},"run_as_business_app":"procurement"}'
  echo
else
  echo "▶ Group F — Skipped (ADMIN_JWT not provided)"
  echo
fi

# =============================================================================
# Summary
# =============================================================================
echo "═══════════════════════════════════════════════════════════"
echo "  Test Summary"
echo "═══════════════════════════════════════════════════════════"
echo "  PASS  count = $PASS_CNT"
echo "  FAIL  count = $FAIL_CNT"
echo

if [ "$FAIL_CNT" -eq 0 ]; then
  echo "  ✅  CROSS-DOMAIN ISOLATION VERIFIED — Gate-3 PASS"
  exit 0
else
  echo "  ❌  $FAIL_CNT VIOLATIONS DETECTED — Gate-3 FAIL"
  echo
  echo "  ┌─ FAIL details ─────────────────────────────────────────"
  i=1
  for log in "${FAIL_LOGS[@]}"; do
    echo "  │ F$i. $log"
    i=$((i+1))
  done
  echo "  └────────────────────────────────────────────────────────"
  exit 1
fi
