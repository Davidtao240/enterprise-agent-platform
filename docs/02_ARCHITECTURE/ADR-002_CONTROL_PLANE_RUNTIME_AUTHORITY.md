# ADR-002: Go Control Plane 与 Python Runtime 的状态权威边界

> 状态：Accepted
> 日期：2026-08-16

## 决策

- Go/PostgreSQL 是 Identity、Tenant、Workflow、Approval、Policy、ToolCall、Audit 和企业 Run Index 的权威来源。
- Python Checkpointer 是 Graph 内部 State、Cursor、局部消息和恢复点的权威来源。
- 外部 ERP、工单和数据库是其业务对象事实的权威来源，平台通过 Connector `verify` 获取真实状态。

## 原因

仅让 Go 保存最终 Agent Run Log 无法恢复 Graph；仅让 Python 保存 Graph State 又无法承担企业权限、审批和审计。分层保存并通过 `run_id`、Checkpoint Version、Runtime Event 关联，可以同时获得 Runtime 灵活性和企业治理一致性。

## 后果

- Workflow 状态机和 Agent Run 状态机分离。
- Runtime Event 至少一次投递且 Go 幂等消费。
- 恢复 Checkpoint 前必须核对 ToolCall 和外部状态。
