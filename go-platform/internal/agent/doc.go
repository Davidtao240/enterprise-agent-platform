// Package agent 实现 Agent Registry、Graph Registry、Agent Gateway 和 Agent Run Logs。
//
// 核心模块：
//   - Gateway: 统一调用入口，验证 graph → 校验 domain policy → 调 Python → 记日志
//   - Repository: 管理 agent_registry、graph_registry、agent_run_logs、approval_tasks 表
//   - Handler: Agent/Tool 的 HTTP API 端点 + 审批任务 API
//
// Gateway 是 Phase 3 的关键连线：
//   Worker(agent_graph node) → Gateway.Execute()
//     → Validate graph_key → Domain Policy Check
//     → POST Python Agent Service(/internal/v1/agent-runs)
//     → Record agent_run_logs → Return result
package agent
