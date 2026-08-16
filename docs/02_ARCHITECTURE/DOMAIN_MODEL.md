# Domain Model

> 文档状态：Active Specification
> 更新日期：2026-08-16

## 模型分层

```text
Enterprise Control Plane
├── Identity / Tenant / RBAC
├── Business App / Workflow
├── Configuration / Policy / Approval / Audit
└── Connector / Credential Binding

Agent Runtime
├── Thread / Run / Step
├── Checkpoint / Interrupt
├── ToolCall
└── EvalRun
```

## 企业控制面对象

### User / Department / Role / Permission / Tenant

定义用户身份、组织、角色、权限和数据归属。客户端不能自行选择 `tenant_id`；每次 Runtime 和 Tool Call 从可信认证上下文继承。

### BusinessApp

业务入口与权限边界，例如 `finance`、`procurement`。BusinessApp 不是独立 Runtime。

### WorkflowTemplateVersion

版本化企业业务流程定义，描述节点、边和输入输出映射，通过 `graph_key`/graph version 显式绑定 Agent Graph。

### WorkflowInstance

一次企业流程执行。状态保持现有业务语义：`draft`、`running`、`waiting_review`、`approved`、`rejected`、`archived`、`failed`、`cancelled`。

### WorkflowNodeInstance

Workflow 节点实例。`agent_graph` 节点可以关联一个或多个 Run Attempt。

### AgentDefinitionVersion

Agent 的版本化定义：目标、能力、模型策略、输入输出 Schema、可用 Skill、Budget 和状态。`agent_id` 表示逻辑身份，version 表示不可变发布版本。

版本对象可以由通用 `configuration_versions` 承载，不要求为每一种配置复制一张版本表。`agent_registry` 等 Registry 继续作为能力发现和当前投影；Run 必须绑定 Published Configuration Version，而不是只记录可变 Registry 行。

### SkillVersion

可版本化的指令、Tool Binding、资源、约束和 Eval Fixture。Skill 不能扩大用户、Tenant 或 Agent 权限。

### ToolDefinitionVersion

定义单一受控能力：输入输出 Schema、风险等级、副作用类型、幂等策略、审批要求、Connector Capability 和状态。

### Connector

外部系统适配器逻辑身份及版本。记录类型、能力、认证方式、健康状态和受支持的 Tool Binding，不保存明文 Secret。

### CredentialRef

指向 Vault/KMS/Secret Manager 中凭证的引用，包含 Tenant、Connector、Environment 和 Scope 绑定。日志、Prompt、Checkpoint 不保存 Secret Value。

### DomainPolicyVersion / RuntimePolicyVersion

- Domain Policy：约束 Business App、Graph、Agent、Skill、Tool 和 Connector 的合法组合。
- Runtime Policy：约束轮次、Token、Cost、Deadline、Retry、Checkpoint、Memory 和 Tool Budget。

### ApprovalTask

人工决策点。高风险 Tool Approval 必须绑定：

- requester、reviewer scope 和职责分离规则。
- `run_id`、`tool_call_id`。
- 不可变 Payload 或 Payload Hash。
- Agent/Skill/Tool/Connector/Policy Version。
- 风险、理由、有效期和来源证据。

### AuditLog

不可缺失的企业行动记录。Audit 与 Debug Trace 不等价：Audit 记录治理事实，Trace 记录技术执行过程。

### File

源文件、生成文件和附件元数据。访问必须同时校验 Tenant、Workflow/Run 关联和调用服务身份。

## Runtime 对象

### Thread

一段连续任务或会话的容器。可以包含多个 Run，但不自动等于长期 Memory。

建议字段：

- `id`、`tenant_id`、`created_by`。
- `business_app_code`、`workflow_instance_id`（可选）。
- `status`、`title`、`created_at`、`updated_at`。

### Run

Agent 为完成一次目标而进行的持久执行。

状态：

```text
queued → running
running → waiting_human | waiting_external | succeeded | failed | cancelled
waiting_human | waiting_external → running | cancelled | failed
```

Run 必须快照：

- Tenant、Actor、Workflow/Node、Graph。
- Agent、Skill/Profile、Tool Policy、Model Config Version。
- Budget、Deadline、Attempt、Checkpoint Cursor。
- Parent Run（可选）和 Trace ID。

### Step

Run 中最小的可观测执行步骤。类型包括：`model`、`tool`、`checkpoint`、`interrupt`、`system`。

Step 是追加式记录；重试创建新 Attempt，不覆盖旧失败事实。

### ToolCall

一次受控工具调用，记录：

- Trusted Context、Tool/Connector Version。
- Input Hash/Redacted Summary。
- Policy Decision、Risk、Approval。
- Idempotency Key、External Request/Object ID。
- Execute、Verify、Reconcile/Compensate 状态。

### Checkpoint

Runtime 可恢复状态的引用和元数据。大体积 Graph State 可由 Python Checkpointer 保存，Go 保存受治理索引、版本和关联。

### Interrupt

暂停 Run 等待人工、补充资料或外部事件。Interrupt 必须有原因、恢复 Schema、到期时间和唯一恢复 Token/版本，重复 Resume 必须幂等。

### AgentRunLog

现有兼容摘要，用于列表、观测和 Finance V1 API。M1 后由 Run/Step 聚合生成，不作为 Checkpoint 或执行状态的唯一来源。

### EvalRun

对 Run 的任务结果、过程、安全、可靠性和成本进行评估。应引用固定 Dataset/Evaluator Version，并与被评估 Run 分离。

## 核心不变量

1. Workflow 状态和 Run 状态不混用。
2. 历史 Run 能定位所有不可变配置版本。
3. ToolCall 身份不信任模型自报字段。
4. Checkpoint 恢复不等于外部副作用未发生；恢复前必须 Reconcile。
5. Shared Code、Skill 和 Memory 不代表 Shared Data。
6. Agent 不直接写平台数据库或企业系统。
