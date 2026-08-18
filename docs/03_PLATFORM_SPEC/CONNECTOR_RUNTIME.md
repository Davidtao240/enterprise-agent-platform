# Connector Runtime

> 文档状态：Active M3 Target Specification
> 更新日期：2026-08-18

## 目的

以统一适配层连接企业数据库、ERP、工单、知识库和文件服务，隔离供应商 API、认证、Schema、分页、限流和错误差异。

## 实现状态（2026-08-18）

| 能力 | 状态 | 位置 |
|---|---|---|
| connector_bindings + credential_secrets（内置 AES-256-GCM Secret Provider） | ✅ M2-D 已实现 | migration 020 / `internal/tool/connector_credential.go` |
| CredentialRef 解析边界（执行瞬间解密、审计零明文） | ✅ M2-D 已实现 | `internal/tool/connector_credential.go` |
| connector_registry 注册与版本治理 | ✅ M3-A 已实现 | migration 021 / `internal/tool/connector_registry.go` |
| Connector Contract Go 接口 | ✅ M3-A 已实现 | `internal/tool/connector_contract.go` |
| Mock Connector + `enterprise_db_read` 只读 | ✅ M3-A 已实现 | `internal/tool/connector_mock_db_read.go` |
| ConnectorRuntime.Execute 接入 ToolCall 执行路径（executing 自动执行 + 审批后执行） | ✅ M3-A 已实现 | `internal/tool/tool_service.go` / `tool_service_lifecycle.go` |
| `ticket_create_or_update`（幂等、乐观锁、状态验证） | ✅ M3-B 已实现 | `internal/tool/connector_ticket_mock.go` |
| webhook_events Inbox（HMAC 签名、去重、乱序恢复、失败重试） | ✅ M3-B 已实现 | migration 022 / `internal/tool/webhook_*.go` |
| `erp_purchase_request`（Dry-run preview、幂等创建、补偿撤销） | ✅ M3-C 已实现 | `internal/tool/connector_erp_mock.go` |
| connector_outbox（Outbox 投递、指数退避、stale Verify 收敛、Compensation） | ✅ M3-C 已实现 | migration 023 / `internal/tool/outbox_*.go` |
| OutboxConnector 契约（UseOutbox 自声明 + BuildCompensation） | ✅ M3-C 已实现 | `internal/tool/connector_contract.go` |

Secret Provider 抽象：内置加密是第一种实现（credential_ref 前缀 `secret:`）；未来接入 Vault 等外部 Provider（前缀 `vault:`）不改变 Binding 与 ToolCall 契约。

## Connector Contract

```text
manifest
capabilities
input/output schema
auth type and credential_ref
health_check
execute
verify
reconcile（需要时）
compensate（可安全补偿时）
```

Connector 不做 Agent 规划，不解释 Prompt，不自行扩大资源范围。

M3-A 以 Go 接口落地 Contract：`Manifest()` / `Capabilities()` / `HealthCheck(ctx)` / `Execute(ctx, req)` / `Verify(ctx, req)`。Connector 实现必须通过 connector_registry 注册，版本不可变。

## Binding

Connector Binding 必须限定：

- tenant、environment、connector/version。
- allowed capability 和 resource scope。
- credential_ref、network policy、rate limit。
- status、owner、rotation/expiry metadata。

同一 Connector 实现可以被多个 Tenant 使用，但 Binding、Credential、数据和 Cache 不能共享。

已实现部分（M2-D）：tenant、business_app_code、connector_code、config_json、credential_ref、status。
M3-A 补齐：environment（发布阶段门禁的机器可执行依据）、allowed_capabilities、connector_version。

## 企业数据库

- 优先企业 API、数据仓库、只读副本或治理视图。
- 禁止模型在生产主库执行任意 SQL。
- 使用 Query Template、参数化输入、表列白名单、LIMIT、timeout 和成本限制。
- 使用最小权限账号、RLS 和列遮蔽；禁止 Superuser/Table Owner/BYPASSRLS 身份。
- 写操作走受控 Command API 或明确存储过程。

## ERP

- 使用供应商发布并支持的 API/Event，不依赖内部表。
- Connector 完成 Canonical Schema Mapping 和版本适配。
- 写操作具备 idempotency、业务单据 ID、Verify 和对账。
- 跨系统流程采用 Saga/Outbox/Compensation，不假设分布式强事务。

## 工单

- 创建、更新、关闭、转派和评论分别授权。
- 本地 run/workflow id 作为外部关联 ID。
- 更新前读取当前状态，使用 ETag/Version 防止覆盖人工修改。
- Webhook 校验签名，Inbox 去重并处理乱序，周期性 Reconcile。

## 知识库与文件

- 索引和检索传播 Tenant、用户和资源 ACL。
- 文档版本、来源、有效期、删除和权限变化同步到索引。
- 内容视为不可信数据并进行 Prompt Injection 防护。

## 发布阶段

```text
Mock Fixture
→ Sandbox Read-only
→ Shadow
→ Human-approved Write
→ Limited Canary
→ Production Expansion
```

registry 的 `release_stage` 与 binding 的 `environment` 共同构成门禁：binding.environment 不低于 connector release_stage 所允许的阶段时，写类 capability 被拒绝。

## M3 子阶段拆分

| 子阶段 | 交付 | 验收对应 |
|---|---|---|
| M3-A | Connector Contract 接口 + connector_registry + binding 补列（environment/allowed_capabilities/version）+ Mock Connector + `enterprise_db_read` 只读 | 版本、能力、认证、健康检查、执行和验证契约 |
| M3-B | `ticket_create_or_update`（幂等、并发控制、状态验证）+ webhook_events Inbox + Reconcile | Webhook 重复、乱序、限流可恢复 |
| M3-C | `erp_purchase_request` Sandbox/Dry-run + connector_outbox + Compensation | 部分成功可恢复、外部请求全链路关联 |

## M3 验收

- 供应商替换不修改 Runtime 核心。
- Schema Drift、限流、超时、Webhook 重复和部分成功测试通过。
- 每次外部请求关联 Run、Step、ToolCall 和 Audit。
- Secret 只通过 CredentialRef 使用。
- 真实写入之前具备 Kill Switch、Verify、Reconcile 和人工修复入口。
