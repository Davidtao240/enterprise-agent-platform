# Agent Protocol

> 文档状态：Active Versioned Protocol
> 更新日期：2026-08-16

## 目标

定义 Go Control Plane、Python Agent Runtime、Agent Definition、Skill/Profile、Tool 和 Connector 的稳定关系。协议跨业务通用，Finance V1 是第一个兼容实现。

业务路由保持：

```text
Workflow Template Version
→ explicit graph_key / graph_version
→ Python Graph Registry
→ Agent Runtime
```

LLM 不自行决定进入 Finance、Procurement、HR 或其他业务域。

## 注册对象

### Agent Definition Version

```json
{
  "agent_id": "finance_analysis_agent",
  "version": "1.0.0",
  "domain": "finance",
  "reusable_scope": "domain_only",
  "capabilities": ["metric_analysis", "risk_explanation"],
  "input_schema": {},
  "output_schema": {},
  "runtime_policy_ref": "runtime_policy@1.0.0",
  "status": "published"
}
```

### Skill / Domain Profile Version

```json
{
  "skill_id": "finance_operating_report_profile",
  "version": "1.0.0",
  "domain": "finance",
  "instructions_ref": "artifact://...",
  "tool_bindings": [],
  "resource_refs": [],
  "eval_fixture_refs": [],
  "status": "published"
}
```

当前 Finance Profile 可以作为 Skill 体系的前置实现，但在治理模型落地前不应虚构其已具备完整 Skill 生命周期。

### Tool Definition Version

```json
{
  "tool_id": "enterprise_db_read",
  "version": "1.0.0",
  "domain": "shared",
  "risk_level": "medium",
  "side_effect": "read",
  "input_schema": {},
  "output_schema": {},
  "connector_capability": "database.query_template",
  "idempotency_policy": "read_repeatable",
  "status": "published"
}
```

Tool Registry 不保存 Secret，也不直接证明某次调用已授权。

## Agent 分类与复用

- `domain_only`：包含真正的领域推理，例如 FinanceAnalysisAgent。
- `shared`：业务领域中立的执行 Core，例如 DataExtract、SchemaMapping、Validation、Report、ReviewSummary。

Shared Agent 的正确结构：

```text
Shared Core
+ Versioned Domain Profile / Skill
+ Explicit Domain Graph
+ Per-call Tool Policy
```

Finance V1 当前边界：

| Agent | domain | reusable_scope | 领域绑定 |
|---|---|---|---|
| DataExtractAgent | shared | shared | 通用解析 + Finance Demo Fallback Profile |
| SchemaMappingAgent | shared | shared | Finance Schema Mapping Profile |
| ValidationAgent | shared | shared | Finance Validation Profile |
| FinanceAnalysisAgent | finance | domain_only | Finance Domain Logic |
| ReportAgent | shared | shared | Finance Report Profile |
| ReviewSummaryAgent | shared | shared | Finance Review Summary Profile |

## Run 绑定

每次 Run 必须快照：

- protocol version。
- workflow template、graph、agent definition。
- profile/skill、model config、runtime policy。
- 允许使用的 Tool Binding/Policy Version。
- Tenant、Actor、Business App、Budget 和 Deadline。

Published Version 不可原地修改。历史 Run 不自动升级。

## Tool 调用协议

Agent/模型只生成 `tool_id`、version、arguments 和 tool_call_id。Go Tool Execution Gateway 从 Run Snapshot 获取可信身份并执行：

```text
Schema
→ Agent/Skill Binding
→ Tenant/Domain/Resource Policy
→ Risk / Approval
→ Idempotency
→ Connector Execute
→ Verify
→ Trace / Audit
```

Agent 不直接获得 Credential，也不能用模型输出覆盖身份和风险策略。

## Runtime 控制

协议支持：

- Start、Interrupt、Resume、Cancel。
- Checkpoint Version 和幂等恢复。
- Parent Run/Child Run（后续可选）。
- Step、ToolCall 和 Runtime Event。
- Max Step、Token、Cost、Deadline 和 Tool Budget。

具体 Envelope 见 [`AGENT_IO_CONTRACT.md`](AGENT_IO_CONTRACT.md)。

## 扩展约束

新增 Agent/Skill/Tool 必须：

- 注册不可变版本并经过治理发布。
- 声明 Domain、Schema、Capability、Risk 和 Runtime Policy。
- 绑定允许的 Graph/Business App/Tenant Scope。
- 提供 Contract Fixture 和至少一个失败案例。
- 不在平台核心引入领域条件分支。
- 不绕过 Tool Gateway、Approval 和 Audit。
- Breaking Change 使用新 Major Version。

新增业务不要求新建业务专用 Agent API；如果通用 Runtime 协议需要演进，则通过版本升级保持兼容。
