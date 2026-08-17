#!/usr/bin/env bash
# =============================================================================
# M1 Durable Agent Run — 六项故障实验运行手册
# =============================================================================
# 本脚本是"可重复执行的故障实验手册"：以注释 + 真实命令组织，用于在 Docker
# 服务已启动的环境里验证 M1 的恢复/幂等/最终一致语义。它不是一次性 demo，
# 而是可审计的验收清单。
#
# ── 前置条件（运行前必须满足）───────────────────────────────────────────────
#   1. Docker Compose 服务已按 README / 交接文档启动：
#        postgres redis minio qdrant go-backend agent-service
#      （frontend 可选，本实验不需要）
#   2. 根目录 .env 中已配置 INTERNAL_SERVICE_TOKEN（服务间认证，非空；为空则
#      internal 路由 fail closed）。本脚本读取该值用于 internal 调用。
#   3. 演示账号已由 migration 播种（4 个用户，密码均为 password）：
#        admin / finance_user / finance_manager / ops_viewer
#   4. 依赖本地工具：curl、python3（解析 JSON）、docker compose、psql（经容器）。
#
# ── 可调参数（环境变量覆盖）──────────────────────────────────────────────────
#   BASE_URL        对外 API 基址        默认 http://localhost:8080/api/v1
#   GO_HOST         Go 后端              默认 http://localhost:8080
#   AGENT_HOST      Python Agent Service  默认 http://localhost:8000
#   INTERNAL_SERVICE_TOKEN  服务间令牌    默认取 .env 的 INTERNAL_SERVICE_TOKEN
#   DB_USER/DB_NAME postgres 连接         默认 platform / enterprise_agent_platform
#   PASSWORD        演示账号密码          默认 password
#   AUTO            置 1 时跳过 read -r 暂停（自动连跑）
#
# ── 用法 ─────────────────────────────────────────────────────────────────────
#   bash scripts/m1-fault-experiments.sh        # 顺序执行 1..6，每项间暂停
#   bash scripts/m1-fault-experiments.sh 3      # 只执行第 3 项
#   AUTO=1 bash scripts/m1-fault-experiments.sh # 不暂停，连跑
#
# ── 重要说明 ─────────────────────────────────────────────────────────────────
#   - 实验 5（ToolCall 超时）是占位/模拟：真实 ToolCall 属于 M2 Tool Execution
#     Gateway，本实验只验证 M1 的"无真实 ToolCall 步骤"前提并给出 Reconcile
#     语义说明与 M2 检查点。
#   - Finance V1 演示图的审批是 human_review 节点（经典审批，不携带
#     durable_run_id/interrupt_id）。运行时中断审批（durable_run_id+interrupt_id）
#     由使用 LangGraph interrupt 的 V2 图触发，本手册在实验 4 中以数据库断言
#     区分两种形态，不臆造演示数据。
# =============================================================================

set -euo pipefail

# ── 环境变量与参数 ───────────────────────────────────────────────────────────
BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
GO_HOST="${GO_HOST:-http://localhost:8080}"
AGENT_HOST="${AGENT_HOST:-http://localhost:8000}"
DB_USER="${DB_USER:-platform}"
DB_NAME="${DB_NAME:-enterprise_agent_platform}"
PASSWORD="${PASSWORD:-password}"
AUTO="${AUTO:-0}"

# 从 .env 读取 INTERNAL_SERVICE_TOKEN（若调用方未显式提供）。
if [[ -z "${INTERNAL_SERVICE_TOKEN:-}" && -f .env ]]; then
  INTERNAL_SERVICE_TOKEN="$(sed -n 's/^INTERNAL_SERVICE_TOKEN[[:space:]]*=[[:space:]]*//p' .env | tail -n1 || true)"
fi

# Python 用于解析 JSON。
if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  echo "需要 python3 解析 JSON 响应" >&2
  exit 1
fi

CSV_PATH="${CSV_PATH:-/tmp/m1-fault-demo.csv}"

# ── 工具函数 ─────────────────────────────────────────────────────────────────

