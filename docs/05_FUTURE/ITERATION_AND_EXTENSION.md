# Iteration and Extension

> 文档状态：Active Extension Rules
> 更新日期：2026-08-16
> 上位路线：[`ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md`](ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md)

## 核心原则

后续扩展不在平台核心堆业务 `if/else`，也不以复制更多业务 Agent 为目标。扩展围绕以下版本化扩展点进行：

- Business App。
- Workflow Template。
- Agent Definition Version。
- Skill / Domain Profile Version。
- Tool Definition Version。
- Connector Binding。
- Domain/Runtime Policy 与 Tenant Binding。
- Eval Fixture 与 Result Contract。

继续保持：

```text
Workflow Template 显式路由 Graph
Graph 按流程隔离
Agent 按能力复用
Tool 按权限隔离
Domain Policy 做业务域约束
企业系统通过 Connector 接入
所有副作用通过 Tool Execution Gateway
```

## 平台 Gate

新增完整业务场景前必须满足：

1. Finance V1 Regression 持续通过。
2. 需要长任务或人工中断时，M1 Durable Run 已完成。
3. 需要企业数据或外部动作时，M2 Tool Execution Gateway 已完成。
4. 对应 M3 Connector 已在 Mock/Sandbox 完成契约和故障测试。
5. Tenant、File、Memory、Tool 和 Credential 作用域明确。
6. 至少具备任务结果、安全和副作用 Eval。

## 各层扩展清单

### 数据与配置

新增业务通常新增或绑定：

- Business App 和 Workflow Template Version。
- Graph、Agent、Skill/Profile、Tool Version。
- Domain Policy、Runtime Policy 和 Tenant Binding。
- Connector、CredentialRef 和 Canonical Schema Mapping。
- Eval Dataset、Contract Fixture 和独立 Expected Result。

领域数据可以先使用受版本化 Schema 约束的 JSONB。业务语义成熟后再拆专用表，不能把 JSONB 当作永久无契约存储。

### Go Control Plane

优先复用：

- Workflow、Run Index、Approval、Policy、Audit。
- Agent Runtime Gateway 和 Tool Execution Gateway。
- Connector Registry、CredentialRef、Configuration Governance。
- Trace Index、Eval Result 和 File Service。

新增业务只允许增加领域 Adapter、Mapping、Policy/Skill 绑定和 Result Assembler，不新增独立 Workflow Engine、Tool Runtime 或 Approval Engine。

### Python Runtime

- Graph 通过显式 `graph_key` 绑定。
- Shared Core 通过 Versioned Domain Profile/Skill 注入业务规则。
- Runtime 必须支持 Checkpoint、Interrupt、Resume、Cancel 和 Budget。
- Python 不直接写平台业务状态，不绕过 Go 调用企业系统。

### Tool 与 Connector

- Tool 定义能力、Schema、风险和执行策略。
- Connector 处理供应商协议、认证、版本、分页、限流和错误映射。
- 一次 Tool Call 的最终权限是 User、Tenant、Agent、Skill、Tool Policy 和 Resource ACL 的交集。
- 写操作必须定义 idempotency、verify；跨系统流程还需 reconcile/compensate。

### 前端

复用目标入口、Run 时间线、审批、Trace、Audit、Eval 和能力管理页面。业务只新增必要输入表单和结果视图，不以复制一套独立工作台为默认方案。

## 协议演进规则

- 已发布协议不能原地做 Breaking Change。
- 向后兼容字段可以在同一协议版本中增加。
- Breaking Schema Change 必须升级 Major Version，并提供迁移与兼容窗口。
- “新增业务不修改协议”改为“新增业务不要求业务专用协议”；Runtime 能力升级可以通过新版本演进通用协议。
- 历史 Run 必须能够定位当时使用的 Template、Graph、Agent、Skill、Tool、Policy、Connector 和模型版本。

## 扩展验收标准

新增一个业务场景应满足：

- 不修改核心 Workflow Engine、Durable Runtime、Tool Gateway 和 Approval Engine 的业务无关逻辑。
- 不引入领域专用平台 API；使用版本化通用契约。
- Agent 无法访问未授权 Tool、Connector、File、Memory 或其他 Tenant 数据。
- 高风险动作审批绑定不可变 Payload，执行前重新校验。
- 重复消息、Retry 和 Resume 不产生重复副作用。
- 外部动作经过 verify，不能仅凭模型文字判定成功。
- Trace 自动关联 Workflow、Run、Step、ToolCall、Approval 和外部请求。
- Contract、故障、安全和业务结果 Eval 通过。
- 先 Mock/Sandbox/Read-only，再 Shadow、Human-approved Write 和 Limited Canary。
