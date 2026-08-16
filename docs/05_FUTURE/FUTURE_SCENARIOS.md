# Future Scenarios

> 文档状态：Deferred Scenario Catalog
> 更新日期：2026-08-16
> 本文件记录候选场景，不代表当前实施顺序。实现准入以 [`../01_PROJECT/ROADMAP.md`](../01_PROJECT/ROADMAP.md) 的 M1–M7 Gate 为准。

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
- Versioned Skill/Domain Profile and Eval Fixture.
- Tool Execution Gateway and Connector binding when enterprise systems are accessed.

Before a complete new scenario is implemented, the platform must have passed the relevant gates:

```text
M1 Durable Agent Run
+ M2 Tool Execution Gateway
+ required M3 Connector
+ baseline Trace/Eval
```

Scenario design and synthetic fixtures may proceed earlier, but real data, external side effects and production activation remain prohibited until their security and integration gates pass.

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

## Procurement Quote Review（已批准 P0）

Business app:

```text
business_app_code: procurement
```

Workflow:

```text
workflow_template_key: procurement_quote_review
graph_key: procurement_quote_review_graph
```

Draft flow:

```text
submit_requirement_and_quotes
-> agent_graph
-> procurement_review
-> archive_decision_package
```

Planned agents:

```text
DocumentExtractionAgent
ValidationAgent
SupplierCompareAgent
RiskEvidenceAgent
ReportAgent
```

Planned tools:

```text
parse_quote
compare_quote
query_approved_policy_fixture
render_structured_report
```

Approval point:

- Procurement manager reviews supplier comparison.
- Agent only provides decision support; the human makes the final decision.
- P0 explicitly prohibits creating a purchase order, contacting suppliers, modifying master data, approving invoices or initiating payment.
- A future ERP write scenario requires M2 Tool Gateway, an approved M3 ERP Connector, exact-payload approval and a separate production gate.

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
Durable Agent Runtime
Tool Execution Gateway
Connector Runtime
Trace / Eval
```

Future scenarios must not introduce separate platform engines such as:

```text
hr_workflow_engine
legal_agent_gateway
procurement_approval_engine
```