# 从 JSON 字符串取点分路径字段（数字段表示数组索引，如 data.0.id）。
json_get() {
  "$PYTHON_BIN" - "$1" "$2" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
for part in sys.argv[2].split("."):
    data = data[int(part)] if isinstance(data, list) else data[part]
print(data)
PY
}

# 从 stdin 的 JSON 取字段（轮询用，失败输出空串且不退出）。
field() {
  "$PYTHON_BIN" -c 'import json,sys
try:
    d=json.load(sys.stdin)
    for k in sys.argv[1].split("."):
        d=d[int(k)] if isinstance(d,list) else d[k]
    print(d)
except Exception:
    print("")' "$1"
}

# 严格版 API 请求：非 2xx 即失败退出。
api_request() {
  local resp_file status
  resp_file="$(mktemp)"
  status=$(curl -sS -w '%{http_code}' -o "$resp_file" "$@")
  if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
    echo "API 请求失败 HTTP $status:" >&2
    cat "$resp_file" >&2
    rm -f "$resp_file"
    return 1
  fi
  cat "$resp_file"
  rm -f "$resp_file"
}

# 经 postgres 容器执行 psql，输出 tuple-only、竖线分隔。
db_row() {
  docker compose exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -t -A -F '|' -c "$1"
}

# 登录并返回 access_token。
# 注意：body 必须先经 printf 构造。macOS 自带 bash 3.2 对「双引号命令替换内
# 再嵌套 \" 转义」存在词法缺陷，会把一条命令拆成两条，因此禁止在 $( ) 内
# 内联书写转义 JSON。
login() {
  local username="$1" body
  body="$(printf '{"username":"%s","password":"%s"}' "$username" "$PASSWORD")"
  json_get "$(api_request -X POST "$BASE_URL/auth/login" \
    -H 'Content-Type: application/json' \
    -d "$body")" "data.access_token"
}

# 轮询工作流实例状态直到等于期望值；服务重启期间连接失败会忽略重试。
wait_workflow_status() {
  local token="$1" wf="$2" want="$3" tries="${4:-120}" interval="${5:-2}"
  local status=""
  for _ in $(seq 1 "$tries"); do
    status="$(curl -sS "$BASE_URL/workflow-instances/$wf" \
      -H "Authorization: Bearer $token" | field 'data.status' || true)"
    if [ "$status" = "$want" ]; then
      return 0
    fi
    if [ "$status" = "failed" ]; then
      echo "工作流 $wf 进入 failed（期望 ${want}）" >&2
      return 1
    fi
    sleep "$interval"
  done
  echo "等待 $((tries*interval))s 后工作流 $wf 状态=${status}（期望 ${want}）" >&2
  return 1
}

# 等待工作流出现 pending 审批任务并返回其 id（Finance 演示流程 Run 成功后
# 进入 waiting_review，必须审批后才能 archived）。
wait_pending_approval() {
  local review_token="$1" wf="$2" approval_id=""
  for _ in $(seq 1 60); do
    approval_id="$(api_request "$BASE_URL/approval-tasks?workflow_instance_id=$wf&status=pending" \
      -H "Authorization: Bearer $review_token" | field 'data.0.id' || true)"
    [ -n "$approval_id" ] && { echo "$approval_id"; return 0; }
    sleep 2
  done
  echo "未能观察到 $wf 的 pending 审批任务" >&2
  return 1
}

# 以 finance_manager 审批并等待工作流归档（实验 1..3 的完整闭环收尾）。
approve_and_archive() {
  local token="$1" review_token="$2" wf="$3" approval_id
  approval_id="$(wait_pending_approval "$review_token" "$wf")"
  api_request -X POST "$BASE_URL/approval-tasks/$approval_id/approve" \
    -H "Authorization: Bearer $review_token" \
    -H 'Content-Type: application/json' \
    -d '{"comment":"Approved by M1 fault experiment."}' > /dev/null
  wait_workflow_status "$token" "$wf" "archived" 120 2
}

# 分段暂停（AUTO=1 跳过）。
pause() {
  if [ "$AUTO" = "1" ]; then
    return 0
  fi
  read -r -p ">>> 按回车继续下一项（Ctrl-C 退出）..." _
}

