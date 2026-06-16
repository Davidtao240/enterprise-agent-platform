# Domain Policy

## Purpose

Domain Policy constrains how business apps, graphs, agents, and tools can be combined. It prevents reusable agents from accidentally gaining access to tools or data from another business domain.

V1 uses finance as the first business domain. Future HR, legal, procurement, IT service, and customer service scenarios must each define their own domain policies.

## Core Concepts

### Business Domain

A business domain is represented by `business_app_code`, such as:

```text
finance
hr
legal
procurement
it_service
customer_service
```

### Graph Domain

Each Python Agent Graph belongs to one business app.

Example:

```text
finance_operating_report_graph -> finance
```

### Agent Domain

Agents can be domain-specific or shared.

Domain-specific examples:

```text
finance_analysis_agent -> finance
contract_risk_agent -> legal
resume_parse_agent -> hr
```

Shared examples:

```text
data_extract_agent -> shared
validation_agent -> shared
report_agent -> shared
```

### Tool Domain

Tools can also be domain-specific or shared.

Domain-specific examples:

```text
generate_finance_report -> finance
query_hr_policy -> hr
parse_contract -> legal
```

Shared examples:

```text
parse_csv -> shared
parse_excel -> shared
send_notification -> shared
```

## Policy Fields

```json
{
  "business_app_code": "finance",
  "allowed_agent_domains": ["finance", "shared"],
  "allowed_tool_domains": ["finance", "shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true
}
```

## Agent Rules

- Domain-specific agents can only run in the same business domain by default.
- Shared agents can run across domains only when `allow_shared_agents` is true.
- Shared agents must still obey tool permissions for the current business app.
- Agent capability reuse must not bypass domain policy.

Allowed:

```text
finance graph -> finance_analysis_agent
finance graph -> data_extract_agent
```

Not allowed:

```text
finance graph -> resume_parse_agent
hr graph -> finance_analysis_agent
```

## Tool Rules

- Domain-specific tools can only be used in the same business domain by default.
- Shared tools can be used across domains only when `allow_shared_tools` is true.
- Every tool call must have an active `agent_tool_permissions` record.
- Tool risk level must be checked before execution.

Allowed:

```text
finance graph -> report_agent -> generate_finance_report
finance graph -> data_extract_agent -> parse_csv
```

Not allowed:

```text
hr graph -> report_agent -> generate_finance_report
finance graph -> resume_parse_agent -> query_hr_policy
```

## High Risk Tool Rules

High-risk tools require explicit review when `high_risk_requires_review` is true.

Examples:

```text
archive_report
create_purchase_order
send_contract_to_counterparty
create_onboarding_task
```

V1 finance uses a human review node before archive. The actual archive action should only run after approval.

## Validation Order

Before running an Agent Graph:

1. Validate `business_app_code`.
2. Validate `workflow_template_key` belongs to `business_app_code`.
3. Validate `graph_key` belongs to the workflow template.
4. Validate graph registry status is active.
5. Validate domain policy status is active.
6. Validate current user can run the workflow.

Before an Agent uses a Tool:

1. Validate `agent_id` exists and is active.
2. Validate `tool_id` exists and is active.
3. Validate `agent_tool_permissions` allows the pair for the business app.
4. Validate agent domain is allowed by domain policy.
5. Validate tool domain is allowed by domain policy.
6. Validate shared agent/tool flags.
7. Validate `risk_level`.
8. Create approval task if high-risk action requires review.
9. Record audit log and agent run log.

## V1 Finance Policy

```json
{
  "business_app_code": "finance",
  "allowed_agent_domains": ["finance", "shared"],
  "allowed_tool_domains": ["finance", "shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true
}
```

Allowed finance agents:

```text
data_extract_agent
schema_mapping_agent
validation_agent
finance_analysis_agent
report_agent
review_summary_agent
```

Allowed finance tools:

```text
parse_csv
parse_excel
normalize_finance_schema
validate_finance_metrics
generate_finance_report
archive_report
```

## Future Domain Policy Pattern

When adding HR:

```json
{
  "business_app_code": "hr",
  "allowed_agent_domains": ["hr", "shared"],
  "allowed_tool_domains": ["hr", "shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true
}
```

When adding legal:

```json
{
  "business_app_code": "legal",
  "allowed_agent_domains": ["legal", "shared"],
  "allowed_tool_domains": ["legal", "shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true
}
```
