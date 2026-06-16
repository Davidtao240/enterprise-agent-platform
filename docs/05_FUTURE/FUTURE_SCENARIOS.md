# Future Scenarios

## Purpose

This document records planned business scenarios after V1 finance. These scenarios are not implemented in V1, but the platform architecture must support them without rewriting Workflow Engine, Agent Gateway, Approval Engine, Audit Log, or Agent Run Log.

Each scenario must be added through:

- Business App.
- Workflow Template.
- Graph Registry.
- Agent Registry.
- Tool Registry.
- Agent Tool Permissions.
- Domain Policy.
- Business form schema.
- Optional business-specific result view.

## Scenario Implementation Rule

When adding a new scenario, create dedicated scenario documents:

```text
<SCENARIO>_AGENT_GRAPH.md
<SCENARIO>_WORKFLOW_TEMPLATE.md
<SCENARIO>_VIEW_SPEC.md
<SCENARIO>_SAMPLE_DATA.md
```

Examples:

```text
HR_AGENT_GRAPH.md
HR_WORKFLOW_TEMPLATE.md
LEGAL_AGENT_GRAPH.md
LEGAL_WORKFLOW_TEMPLATE.md
```

## HR Onboarding Material Review

Business app:

```text
business_app_code: hr
```

Workflow:

```text
workflow_template_key: hr_onboarding_review
graph_key: hr_onboarding_review_graph
```

Draft flow:

```text
upload_materials
-> agent_graph
-> human_review
-> create_onboarding_task
```

Planned agents:

```text
ResumeParseAgent
MaterialCheckAgent
PolicyCheckAgent
OnboardingNoticeAgent
```

Planned tools:

```text
parse_resume
query_position
query_hr_policy
create_onboarding_task
send_notification
```

Approval point:

- HR reviewer confirms material completeness and policy compliance.
- High-risk action: create onboarding task.

## Legal Contract Review

Business app:

```text
business_app_code: legal
```

Workflow:

```text
workflow_template_key: contract_review
graph_key: contract_review_graph
```

Draft flow:

```text
upload_contract
-> agent_graph
-> legal_review
-> archive_review_result
```

Planned agents:

```text
ContractExtractAgent
ClauseRiskAgent
PolicyRAGAgent
LegalReportAgent
```

Planned tools:

```text
parse_contract
query_legal_policy
compare_clause_template
generate_contract_risk_report
```

Approval point:

- Legal reviewer confirms risk findings.
- High-risk clauses require explicit review.

## Procurement Request Approval

Business app:

```text
business_app_code: procurement
```

Workflow:

```text
workflow_template_key: procurement_request
graph_key: procurement_request_graph
```

Draft flow:

```text
submit_request
-> agent_graph
-> budget_review
-> create_purchase_order
```

Planned agents:

```text
RequirementParseAgent
SupplierCompareAgent
BudgetCheckAgent
PurchaseOrderAgent
```

Planned tools:

```text
query_supplier
compare_quote
query_budget
create_purchase_order
```

Approval point:

- Procurement manager reviews supplier comparison.
- Finance reviewer reviews budget if amount exceeds threshold.
- High-risk action: create purchase order.

## IT Service Ticket Handling

Business app:

```text
business_app_code: it_service
```

Workflow:

```text
workflow_template_key: incident_ticket
graph_key: incident_ticket_graph
```

Draft flow:

```text
submit_ticket
-> agent_graph
-> owner_review
-> update_ticket
```

Planned agents:

```text
IncidentClassifyAgent
LogAnalysisAgent
SolutionRecommendAgent
TicketUpdateAgent
```

Planned tools:

```text
query_logs
query_service_status
create_ticket
notify_owner
```

Approval point:

- IT owner reviews recommended solution for medium or high severity incidents.

## Customer Service Ticket Quality Review

Business app:

```text
business_app_code: customer_service
```

Workflow:

```text
workflow_template_key: customer_ticket_quality_review
graph_key: customer_ticket_quality_review_graph
```

Draft flow:

```text
load_ticket
-> agent_graph
-> quality_review
-> archive_quality_result
```

Planned agents:

```text
IntentClassifyAgent
KnowledgeAnswerAgent
TicketCreateAgent
QualityReviewAgent
```

Planned tools:

```text
query_knowledge_base
create_customer_ticket
update_sla_record
generate_quality_report
```

Approval point:

- Customer service manager reviews low-quality or high-risk responses.

## Shared Platform Guarantees

Every future scenario must reuse:

```text
Workflow Engine
Workflow Template
Workflow Instance
Workflow Node Instance
Agent Gateway
Approval Task
Audit Log
Agent Run Log
Domain Policy
Tool Permission
File Metadata
```

Future scenarios must not introduce separate platform engines such as:

```text
hr_workflow_engine
legal_agent_gateway
procurement_approval_engine
```