# 创建并启动一个 Finance 演示工作流，返回 workflow 实例 id。
create_finance_demo() {
  local token="$1"
  if [ ! -f "$CSV_PATH" ]; then
    cat > "$CSV_PATH" <<'CSV'
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,1200000,760000,440000,310000,860,1430
2026-05,East Region,680000,420000,260000,180000,420,760
CSV
  fi
  local file_id wf_id body
  file_id="$(json_get "$(api_request -X POST "$BASE_URL/files" \
    -H "Authorization: Bearer $token" \
    -F business_app_code=finance \
    -F file_role=source \
    -F "file=@$CSV_PATH")" "data.file_id")"
  body="$(printf '{"business_app_code":"finance","workflow_template_key":"finance_operating_report","title":"M1 fault experiment","input":{"month":"2026-05","department":"Finance Center","file_id":"%s"}}' "$file_id")"
  wf_id="$(json_get "$(api_request -X POST "$BASE_URL/workflow-instances" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d "$body")" "data.id")"
  api_request -X POST "$BASE_URL/workflow-instances/$wf_id/start" \
    -H "Authorization: Bearer $token" > /dev/null
  echo "$wf_id"
}

# 取某工作流最新的 durable run 行（id|tenant|node|attempt|status）。
latest_run_row() {
  db_row "SELECT ar.id, ar.tenant_id, ar.node_instance_id, ar.attempt, ar.status
          FROM agent_runs ar
          WHERE ar.workflow_instance_id = '$1'
          ORDER BY ar.created_at DESC LIMIT 1;"
}

# ── 实验 1：模型步骤杀 Python 后从 Checkpoint 恢复 ───────────────────────────
experiment_1() {
  echo
  echo "=============================================================="
  echo "实验 1：杀 agent-service（Python）后从 Checkpoint 恢复"
  echo "=============================================================="
  echo "目的：Python 进程中途被杀，重启后 Run 从 LangGraph Checkpoint 恢复"
  echo "      （thread_id=run_id），而不是重新开始或凭空失败。"
  echo

  local token wf run_line run_id status
  token="$(login finance_user)"
  wf="$(create_finance_demo "$token")"
  echo "已创建工作流并启动: $wf"

  echo "-- 等待 agent_graph 节点进入 running 且 durable run 出现 ..."
  for _ in $(seq 1 60); do
    run_line="$(latest_run_row "$wf" || true)"
    [ -n "$run_line" ] && break
    sleep 2
  done
  [ -n "$run_line" ] || { echo "未能在 120s 内观察到 durable run" >&2; exit 1; }
  echo "run 行: $run_line"
  run_id="$(echo "$run_line" | cut -d'|' -f1)"

  echo "-- 注入故障：重启 agent-service（模拟模型步骤中杀 Python）"
  echo "   注：无 LLM 确定性模式下 Run 可能毫秒级完成，重启未必命中执行中；"
  echo "   命中时验证 checkpoint 恢复，未命中时验证正常完成，终态断言一致。"
  docker compose restart agent-service

  echo "-- 等待恢复：Python 启动时 recover() 重新调度 queued/running 的 Run，"
  echo "   从 LangGraph checkpoint 续跑直至成功（推进到 waiting_review）。"
  wait_workflow_status "$token" "$wf" "waiting_review" 120 2

  echo "-- 断言 A：Run 终态为 succeeded（V1 桥路径 checkpoint_version 允许为空；"
  echo "   该列由 V2 Runtime Event 路径维护，见实验 2 的 runtime_events 断言）"
  db_row "SELECT id, status, attempt, checkpoint_version
          FROM agent_runs WHERE id = '$run_id';"
  echo "   期望 status=succeeded；checkpoint_version 空(V1 桥)或 >=1(V2)"

  echo "-- 审批收尾：验证恢复后的 Run 输出能继续推进到归档"
  approve_and_archive "$token" "$(login finance_manager)" "$wf"

  echo "-- 断言 B：工作流已推进到 archived"
  db_row "SELECT id, status FROM workflow_instances WHERE id = '$wf';"
  echo
  echo "[实验 1 完成]"
}

