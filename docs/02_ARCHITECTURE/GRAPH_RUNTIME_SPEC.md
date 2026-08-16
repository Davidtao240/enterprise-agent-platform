# Graph Runtime Spec

> 文档状态：Active M1 Target Specification
> 更新日期：2026-08-16
> 兼容要求：Finance V1 `/internal/v1/agent-runs` 外部行为在迁移期保持可用。

## 目的

定义 Python Agent Runtime 如何启动、持久化、暂停、恢复、取消和报告一次 Graph Run，并明确 Go Control Plane 与 Python Checkpointer 的边界。

## 核心原则

- `graph_key` 由 Workflow Template 显式提供，LLM 不选择业务域。
- Graph Definition 版本不可变，Run 绑定明确版本。
- Runtime State 可持久化，进程重启后可恢复。
- Tool Call 不在 Python 内直接执行企业动作，必须经过 Go Tool Execution Gateway。
- Go 保存企业治理状态；Python 保存 Graph 内部 Checkpoint。
- Runtime Event 至少一次投递，消费方必须幂等。

## Graph Registry

逻辑键与版本共同定位 Graph：

```text
finance_operating_report_graph@1.0.0
```

Run 创建后不得自动漂移到最新 Graph、Agent、Profile、Skill 或模型配置。

## Run 输入

M1 目标请求：

```json
{
  "protocol_version": "2.0",
  "run_id": "run_001",
  "thread_id": "thread_001",
  "trace_id": "trace_001",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "business_app_code": "finance",
  "graph": {"key": "finance_operating_report_graph", "version": "1.0.0"},
  "configuration": {
    "agent_definition_version": "1.0.0",
    "profile_or_skill_version": "finance_operating_report_profile@1.0.0",
    "model_config_version": "model_config@1.0.0"
  },
  "input": {"file_id": "file_001"},
  "trusted_context": {
    "user_id": "user_001",
    "tenant_id": "tenant_001",
    "department_id": "finance"
  },
  "budget": {"deadline_at": null, "max_steps": 30, "max_cost": 1.0}
}
```

`trusted_context` 由 Go 从认证和持久状态生成，不能采用模型输出或客户端自报的替代值。

## Run 状态

| 状态 | 含义 |
|---|---|
| `queued` | 已创建，等待 Worker |
| `running` | Runtime 正在执行 |
| `waiting_human` | 等待审批或人工输入 |
| `waiting_external` | 等待外部系统事件或长任务 |
| `succeeded` | Runtime 产生通过契约校验的最终结果 |
| `failed` | Runtime 无法继续 |
| `cancelled` | 已收到并应用取消 |

`succeeded` 只表示 Agent Run 完成，不自动表示 Workflow 已归档，也不自动表示外部业务目标已经成功。

## Runtime 操作

### Start

1. Go 创建 Run 及不可变配置快照。
2. Asynq 投递 Start Job。
3. Worker 获得 lease 后调用 Python。
4. Python 加载 Graph 和最新有效 Checkpoint。
5. Python 产生 Step/Event，直到成功、失败或 Interrupt。

### Interrupt

- Python 保存 Checkpoint 后产生 `waiting_human` 或 `waiting_external` Event。
- Go 创建对应 Approval/External Wait Record 并更新 Run Index。
- Interrupt 不允许丢失恢复所需 Schema 和版本。

### Resume

- Go 校验审批、权限、Payload、有效期和当前外部状态。
- Resume 使用唯一 idempotency key 和期望 Checkpoint Version。
- Python 只从匹配的 Checkpoint 恢复；重复 Resume 返回原结果或无副作用成功。

### Cancel

- Go 记录取消意图并通知 Runtime/Worker。
- Python 在安全边界检查取消，停止新的模型和 Tool Call。
- 已发出的外部动作不能假装回滚，必须 Verify/Reconcile。

## Lease 与 Heartbeat

- Worker 执行 Run 前获得有期限 lease。
- Heartbeat 延长 lease，必须记录 owner 和 attempt。
- lease 过期后可以被新 Worker 接管，但接管前核对 Checkpoint 和 ToolCall 状态。
- 旧 Worker 的迟到结果通过 attempt/version 校验拒绝。

## Checkpoint

Checkpoint 至少包含或引用：

- `run_id`、`thread_id`、graph key/version。
- checkpoint version/cursor。
- 当前 Graph State 和下一执行位置。
- 已提交 Runtime Event 序号。
- 已请求 ToolCall ID 及其已知状态。
- 配置版本和 Budget 消耗。

Checkpoint 不保存 Secret；敏感原文按数据策略外置并通过受权引用访问。

## Runtime Event

事件 Envelope：

```json
{
  "event_id": "evt_001",
  "run_id": "run_001",
  "sequence": 8,
  "type": "run.interrupted",
  "occurred_at": "2026-08-16T10:00:00Z",
  "payload": {},
  "checkpoint_version": 4
}
```

事件可能重复，Go 按 `event_id` 或 `(run_id, sequence)` 去重。

## Tool Call

Python 只产生结构化 Tool Request。Go Tool Gateway 从 Run 快照补齐可信身份并执行授权。Python 不传递或持有 Connector Secret。

恢复时如果 ToolCall 处于未知状态：

```text
query/verify external state
→ confirmed succeeded: reuse result
→ confirmed absent: retry if policy allows
→ indeterminate: waiting_external / manual reconcile
```

## 状态存储边界

- Python Checkpointer：Graph 内部 State、Cursor、局部消息和恢复点。
- Go PostgreSQL：Run Index、Workflow 关系、版本、Approval、ToolCall、Audit 和 Event 去重。
- 企业系统：ERP 单据、工单、生产数据的最终事实。

## M1 故障实验

1. 在模型步骤中杀死 Python，重启后从 Checkpoint 恢复。
2. 在事件返回前杀死 Go Worker，重复事件不重复推进状态。
3. 重复投递 Start/Resume Job，Run 只有一个有效 attempt。
4. 在等待审批时重启全部服务，仍能恢复。
5. 产生 ToolCall 后模拟超时，恢复路径先 Reconcile，不盲目重试。
6. 所有实验保持 Finance V1 Contract Regression 通过。
