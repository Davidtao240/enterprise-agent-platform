# Seed Data

> 文档状态：Finance V1 Seed Baseline
> 说明：记录当前演示和回归所需 Seed；不表示 Durable Run、Tool Gateway 或 Connector Target Schema 已实现。

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

## M7 Increment: 对话式 Agent 种子

M7-C 交付 3 个对话式 Agent：财务报告（对话式改造）+ 文档总结 + 会议纪要。新增 `productivity` 业务域承载通用办公 Agent；`conversation:read` / `conversation:write` 权限授予全部默认角色。

### Business App: productivity

```json
{
  "code": "productivity",
  "name": "Productivity Hub",
  "description": "General-purpose office assistants: document summary, meeting minutes.",
  "icon": "appstore",
  "sort_order": 20,
  "status": "active"
}
```

### Graph Registry (M7 新增)

```json
[
  {
    "graph_key": "document_summary_graph",
    "business_app_code": "productivity",
    "name": "Document Summary Graph",
    "version": "1.0.0",
    "description": "Parse document, summarize sections, answer follow-up questions.",
    "status": "active"
  },
  {
    "graph_key": "meeting_minutes_graph",
    "business_app_code": "productivity",
    "name": "Meeting Minutes Graph",
    "version": "1.0.0",
    "description": "Extract decisions, action items and owners from transcript.",
    "status": "active"
  }
]
```

`finance_operating_report_graph` 保持不变，M7-C 为其增加对话式入口（澄清月份/部门、追问分析）。

### Agent Packages (画廊种子)

| package_code | name | category | business_app_code | graph_key | entry_type | status |
|---|---|---|---|---|---|---|
| finance_operating_report_assistant | 财务经营报告助手 | departmental | finance | finance_operating_report_graph | conversation | published |
| document_summary_assistant | 文档总结助手 | general | productivity | document_summary_graph | conversation | published |
| meeting_minutes_assistant | 会议纪要助手 | general | productivity | meeting_minutes_graph | conversation | published |

每个包附 `sample_prompts_json`（每包 3 条，文案以 [WORKBENCH_DESIGN.md](../06_FRONTEND/WORKBENCH_DESIGN.md) §7.5.2 定稿为准，seed 实现直接引用该清单，不得另行编写）。卡片 icon/description 同 §7.5.1。seed 同时初始化 `agent_package_usage_stats` 空表结构数据（由每日任务自然填充，不造假统计）。

### Agents / Tools / Permissions (productivity)

Agents:

| agent_id | Name | Domain | reusable_scope | Capabilities |
|---|---|---|---|---|
| document_summary_agent | Document Summary Agent | shared | shared | parse_document, section_summary, followup_qa |
| meeting_minutes_agent | Meeting Minutes Agent | shared | shared | parse_transcript, extract_action_items, format_minutes |

Tools:

| tool_id | Name | Domain | risk_level | is_shared |
|---|---|---|---|---|
| summarize_document | Summarize Document | shared | low | true |
| generate_minutes | Generate Minutes | shared | low | true |

Agent Tool Permissions:

| agent_id | tool_id | business_app_code |
|---|---|---|
| document_summary_agent | summarize_document | productivity |
| meeting_minutes_agent | generate_minutes | productivity |

### Domain Policy: productivity

```json
{
  "business_app_code": "productivity",
  "allowed_agent_domains": ["shared"],
  "allowed_tool_domains": ["shared"],
  "allow_shared_agents": true,
  "allow_shared_tools": true,
  "high_risk_requires_review": true,
  "status": "active"
}
```

### Permissions (M7 新增)

| Code | Resource | Action | 授予角色 |
|---|---|---|---|
| conversation:read | conversation | read | 全部角色 |
| conversation:write | conversation | write | 全部角色 |