# ── 实验 2：事件返回前杀 Go Worker，重复事件不重复推进状态 ──────────────────
experiment_2() {
  echo
  echo "=============================================================="
  echo "实验 2：杀 go-backend（Go Worker），重复事件不重复推进状态"
  echo "=============================================================="
  echo "目的：Python Outbox 至少一次投递 Runtime Event；Go 重启后即使重投，"
  echo "      也按 event_id/(run_id,sequence) 去重，节点/状态不重复推进。"
  echo

  local token wf run_line run_id
  token="$(login finance_user)"
  wf="$(create_finance_demo "$token")"
  echo "已创建工作流并启动: $wf"

  echo "-- 等待 durable run 出现并开始消费事件 ..."
  for _ in $(seq 1 60); do
    run_line="$(latest_run_row "$wf" || true)"
    [ -n "$run_line" ] && break
    sleep 2
  done
  run_id="$(echo "$run_line" | cut -d'|' -f1)"
  echo "run_id: $run_id"

  echo "-- 注入故障：在事件流投递期间重启 go-backend"
  docker compose restart go-backend

  echo "-- 等待事件重投并被幂等消费，工作流推进到 waiting_review"
  wait_workflow_status "$token" "$wf" "waiting_review" 120 2

  echo "-- 断言 A：runtime_events 无重复（event_id 与 (run_id,sequence) 唯一）"
  db_row "SELECT count(*) AS total,
                 count(DISTINCT event_id) AS distinct_event_ids,
                 count(DISTINCT (run_id, sequence)) AS distinct_seq
          FROM runtime_events WHERE run_id = '$run_id';"
  echo "   期望三个值相等（total = distinct_event_ids = distinct_seq）"

  echo "-- 断言 B：agent_graph 节点只被推进一次（单一终态，不重复推进）"
  db_row "SELECT node_key, status, count(*) FROM workflow_node_instances
          WHERE workflow_instance_id = '$wf' GROUP BY node_key, status
          ORDER BY node_key;"
  echo "   期望每个 node_key 只出现一次终态 status"

  echo "-- 审批收尾：验证事件幂等消费后的完整闭环"
  approve_and_archive "$token" "$(login finance_manager)" "$wf"
  echo
  echo "[实验 2 完成]"
}

# ── 实验 3：重复投递 Start，Run 只有一个有效 attempt ────────────────────────
experiment_3() {
  echo
  echo "=============================================================="
  echo "实验 3：重复投递 Start Job，Run 只有一个有效 attempt"
  echo "=============================================================="
  echo "目的：同一节点的 Start 任务重放，agent_runs 的 (tenant,node,attempt)"
  echo "      唯一约束保证只有一行；失联接管以同一 attempt 重驱动，不产生新 attempt。"
  echo

  local token wf run_line node_id tenant_id
  token="$(login finance_user)"
  wf="$(create_finance_demo "$token")"
  echo "已创建工作流并启动: $wf"

  for _ in $(seq 1 60); do
    run_line="$(latest_run_row "$wf" || true)"
    [ -n "$run_line" ] && break
    sleep 2
  done
  node_id="$(echo "$run_line" | cut -d'|' -f3)"
  tenant_id="$(echo "$run_line" | cut -d'|' -f2)"
  echo "tenant_id=$tenant_id  node_instance_id=$node_id"

  echo "-- 断言 A：初始时 (tenant,node,attempt=1) 恰好一行"
  db_row "SELECT tenant_id, node_instance_id, attempt, count(*)
          FROM agent_runs
          WHERE tenant_id = '$tenant_id' AND node_instance_id = '$node_id'
          GROUP BY tenant_id, node_instance_id, attempt;"
  echo "   期望只有一行 attempt=1 且 count=1"

  echo "-- 注入故障：把 lease 置为已过期，模拟 Worker 失联，"
  echo "   收敛扫描器将以同一 attempt 重新入队（不 attempt+1）。"
  db_row "UPDATE agent_runs SET lease_expires_at = now() - interval '5 seconds'
          WHERE tenant_id = '$tenant_id' AND node_instance_id = '$node_id'
            AND status IN ('queued','running');" > /dev/null

  echo "-- 等待收敛扫描器（默认 60s 周期）以同一 attempt 重驱动并推进到 waiting_review"
  wait_workflow_status "$token" "$wf" "waiting_review" 150 2

  echo "-- 断言 B：重放/接管后仍只有一行，且 attempt 仍为 1（无 attempt=2 行）"
  db_row "SELECT tenant_id, node_instance_id, attempt, count(*)
          FROM agent_runs
          WHERE tenant_id = '$tenant_id' AND node_instance_id = '$node_id'
          GROUP BY tenant_id, node_instance_id, attempt;"
  echo "   期望仍只有一行 attempt=1 且 count=1"

  echo "-- 审批收尾：验证接管重驱动后的完整闭环"
  approve_and_archive "$token" "$(login finance_manager)" "$wf"
  echo
  echo "说明：Resume 的幂等（approval:<interrupt_id>）在实验 4 的审批恢复中一并验证。"
  echo "[实验 3 完成]"
}

