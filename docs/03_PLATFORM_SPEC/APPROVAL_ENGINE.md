# Approval Engine

> 文档状态：Active Specification
> 更新日期：2026-08-16

## Purpose

Approval is a platform capability, not a finance-only feature. Finance reports, HR onboarding material review, procurement requests, legal contract review, IT change requests, and customer service escalations can all reuse the same Approval Engine.

## Approval Task

An `ApprovalTask` represents one human decision point in a workflow.

It is usually created by a `human_review` workflow node or by high-risk tool policy.

## Approval Status

| Status | Meaning |
|---|---|
| pending | Waiting for human decision |
| approved | Approved by reviewer |
| rejected | Rejected by reviewer |
| cancelled | Cancelled because workflow was cancelled |
| expired | Expired after timeout |

## Creation Rules

Approval task is created when:

- Workflow reaches a `human_review` node.
- Domain policy requires review for a high-risk action.
- Template edge or node configuration explicitly requires manual confirmation.

Required fields:

```text
workflow_instance_id
node_instance_id
business_app_code
title
status
assignee_role or assignee_user_id
```

## Reviewer Matching

V1 supports role-based reviewer matching.

Example:

```json
{
  "node_type": "human_review",
  "role": "business_reviewer"
}
```

Rules:

- If `assignee_user_id` is present, only that user can decide.
- If `assignee_role` is present, any user with that role can decide.
- User must also have `approval:decide` permission.
- A user should not approve their own task when separation of duties is enabled in future versions.

## Approval Action

Approve request:

```json
{
  "comment": "Approved."
}
```

Effects:

1. Approval task status becomes `approved`.
2. `decision_by`, `decision_comment`, and `decided_at` are saved.
3. Related workflow node status becomes `succeeded`.
4. Workflow Engine evaluates outgoing edges with `when = approved`.
5. Audit log is written.

## Rejection Action

Reject request:

```json
{
  "comment": "Please correct revenue data."
}
```

Effects:

1. Approval task status becomes `rejected`.
2. Rejection comment is required.
3. Related workflow node status becomes `failed` or `succeeded` according to template design.
4. Workflow Engine evaluates outgoing edges with `when = rejected`.
5. V1 finance workflow ends as `rejected`.
6. Audit log is written.

## High-Risk Tool Approval

When `domain_policies.high_risk_requires_review` is true, high-risk actions cannot silently execute.

Examples:

```text
archive_report
create_purchase_order
send_contract_to_counterparty
create_onboarding_task
```

V1 finance handles this through an explicit `human_review` node before archive.

Future versions may support dynamic approval tasks created directly by high-risk tool requests.

M2 将该能力升级为 Tool Gateway 的正式执行 Gate。审批任务必须绑定：

```text
run_id
tool_call_id
requester_user_id / service_identity
tenant_id
tool_id / tool_version
connector_binding_id
policy_version
payload_hash
redacted_payload
risk_level / reason / evidence
expires_at
```

规则：

- Reviewer 必须看到将要执行的确定 Payload，而不只是 AI 摘要。
- Approval 只能授权被展示并 Hash 绑定的 Payload；审批后模型不得重新生成参数。
- Approval 有效期结束、权限变化、资源版本变化或外部状态变化时，执行前必须重新校验。
- Requester 和 Approver 的职责分离由 Go 强制，不能交给 Prompt。
- 重复 Approve/Resume 必须幂等。
- Approval 不直接证明外部动作完成；Connector 执行后仍需 Verify。

## Time-of-check / Time-of-use 防护

审批发生在执行之前，两者之间状态可能变化。Tool Gateway 在真正执行时重新检查：

- 审批仍有效且 Payload Hash 一致。
- 用户、Reviewer、Tenant 和资源权限仍有效。
- 预算、库存、价格、工单版本等业务前置条件仍成立。
- Connector Binding 和 Credential 没有被禁用或轮换为不兼容 Scope。

若条件变化，返回新的 Interrupt 或要求重新审批，不静默执行旧决定。

## Approval Task Visibility

A reviewer can see a task when:

- Task status is `pending`.
- The reviewer has the required role or is the assigned user.
- The reviewer has access to the business app.

Admins and ops viewers may read tasks for troubleshooting if they have proper permissions.

## Audit Requirements

Audit logs must be written for:

- Approval task creation.
- Approval task approval.
- Approval task rejection.
- Approval task cancellation.
- Approval task expiration.

Audit detail should include:

```json
{
  "approval_task_id": "approval_001",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_003",
  "decision": "approved",
  "comment": "Approved."
}
```

## V1 Finance Approval Example

Finance workflow:

```text
upload
-> agent_graph
-> human_review
-> archive
```

Review payload shown to finance manager:

```json
{
  "workflow_title": "2026-05 Operating Data Report",
  "summary": "Revenue increased by 8.2%.",
  "warnings": [
    "Cost growth is higher than revenue growth."
  ],
  "key_metrics": {
    "revenue": 1200000,
    "cost": 760000,
    "net_profit": 310000
  },
  "report_file_id": "file_099"
}
```

Approve:

```text
human_review -> archive -> workflow archived
```

Reject:

```text
human_review -> workflow rejected
```
