# ADR-005: Connector 与 CredentialRef

> 状态：Accepted
> 日期：2026-08-16

## 决策

以 Connector 封装数据库、ERP、工单和知识库的供应商差异。平台数据库只保存 CredentialRef；Secret Value 由受控 Secret Manager 在执行时提供。

Connector 至少实现 `health_check`、`execute`、`verify`，需要补偿的写能力再实现 `compensate`。

## 后果

- Agent、Skill 和 Prompt 不保存企业凭证。
- Connector Binding 按 Tenant、Environment、Capability 和 Scope 限定。
- 供应商 API 升级通过 Connector Version 和 Contract Test 隔离。