# ── 实验 4：等待审批时重启全部服务，审批后仍能继续 ──────────────────────────
experiment_4() {
  echo
  echo "=============================================================="
  echo "实验 4：等待审批时重启全部服务，审批后仍能继续"
  echo "=============================================================="
  echo "目的：工作流进入 waiting_review 后全量重启，审批决策仍能驱动推进。"
  echo

  local token review_token wf approval_id
  token="$(login finance_user)"
  review_token="$(login finance_manager)"
  wf="$(create_finance_demo "$token")"
  echo "已创建工作流并启动: $wf"

  echo "-- 等待进入 waiting_review 并出现审批任务 ..."
  wait_workflow_status "$token" "$wf" "waiting_review" 120 2
  for _ in $(seq 1 30); do
    approval_id="$(api_request "$BASE_URL/approval-tasks?workflow_instance_id=$wf&status=pending" \
      -H "Authorization: Bearer $review_token" | field 'data.0.id' || true)"
    [ -n "$approval_id" ] && break
    sleep 2
  done
  [ -n "$approval_id" ] || { echo "未能在 60s 内观察到 pending 审批任务" >&2; exit 1; }
  echo "approval_task_id: $approval_id"

  echo "-- 断言 A：审批任务形态（finance 演示为经典 human_review，runtime 中断为 NULL）"
  db_row "SELECT id, status,
                 (durable_run_id IS NOT NULL) AS runtime_interrupt,
                 interrupt_id
          FROM approval_tasks WHERE id = '$approval_id';"
  echo "   经典 human_review：runtime_interrupt=f、interrupt_id 为空；"
  echo "   运行时中断审批：runtime_interrupt=t 且携带 interrupt_id（M1-C 新增列）。"

  echo "-- 注入故障：重启全部服务（等待审批期间）"
  docker compose restart postgres redis minio qdrant go-backend agent-service

  echo "-- 等待服务恢复健康 ..."
  sleep 10
  for _ in $(seq 1 30); do
    if curl -sS -o /dev/null "$GO_HOST/health" 2>/dev/null; then break; fi
    sleep 2
  done

  echo "-- 审批（finance_manager）"
  api_request -X POST "$BASE_URL/approval-tasks/$approval_id/approve" \
    -H "Authorization: Bearer $review_token" \
    -H 'Content-Type: application/json' \
    -d '{"comment":"Approved after full restart."}' > /dev/null

  echo "-- 等待推进到归档"
  wait_workflow_status "$token" "$wf" "archived" 120 2

  echo "-- 断言 B：审批已落库且工作流推进"
  db_row "SELECT status, decided_at IS NOT NULL AS decided FROM approval_tasks WHERE id = '$approval_id';"
  db_row "SELECT id, status FROM workflow_instances WHERE id = '$wf';"
  echo "   期望 approval status=approved、workflow status=archived"

  echo "-- 说明：若审批任务携带 durable_run_id+interrupt_id，approve 会调用"
  echo "   ResumeInterruptedRun(resume_input={\"decision\":\"approved\"}) 恢复 Run，"
  echo "   随后 Run 的终态事件经 RunEventBridge 推进节点。"
  echo
  echo "[实验 4 完成]"
}

