# Seed Data

## Purpose

This document defines the initial data required for V1 local development and demo. V1 initializes only the finance business scenario, while keeping the data model ready for future HR, legal, procurement, IT service, and customer service scenarios.

## Default Departments

| Code | Name | Status |
|---|---|---|
| platform | Platform Team | active |
| finance | Finance Center | active |
| ops | Operations Team | active |

## Default Users

| Username | Display Name | Department | Roles |
|---|---|---|---|
| admin | Platform Admin | platform | platform_admin |
| finance_user | Finance User | finance | business_user |
| finance_manager | Finance Manager | finance | business_reviewer |
| ops_viewer | Ops Viewer | ops | ops_viewer |

Default development password may be `password` and must be changed in production-like deployments.

## Default Roles

| Code | Name |
|---|---|
| platform_admin | Platform Admin |
| business_user | Business User |
| business_reviewer | Business Reviewer |
| ops_viewer | Ops Viewer |

## Default Permissions

| Code | Resource | Action |
|---|---|---|
| business_app:read | business_app | read |
| workflow_template:read | workflow_template | read |
| workflow:create | workflow | create |
| workflow:read | workflow | read |
| workflow:start | workflow | start |
| workflow:cancel | workflow | cancel |
| workflow:retry | workflow | retry |
| file:upload | file | upload |
| file:read | file | read |
| approval:read | approval | read |
| approval:decide | approval | decide |
| audit:read | audit | read |
| agent:manage | agent | manage |
| tool:manage | tool | manage |
| user:manage | user | manage |
| role:manage | role | manage |

## Role Permission Mapping

### platform_admin

All permissions.

### business_user

```text
business_app:read
workflow_template:read
workflow:create
workflow:read
workflow:start
file:upload
file:read
```

### business_reviewer

```text
business_app:read
workflow_template:read
workflow:read
file:read
approval:read
approval:decide
```

### ops_viewer

```text
business_app:read
workflow_template:read
workflow:read
workflow:retry
file:read
approval:read
audit:read
```

## Business Apps

### finance

```json
{
  "code": "finance",
  "name": "Finance Center",
  "description": "Operating data reporting, finance analysis, report review, and archive.",
  "icon": "chart",
  "sort_order": 10,
  "status": "active"
}
```

## Graph Registry

```json
{
  "graph_key": "finance_operating_report_graph",
  "business_app_code": "finance",
  "name": "Finance Operating Report Graph",
  "version": "1.0.0",
  "description": "Extract, validate, analyze, and generate finance operating report.",
  "status": "active"
}
```

## Workflow Template

### finance_operating_report

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "name": "Operating Data Report",
  "version": "1.0.0",
  "graph_key": "finance_operating_report_graph",
  "description": "Monthly operating data reporting and finance analysis workflow.",
  "status": "active",
  "definition_json": {
    "nodes": [
      {
        "id": "upload",
        "type": "file_upload",
        "name": "Upload Operating Data",
        "required": true
      },
      {
        "id": "agent_graph",
        "type": "agent_graph",
        "name": "AI Finance Analysis",
        "graph_key": "finance_operating_report_graph",
        "input_mapping": {
          "file_id": "$files[0].id",
          "month": "$input.month",
          "department": "$input.department"
        }
      },
      {
        "id": "human_review",
        "type": "human_review",
        "name": "Finance Manager Review",
        "role": "business_reviewer",
        "required": true
      },
      {
        "id": "archive",
        "type": "system",
        "name": "Archive Report",
        "action": "archive_result"
      }
    ],
    "edges": [
      { "from": "upload", "to": "agent_graph" },
      { "from": "agent_graph", "to": "human_review" },
      { "from": "human_review", "to": "archive", "when": "approved" }
    ]
  }
}
```

## Agent Registry

| agent_id | Name | Domain | reusable_scope | Capabilities |
|---|---|---|---|---|
| data_extract_agent | Data Extract Agent | shared | shared | extract_table, parse_csv, parse_excel |
| schema_mapping_agent | Schema Mapping Agent | shared | shared | normalize_fields, map_schema |
| validation_agent | Validation Agent | shared | shared | validate_required_fields, detect_outliers |
| finance_analysis_agent | Finance Analysis Agent | finance | domain_only | metric_analysis, trend_summary, risk_explanation |
| report_agent | Report Agent | shared | shared | report_generation, summary_generation |
| review_summary_agent | Review Summary Agent | shared | shared | review_summary, warning_summary |

## Tool Registry

| tool_id | Name | Domain | risk_level | is_shared |
|---|---|---|---|---|
| parse_csv | Parse CSV | shared | low | true |
| parse_excel | Parse Excel | shared | low | true |
| normalize_finance_schema | Normalize Finance Schema | finance | low | false |
| validate_finance_metrics | Validate Finance Metrics | finance | medium | false |
| generate_finance_report | Generate Finance Report | finance | medium | false |
| archive_report | Archive Report | finance | high | false |

## Agent Tool Permissions

| agent_id | tool_id | business_app_code |
|---|---|---|
| data_extract_agent | parse_csv | finance |
| data_extract_agent | parse_excel | finance |
| schema_mapping_agent | normalize_finance_schema | finance |
| validation_agent | validate_finance_metrics | finance |
| finance_analysis_agent | validate_finance_metrics | finance |
| report_agent | generate_finance_report | finance |
| review_summary_agent | generate_finance_report | finance |

## Domain Policy

```json
{
  "business_app_code": "finance",
  "allowed_agent_domains": ["finance", "shared"],
  "allowed_tool_domains": ["finance", "shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true,
  "status": "active"
}
```

## Sample Finance Workflow Instance

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "title": "2026-05 Operating Data Report",
  "input": {
    "month": "2026-05",
    "department": "Finance Center"
  }
}
```

## Future Seed Data Policy

Future HR, legal, procurement, IT service, and customer service seed data should be added only when the corresponding scenario is implemented. Each new scenario must add:

- Business App.
- Workflow Template.
- Graph Registry record.
- Domain Policy.
- Domain agents.
- Domain tools.
- Agent Tool Permissions.
- Demo users or reviewer roles if needed.
