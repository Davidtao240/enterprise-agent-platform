// Package workflow 实现跨业务工作流引擎。
//
// 核心原则：引擎只解释模板（definition_json），不硬编码业务 if/else。
// 业务差异全部通过 workflow_templates 中的 nodes + edges + graph_key 表达。
//
// 模块结构：
//   - model.go    → 类型、常量、请求/响应体
//   - repository.go → PostgreSQL CRUD
//   - engine.go   → 状态机校验、模板解释、节点调度
//   - service.go  → 业务编排（创建实例、启动、取消、重试、节点回调）
//   - handler.go  → HTTP 端点（REST API）
//   - worker.go   → Asynq 异步任务队列（入队 + 消费）
//
// 数据流（以 finance 为例）：
//   前端 POST /workflow-instances → handler → service.CreateInstance
//   → 查模板 → 创建 instance + 所有 node_instance（pending）
//   → 前端 POST /workflow-instances/{id}/start
//   → service.StartWorkflow → 入口节点入队 Asynq
//   → Asynq worker 消费 → handleExecuteNode
//   → 按节点类型分发 → 完成后 OnNodeCompleted → 找下一节点 → 入队
//   → 循环直到所有节点完成或失败
package workflow
