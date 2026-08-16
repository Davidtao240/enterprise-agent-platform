# Agent IO Contract

> 文档状态：Active Versioned Contract
> 更新日期：2026-08-16

## 目的

定义 Go Control Plane、Python Agent Runtime 和 Go Tool Execution Gateway 之间的跨栈契约。Graph Run、Runtime Control 和 Tool Execution 是三个不同协议层，不能继续共用一个同时含 `graph_key`、`agent_id` 和 `tool_id` 的模糊请求体。

## 协议版本

- V1：现有同步 `POST /internal/v1/agent-runs`，作为 Finance 兼容契约保留。
- V2：Durable Run 的 Start/Resume/Cancel/Event 契约。
- Tool V1：Python Runtime 到 Go Tool Gateway 的独立执行契约。

Breaking Change 必须升级 Major Version。历史 Run 必须保存协议和配置版本。

## V1 Finance 兼容契约

迁移期间保持：

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {"file_id": "file_001"},
  "context": {
    "user_id": "user_001",
    "department_id": "finance",
    "tenant_id": "default"
  }
}
```

响应仍包含 `run_id`、`graph_key`、`status`、`output`、`usage` 和 `error`。V1 Regression Fixture 是兼容性判定依据。

## V2 Run Start Contract

详细字段见 [`GRAPH_RUNTIME_SPEC.md`](GRAPH_RUNTIME_SPEC.md)。必填类别：

- protocol/run/thread/trace identity。
- workflow/node/business app identity。
- graph key/version。
- immutable configuration versions。
- input、trusted context、budget。

响应是接受结果，不要求在一个 HTTP 请求内完成整个 Run：

```json
{
  "protocol_version": "2.0",
  "run_id": "run_001",
  "status": "queued",
  "accepted_at": "2026-08-16T10:00:00Z"
}
```

## Resume Contract

```json
{
  "protocol_version": "2.0",
  "run_id": "run_001",
  "interrupt_id": "interrupt_001",
  "expected_checkpoint_version": 4,
  "idempotency_key": "resume:interrupt_001:approved",
  "resume_input": {
    "decision": "approved",
    "approval_task_id": "approval_001"
  }
}
```

Resume Input 必须通过 Interrupt 声明的 Schema；重复请求不得重复推进。

## Cancel Contract

```json
{
  "protocol_version": "2.0",
  "run_id": "run_001",
  "reason": "cancelled_by_user",
  "requested_by": "user_001",
  "idempotency_key": "cancel:run_001"
}
```

`requested_by` 在 Go 内重新校验，不能只信任请求 JSON。

## Runtime Event Contract

Runtime Event 使用 `event_id` 和单调 `sequence` 去重。事件类型至少包括：

- `run.started`、`run.interrupted`、`run.resumed`。
- `step.started`、`step.completed`、`step.failed`。
- `checkpoint.saved`。
- `run.succeeded`、`run.failed`、`run.cancelled`。

Event Payload 必须脱敏，不能携带 Secret 或默认保存完整 Prompt/Document。

## Tool Execution Contract

Python 提交候选 Tool Call：

```json
{
  "protocol_version": "1.0",
  "run_id": "run_001",
  "step_id": "step_006",
  "tool_call_id": "toolcall_001",
  "tool": {"id": "enterprise_db_read", "version": "1.0.0"},
  "arguments": {"query_template": "monthly_metrics", "month": "2026-07"},
  "idempotency_key": "run_001:step_006:toolcall_001"
}
```

请求中不得由模型决定或覆盖：

- `user_id`、`tenant_id`、business app、graph、agent、skill。
- Credential、Connector Endpoint、Policy Version。
- 审批是否必要。

这些字段由 Go 根据 Run/Step 快照补齐。

Tool Result：

```json
{
  "tool_call_id": "toolcall_001",
  "status": "succeeded",
  "output": {},
  "external_request_id": "ext_001",
  "external_object_id": null,
  "verification": {"status": "confirmed"},
  "error": null
}
```

状态可以是：`pending_approval`、`executing`、`succeeded`、`failed`、`indeterminate`、`cancelled`。

## Error Contract

```json
{
  "code": "TOOL_EXECUTION_TIMEOUT",
  "message": "tool execution timed out",
  "retryable": true,
  "category": "transient",
  "details": {},
  "cause_id": "toolcall_001"
}
```

- `retryable` 只是错误分类，不代表调用方可以盲目重试副作用操作。
- `indeterminate` 必须先 Verify/Reconcile。
- 错误响应不得包含 Token、Connection String、完整敏感文档或内部堆栈。

## 契约测试

- V1 Finance Success、Fallback、Validation Failure、Retryable/Non-Retryable Failure。
- V2 Start、Interrupt、Resume、Cancel 和重复 Event。
- Tool 未授权、跨 Tenant、Schema Invalid、Approval Required。
- Tool Timeout、Duplicate、Indeterminate、Verify/Recover。
- Go/Python JSON 序列化和向后兼容。
