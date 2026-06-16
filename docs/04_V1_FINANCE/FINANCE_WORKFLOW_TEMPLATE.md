# Finance Workflow Template

## Purpose

This document defines the V1 finance operating report workflow template. It is a business template interpreted by the generic Go Workflow Engine.

The Workflow Engine must not hard-code finance-specific logic.

## Template Identity

```text
business_app_code: finance
workflow_template_key: finance_operating_report
name: Operating Data Report
version: 1.0.0
graph_key: finance_operating_report_graph
```

## Business Goal

Finance users upload monthly operating data. AI agents analyze the data and generate a report draft. A finance manager reviews the report. Approved reports are archived.

## Node Overview

```text
upload
-> agent_graph
-> human_review
-> archive
```

## Node Definitions

### upload

```json
{
  "id": "upload",
  "type": "file_upload",
  "name": "Upload Operating Data",
  "required": true,
  "accepted_file_types": [".csv", ".xlsx"],
  "max_file_size_mb": 20
}
```

### agent_graph

```json
{
  "id": "agent_graph",
  "type": "agent_graph",
  "name": "AI Finance Analysis",
  "graph_key": "finance_operating_report_graph",
  "input_mapping": {
    "file_id": "$files[0].id",
    "month": "$input.month",
    "department": "$input.department"
  },
  "output_mapping": {
    "summary": "$output.summary",
    "key_metrics": "$output.key_metrics",
    "warnings": "$output.warnings",
    "report": "$output.report",
    "result_file_id": "$output.result_file_id",
    "review_suggestions": "$output.review_suggestions"
  },
  "max_retries": 2
}
```

### human_review

```json
{
  "id": "human_review",
  "type": "human_review",
  "name": "Finance Manager Review",
  "role": "business_reviewer",
  "required": true,
  "review_payload_mapping": {
    "summary": "$nodes.agent_graph.output.summary",
    "key_metrics": "$nodes.agent_graph.output.key_metrics",
    "warnings": "$nodes.agent_graph.output.warnings",
    "report": "$nodes.agent_graph.output.report",
    "result_file_id": "$nodes.agent_graph.output.result_file_id"
  }
}
```

### archive

```json
{
  "id": "archive",
  "type": "system",
  "name": "Archive Report",
  "action": "archive_result",
  "input_mapping": {
    "result_file_id": "$nodes.agent_graph.output.result_file_id",
    "approval_task_id": "$nodes.human_review.output.approval_task_id"
  }
}
```

## Edge Definitions

```json
[
  { "from": "upload", "to": "agent_graph" },
  { "from": "agent_graph", "to": "human_review", "when": "succeeded" },
  { "from": "agent_graph", "to": "failed", "when": "failed" },
  { "from": "human_review", "to": "archive", "when": "approved" },
  { "from": "human_review", "to": "rejected", "when": "rejected" }
]
```

## Full definition_json Example

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "name": "Operating Data Report",
  "version": "1.0.0",
  "graph_key": "finance_operating_report_graph",
  "description": "Monthly operating data reporting and finance analysis workflow.",
  "nodes": [
    {
      "id": "upload",
      "type": "file_upload",
      "name": "Upload Operating Data",
      "required": true,
      "accepted_file_types": [".csv", ".xlsx"],
      "max_file_size_mb": 20
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
      },
      "output_mapping": {
        "summary": "$output.summary",
        "key_metrics": "$output.key_metrics",
        "warnings": "$output.warnings",
        "report": "$output.report",
        "result_file_id": "$output.result_file_id",
        "review_suggestions": "$output.review_suggestions"
      },
      "max_retries": 2
    },
    {
      "id": "human_review",
      "type": "human_review",
      "name": "Finance Manager Review",
      "role": "business_reviewer",
      "required": true,
      "review_payload_mapping": {
        "summary": "$nodes.agent_graph.output.summary",
        "key_metrics": "$nodes.agent_graph.output.key_metrics",
        "warnings": "$nodes.agent_graph.output.warnings",
        "report": "$nodes.agent_graph.output.report",
        "result_file_id": "$nodes.agent_graph.output.result_file_id"
      }
    },
    {
      "id": "archive",
      "type": "system",
      "name": "Archive Report",
      "action": "archive_result",
      "input_mapping": {
        "result_file_id": "$nodes.agent_graph.output.result_file_id",
        "approval_task_id": "$nodes.human_review.output.approval_task_id"
      }
    }
  ],
  "edges": [
    { "from": "upload", "to": "agent_graph" },
    { "from": "agent_graph", "to": "human_review", "when": "succeeded" },
    { "from": "agent_graph", "to": "failed", "when": "failed" },
    { "from": "human_review", "to": "archive", "when": "approved" },
    { "from": "human_review", "to": "rejected", "when": "rejected" }
  ]
}
```

## State Behavior

Approved path:

```text
draft
-> running
-> waiting_review
-> approved
-> archived
```

Rejected path:

```text
draft
-> running
-> waiting_review
-> rejected
```

Agent failure path:

```text
draft
-> running
-> failed
```

## Archive Output

Archived workflow output should include:

```json
{
  "result_file_id": "file_099",
  "summary": "Revenue increased by 8.2%.",
  "key_metrics": {},
  "approved_by": "user_002",
  "approved_at": "2026-05-26T10:00:00Z"
}
```
