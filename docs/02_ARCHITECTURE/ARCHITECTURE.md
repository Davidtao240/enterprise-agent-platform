# Architecture

> 文档状态：Active Specification
> 更新日期：2026-08-16
> 当前实现完成到 Finance V1 + Shared Core；Durable Run、Tool Execution Gateway 和 Connector 为目标能力，必须按 Roadmap Gate 实现。

## 总体架构

```mermaid
flowchart LR
    U["员工 / 经理 / 管理员"] --> W["React Enterprise Workbench"]
    W --> C["Go Control Plane<br/>Auth、RBAC、Workflow、Approval、Audit"]
    C --> R["Python Agent Runtime<br/>Graph、Loop、Checkpoint、Interrupt"]
    R --> G["Go Tool Execution Gateway<br/>Policy、Approval、Idempotency、Audit"]
    G --> X["Connector Runtime"]
    X --> D["Database / ERP / Ticket / Knowledge"]

    P["Tenant / Policy / Secret"] --> C
    P --> G
    C --> O["Trace / Eval / Replay"]
    R --> O
    G --> O
```

当前已实现的主链仍是：

```text
React
→ Go Workflow + Asynq Worker
→ Go Agent Gateway
→ Python FastAPI + Finance LangGraph
→ Go 持久化节点结果、审批、归档和审计
```

目标架构在此基础上增加 Durable Run、Tool Gateway、Connector 和 Trace/Eval，不通过重写现有 Finance 主链获得。

## 架构原则

1. Go 是企业身份、Workflow、Approval、业务状态、Policy 和 Audit 的 System of Record。
2. Python 是 Agent Runtime，负责 Graph、模型调用、Context 和 Checkpoint；不能独立改变企业业务最终状态。
3. Workflow Template 通过 `graph_key` 显式路由 Graph；LLM 不自主选择业务域。
4. Workflow 管企业业务流程，Agent Run 管智能体内部执行，两套状态机分离但可追踪关联。
5. Graph 按流程隔离，Agent 按能力复用，Tool 按权限隔离，Domain Policy 约束组合关系。
6. Tool Registry 只描述能力；所有企业系统调用经过 Tool Execution Gateway。
7. Connector 隔离供应商协议、认证和 Schema 差异；Runtime 核心不依赖 SAP、ServiceNow 或具体数据库。
8. 外部内容是不可信数据，不能覆盖系统指令或 Policy。
9. 所有关键状态和副作用都可审计、可验证、可恢复、可评估。

## 服务职责

### React Workbench

- 登录、业务入口和授权能力发现。
- Workflow/Run 时间线、节点、Step、Checkpoint 和等待原因。
- 人工审批时展示原始资料、Agent 依据、风险和不可变执行 Payload。
- Trace、Tool Call、外部请求、错误、恢复、成本和 Eval 结果。
- 不直接调用 Python Runtime 或企业 Connector。

### Go Control Plane

- Auth、User、Department、Role、Permission、Tenant。
- Business App、Workflow Template、Instance、Node Instance。
- Thread/Run 企业索引及其与 Workflow 的关联。
- Agent/Skill/Tool/Connector/Policy 配置版本治理。
- Agent Runtime Gateway、Tool Execution Gateway。
- Approval、Audit、File、CredentialRef、Trace Index、Eval Result。
- Redis/Asynq 作业、lease、heartbeat、retry、DLQ 和恢复协调。

Go 核心必须保持业务领域中立，不包含 Finance/Procurement 专用条件分支。

### Python Agent Runtime

- 根据可信的 `graph_key` 加载明确版本的 Graph。
- 执行 Agent Loop、模型调用、结构化输出和局部任务规划。
- Context Builder、Skill/Profile 绑定和 Runtime Budget。
- 持久化 Checkpoint，产生 Interrupt，接受 Resume/Cancel。
- 向 Go Tool Execution Gateway 发起结构化 Tool Call。
- 返回 Runtime Event 和结果，不直接写平台业务表。

### Tool Execution Gateway

- 从可信 Run Context 获取用户、Tenant、Agent、Skill、Graph 和 Workflow 身份。
- 校验 Tool Schema、版本、Agent-Tool Binding、Domain Policy 和资源范围。
- 按风险等级执行、拒绝或创建审批。
- 管理 idempotency、timeout、retry、circuit breaker、CredentialRef 和 Audit。
- 调用 Connector 后执行 `verify`；必要时进入 `reconcile` 或 `compensate`。

### Connector Runtime

- 封装数据库、ERP、工单、知识库和文件系统协议。
- 管理认证、分页、限流、字段映射、版本和错误归一化。
- 公开 `health_check`、`execute`、`verify`，必要时公开 `compensate`。
- 不做 Agent 规划，也不自行扩大权限。

## 两类 Gateway

### Agent Runtime Gateway

方向：Go → Python。

职责：Start、Resume、Cancel、查询 Run 状态和接收 Runtime Event。

### Tool Execution Gateway

方向：Python → Go → Connector。

职责：对每次 Tool Call 授权、审批、执行、验证和审计。

两者不能混成一个“Go 调 Python 的 HTTP Client”。

## Workflow 与 Agent Run

```text
Workflow Instance
└── Workflow Node Instance: agent_graph
    └── Agent Run
        ├── Model Step
        ├── Tool Step
        ├── Checkpoint
        ├── Interrupt / Approval
        └── Final Step
```

- Workflow 可以包含文件、人工审批和系统归档等非 Agent 节点。
- 一个 Workflow Node Retry 可以创建新的 Run Attempt。
- 一个 Run 可以产生多个 Step 和 Checkpoint。
- `agent_run_logs` 是兼容摘要，不再承担完整 Runtime 状态。

## 状态权威边界

| 状态 | 权威来源 |
|---|---|
| 用户、Tenant、权限、Workflow、Approval、Audit | Go + PostgreSQL |
| Run 企业索引、配置版本、ToolCall/外部请求关联 | Go + PostgreSQL |
| Graph 内部 State、模型消息、局部游标、Checkpoint | Python Checkpointer |
| 外部工单、ERP 单据、企业数据库事实 | 对应企业系统，通过 Connector verify |

Go 与 Python 都不得把自身缓存视为外部业务事实。恢复执行前必须根据 ToolCall 和 external request id 进行 Reconcile。

## 目标调用链

```text
User
→ Go 创建 Workflow/Run
→ Asynq 调度 Run Start
→ Python 从 Checkpoint 执行 Graph
→ 模型产生 Tool Call
→ Go Tool Gateway 授权/审批
→ Connector 调企业系统
→ verify 外部真实状态
→ Python 保存 Checkpoint 并继续
→ Go 更新 Run/Workflow 摘要和 Audit
→ Eval 判断业务结果与过程可靠性
```

## 扩展原则

新增业务通过以下组合接入：

```text
Business App
+ Workflow Template Version
+ Explicit Graph Version
+ Agent Definition / Skill / Domain Profile
+ Tool Definition / Connector Binding
+ Domain and Runtime Policy
+ Eval Fixture
```

不得新增独立的 `procurement_runtime`、`hr_tool_gateway` 或 `legal_approval_engine`。
