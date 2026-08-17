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

> 实现状态：V1 桥执行路径已按 `### M1-C-A` 落地；V2 异步路径待
> M1-C-C 事件驱动化后迁移。

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

### Current M1-B Checkpointer Backend

当前实现使用持久 SQLite `AsyncSqliteSaver`，默认容器路径为
`/app/runtime/agent-checkpoints.sqlite3`，由 `agent_runtime_data` named volume
持久化。除了 LangGraph 自有 checkpoint/writes 表，Runtime 自有表保存：

- Run 的协议快照、单调 Checkpoint Version 和 active Interrupt。
- Start/Resume/Cancel 的 request hash 与幂等响应。
- 按 Run 单调 sequence 的 Runtime Event Outbox 和投递状态。

事件投递采用 head-of-line 语义：每个 Run 只有最靠前、退避已到期的待投递
事件可被投出；Go 返回 4xx（终态拒绝）时事件进入 dead-letter 并告警，
5xx/网络错误按指数退避（上限 60s）重试。

SQLite 是 M1 的单 Runtime 实例持久后端；横向扩展前必须升级到支持多实例
协调的 Checkpointer，不得把内存状态重新变成唯一事实来源。

### M1-C-A：Worker Lease 与失联接管（已实现）

> M1-C-B（迟到结果拒绝的 lease owner 校验）已在同一批实现，见下文
> "迟到结果拒绝"条目与 `TestDurableRunLateResultRejectionPostgresAcceptance`。

V1 桥执行路径已接入 `agent_runs` 的 lease 列（`lease_owner`、
`lease_expires_at`、`heartbeat_at`）：

- **获取**：每次执行尝试生成唯一 owner token，以单条条件 UPDATE 原子获取
  lease（无主、已过期、或持有者正是 owner 才成功）。默认 TTL 90s。
- **心跳**：Python 调用期间每 30s 续租并记录 `heartbeat_at`；lease 被接管
  （owner 不匹配）时停止心跳并告警。存活 Worker 的长期同步调用不会被
  误判为失联。
- **接管**：收敛扫描器（60s 周期）找出 lease 已过期的非终态 Run，对
  "节点仍 running 且 attempt 匹配"的节点重新入队；重新执行时以同一 Run
  身份重驱动 Python（Graph 以 run_id 为 checkpoint thread，幂等恢复）。
- **迟到结果拒绝**：完成路径按 attempt 校验拒绝旧 attempt；同 attempt 的
  迟到完成由 lease owner 校验拒绝（M1-C-B）——`V1DurableRunCompletion`
  携带执行者 owner token，Run 仍非终态且 `lease_owner` 是其他执行者时，
  完成写入返回 `ErrLeaseNotHeld`，不覆盖接管者的执行；同状态幂等重放
  与空 owner（升级前存量）不受影响。Gateway 收到 `ErrLeaseNotHeld` 时
  不向 Worker 报错，而是回放 Run 当前状态：非终态回放使 Worker 跳过
  推进（接管者仍在执行），避免 `OnNodeFailed` 回退节点。
- **升级兼容**：从未取得 lease 的存量非终态 Run，以
  `AGENT_RUN_STALE_AFTER`（默认 10 分钟）年龄阈值作为接管回退条件。

接管不会中断正在进行的 HTTP 调用；旧 Worker 的迟到完成与接管者通过
Run 行锁、lease owner 校验与状态幂等收敛。M1-C-C 事件驱动化后，本机制
迁移到 V2 异步路径。

## M1 故障实验

1. 在模型步骤中杀死 Python，重启后从 Checkpoint 恢复。
2. 在事件返回前杀死 Go Worker，重复事件不重复推进状态。
3. 重复投递 Start/Resume Job，Run 只有一个有效 attempt。
4. 在等待审批时重启全部服务，仍能恢复。
5. 产生 ToolCall 后模拟超时，恢复路径先 Reconcile，不盲目重试。
6. 所有实验保持 Finance V1 Contract Regression 通过。
