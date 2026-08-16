# Tool Execution Runtime

> 文档状态：Active M2 Target Specification
> 更新日期：2026-08-16

## 目的

将 Tool Registry 从“能力目录”升级为可信执行边界。本文不表示 M2 已经实现。

## 执行链

```text
Python Runtime candidate Tool Request
→ load trusted Run Snapshot
→ validate Tool Schema/Version
→ authorize User/Tenant/Agent/Skill/Resource
→ evaluate Risk and Approval
→ check Idempotency
→ resolve Connector Binding and CredentialRef
→ execute Connector
→ verify external state
→ persist ToolCall / Trace / Audit
```

## 信任边界

模型可以提供：

- 候选 `tool_id`、version、arguments。
- 业务理由和证据引用。

模型不能提供或覆盖：

- 用户、Tenant、Agent、Skill、Workflow 身份。
- Credential、Endpoint、Policy Version。
- Risk Level、Approval Requirement。
- 已经成功的执行状态。

## 风险等级

| 类型 | 默认策略 |
|---|---|
| Pure transform | Schema 校验后执行 |
| Read | ACL、范围、限流、数据最小化 |
| Low-risk write | 幂等、权限、Verify |
| High-risk write | 不可变 Payload 审批 + 执行前重检 + Verify |
| Prohibited | 确定性拒绝，不可由 Prompt 解除 |

## 幂等与不确定结果

- Side-effect Tool 必须提供 Tenant-scoped idempotency key。
- 超时不等于失败；先根据 external request/object id Verify。
- 无法确定时标记 `indeterminate`，进入 Reconcile 或人工处理。
- 只有确认未执行且 Policy 允许时才能重试。

## 可靠性

- 每种 Tool 定义 timeout、retry、backoff 和 circuit breaker。
- 只读/确定幂等操作可以自动重试。
- 失败进入 DLQ，重放仍经过授权、幂等和版本校验。
- Run 恢复时使用已有 ToolCall 记录，不能创建新副作用来“试试看”。

## 审计

记录 actor/service、tenant、run/step/toolcall、所有配置版本、input hash、Policy Decision、Approval、Connector、external id、Verify 和 Error。日志只保存脱敏摘要。

## M2 验收

- 未授权、跨域、跨 Tenant、模型伪造身份全部拒绝。
- 同一 idempotency key 的重复调用不会产生重复记录。
- 审批 Payload 与执行 Payload 完全一致。
- Timeout/Indeterminate 先 Reconcile。
- Secret 不进入数据库业务列、Prompt、Trace 或错误响应。
