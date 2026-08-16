# Connector Runtime

> 文档状态：Active M3 Target Specification
> 更新日期：2026-08-16

## 目的

以统一适配层连接企业数据库、ERP、工单、知识库和文件服务，隔离供应商 API、认证、Schema、分页、限流和错误差异。

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

## Binding

Connector Binding 必须限定：

- tenant、environment、connector/version。
- allowed capability 和 resource scope。
- credential_ref、network policy、rate limit。
- status、owner、rotation/expiry metadata。

同一 Connector 实现可以被多个 Tenant 使用，但 Binding、Credential、数据和 Cache 不能共享。

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

## M3 验收

- 供应商替换不修改 Runtime 核心。
- Schema Drift、限流、超时、Webhook 重复和部分成功测试通过。
- 每次外部请求关联 Run、Step、ToolCall 和 Audit。
- Secret 只通过 CredentialRef 使用。
- 真实写入之前具备 Kill Switch、Verify、Reconcile 和人工修复入口。