# ── 实验 5：ToolCall 超时先 Reconcile（占位/模拟） ───────────────────────────
experiment_5() {
  echo
  echo "=============================================================="
  echo "实验 5：ToolCall 超时先 Reconcile（占位/模拟）"
  echo "=============================================================="
  echo "目的：验证"恢复路径对不确定外部副作用先 Verify/Reconcile、不盲目重试"的"
  echo "      前置条件，并说明 M2 Tool Execution Gateway 落地后的验收方式。"
  echo "注：真实 ToolCall 属于 M2；M1 尚无 Tool Execution Gateway。"
  echo

  echo "-- 断言 A：M1 运行时不存在真实 tool 步骤（step_type='tool' 应为 0）"
  db_row "SELECT count(*) AS tool_steps FROM agent_run_steps WHERE step_type = 'tool';"
  echo "   期望 tool_steps=0"

  echo "-- 断言 B：tool_calls 表尚未创建（M2 Target，to_regclass 应为空）"
  db_row "SELECT to_regclass('public.tool_calls') AS tool_calls_table;"
  echo "   期望 tool_calls_table 为空行（NULL）"

  echo "-- Reconcile 语义（M2 落地后据此验收，当前为说明）"
  cat <<'EOF'
Python 只产生结构化 Tool Request，Go Tool Gateway 补齐可信身份并执行。恢复时
若 ToolCall 状态未知，按以下顺序，不盲目重试副作用：
  query/verify external state
  → confirmed succeeded: 复用结果
  → confirmed absent:    策略允许则重试
  → indeterminate:       进入 waiting_external / 人工 reconcile
对应契约错误码：TOOL_CALL_INDETERMINATE（必须先 Verify/Reconcile）。
M2 接口：POST /internal/v1/tool-calls、
         GET  /internal/v1/tool-calls/{id}、
         POST /internal/v1/tool-calls/{id}/reconcile
EOF
  echo
  echo "[实验 5 完成（占位/模拟）]"
}

# ── 实验 6：Finance V1 Contract Regression 持续通过 ─────────────────────────
experiment_6() {
  echo
  echo "=============================================================="
  echo "实验 6：Finance V1 Contract Regression 持续通过"
  echo "=============================================================="
  echo "目的：Python 侧 V1 契约回归（离线 fixture，无需 LLM）持续通过。"
  echo

  echo "-- 在 agent-service 容器内运行 unittest 回归测试"
  docker compose exec -T agent-service \
    python -m unittest discover -s tests -p 'test_*.py' -v

  echo "-- 亦可单独运行 Finance Contract Regression："
  echo "   docker compose exec -T agent-service \\"
  echo "     python -m unittest discover -s tests -p 'test_finance_contract_regression.py' -v"
  echo
  echo "[实验 6 完成]"
}

# ── 主入口 ───────────────────────────────────────────────────────────────────
experiments=(experiment_1 experiment_2 experiment_3 experiment_4 experiment_5 experiment_6)

run_one() {
  case "$1" in
    1) experiment_1 ;;
    2) experiment_2 ;;
    3) experiment_3 ;;
    4) experiment_4 ;;
    5) experiment_5 ;;
    6) experiment_6 ;;
    *) echo "未知实验编号: $1（支持 1..6）" >&2; exit 1 ;;
  esac
}

main() {
  echo "M1 故障实验运行手册"
  echo "  BASE_URL=$BASE_URL"
  echo "  GO_HOST=$GO_HOST  AGENT_HOST=$AGENT_HOST"
  echo "  INTERNAL_SERVICE_TOKEN=$([ -n "$INTERNAL_SERVICE_TOKEN" ] && echo '<已配置>' || echo '<空:internal 路由将 fail closed>')"
  echo

  if [ "$#" -ge 1 ]; then
    run_one "$1"
    return 0
  fi

  local idx=1
  for fn in "${experiments[@]}"; do
    "$fn"
    if [ "$idx" -lt "${#experiments[@]}" ]; then
      pause
    fi
    idx=$((idx + 1))
  done
  echo
  echo "六项故障实验执行完毕。"
}

main "$@"
