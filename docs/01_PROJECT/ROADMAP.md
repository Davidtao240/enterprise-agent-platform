# Roadmap

> 文档状态：Active Roadmap
> 更新日期：2026-08-18
> 目标依据：[`../05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md`](../05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md)

## 使用方式

本文件决定当前实施顺序。详细对象、API 和数据契约以 `02_ARCHITECTURE/`、`03_PLATFORM_SPEC/` 为准；Finance V1 外部行为以 `04_V1_FINANCE/` 和回归 Fixture 为准。

## 历史阶段：V1 平台骨架（已完成）

### Phase 0–1：项目骨架与身份权限

- Monorepo、Docker Compose。
- Auth、User、Department、Role、Permission、RBAC。

### Phase 2：Workflow Core

- Workflow Template、Instance、Node Instance。
- 状态机、Asynq 异步任务、模板解释执行。
- `graph_key` 显式路由。

### Phase 3：Registry、Gateway 与治理

- Agent、Graph、Tool Registry。
- Agent Gateway、Agent Run Log、Domain Policy。
- Approval、Audit、Configuration Governance。

### Phase 4–6：Finance V1、Workbench 与可观测性

- Finance Agent Graph 和端到端流程。
- 文件、报告、人工审批、归档和审计。
- Token/Cost、平台汇总、演示和质量闸门。

以上阶段是现有能力和兼容基线，不代表最终 Agent Runtime 已完成。

## 过渡阶段：契约与 Shared Core（第一批已完成，待验收）

- Finance Contract / Regression Fixture。
- Shared Core 与 Versioned Finance Profile。
- Python/Go Agent Domain Metadata 对齐。
- Finance Graph 显式绑定 Profile，保持 V1 契约兼容。

本阶段完成不代表 Procurement Phase B 或真实企业集成取得准入。

## M1：Durable Agent Run（已完成）

目标：把一次性 Graph 调用升级为可持久化、可恢复、可取消的 Agent 执行。

交付：

- Thread、Run、Step、Checkpoint、Interrupt 数据模型。
- Run Start、Resume、Cancel、Event 协议。
- Python 持久化 Checkpointer。
- Worker lease、heartbeat、attempt 和失联接管语义。
- Run 绑定不可变的 Graph、Agent、Profile/Skill 和模型配置版本。
- Workflow 状态机与 Agent Run 状态机明确分离和关联。

完成 Gate：

- 执行中终止 Python 后可以从正确 Checkpoint 恢复。
- 执行中终止 Go Worker 后不会重复完成。
- 重复 Resume、Retry 或队列消息不会造成重复副作用。
- 等待审批期间重启服务，审批后仍能继续。
- Finance V1 回归保持通过。

## M2：Tool Execution Gateway（已完成）

目标：所有企业系统访问和副作用都经过可信的 Go 执行边界。

交付：

- Tool Definition Version 和 Tool Execution Contract。
- 调用级身份、Tenant、Domain、资源范围和风险校验。
- 不可变审批 Payload、幂等、超时、重试、熔断和 DLQ。
- CredentialRef 与 Secret 使用边界。
- Tool Call、Policy Decision、Approval、Result、Verify 的 Trace/Audit。

完成 Gate：

- 模型无法伪造身份或绕过网关。
- 未授权和跨 Tenant Tool Call 确定性拒绝。
- 重复调用不产生重复外部副作用。
- 审批内容和实际执行内容一致。

子阶段（M2-A Runtime 桥接、M2-B Tool Call 生命周期、M2-C 超时/熔断/DLQ、M2-D CredentialRef 边界、M2-E 全链路 Trace）均已完成并通过审查。

## M3：Connector Runtime（当前主线）

目标：以供应商无关契约接入真实企业系统。

首批能力：

- `enterprise_db_read`：治理后的只读视图。
- `ticket_create_or_update`：幂等、并发控制、审批和状态验证。
- `erp_purchase_request`：Sandbox/Dry-run 优先。

子阶段：

- M3-A：Connector Contract Go 接口、connector_registry 注册与版本治理、Binding 补列（environment/allowed_capabilities/connector_version）、Mock Connector、`enterprise_db_read` 只读、ConnectorRuntime.Execute 接入 ToolCall 执行路径。Gate：版本、能力、认证、健康检查、执行和验证契约可用。（已完成）
- M3-B：`ticket_create_or_update`（幂等键去重、expected_version 乐观锁、closed 终态拒绝）、webhook_events Inbox（HMAC-SHA256 签名、(connector_code, external_event_id) 唯一去重、occurred_at 乱序恢复、失败重试）、WebhookConsumer 周期消费。Gate：Webhook 重复与乱序可恢复。（已完成）
- M3-C：`erp_purchase_request` Sandbox（Dry-run preview、幂等创建、补偿撤销）、connector_outbox（OutboxConnector 契约自声明、指数退避重试、stale Verify 收敛）、Compensation（自动补偿 + 人工治理端点）。Gate：部分成功可恢复，外部请求全链路关联。（已完成）

完成 Gate：

- Connector 具备版本、能力、认证、健康检查、执行和验证契约。
- 限流、超时、Schema Drift、Webhook 重复和部分成功可恢复。
- 外部请求可关联到本地 Run、Step、ToolCall 和 Audit。

## M4：Context、Skill 与 Memory

- Context Builder、来源、ACL、裁剪和预算。
- Skill Version 及 Draft/Review/Published/Deprecated 生命周期。
- Run State、Thread Memory、User Memory 和 Team/Domain Memory 分层。
- 写入、读取、过期、删除、敏感数据和跨 Tenant 隔离策略。

## M5：Trace、Eval 与 Replay

- Workflow → Run → Model Turn → Tool Call → Checkpoint → Interrupt 层次 Trace。
- Tool 选择、参数、副作用、权限、成本和业务结果 Eval。
- 失败重放、Regression、Shadow、Canary 和人工修改反馈。

## M6：Enterprise Workbench 与受控路由

- 统一目标入口、授权能力发现和 Run 时间线。
- 展示等待原因、审批 Payload、Tool Call、外部请求和恢复记录。
- Router 只能在用户授权的 Business App、Agent、Skill 和 Tool 范围内选择。
- 不建设拥有全部权限的“万能主 Agent”。

## M7：业务场景扩展

只有相关平台 Gate 通过后，才按顺序进入完整 Procurement、HR、Legal、IT Service 或 Customer Service 场景。

每个新场景必须：

- 复用 Workflow Engine、Durable Runtime、Tool Gateway、Connector、Approval、Audit 和 Eval。
- 不在 Go 平台核心增加业务条件分支。
- 提供版本化 Domain Profile/Skill、契约 Fixture 和独立预期结果。
- 先 Mock/Sandbox/Read-only，再 Shadow、人工审批写入和 Limited Canary。

## 当前明确不做

- 直接开发多个完整业务 Agent。
- 无 Durable Run 的长期任务。
- 无 Tool Gateway 的真实数据库、ERP 或工单写入。
- 任意 SQL、HTTP、Shell、Filesystem 或全权限 Tool。
- 无 Eval 目标的复杂多 Agent 扩张。
- 大规模前端视觉重构。
