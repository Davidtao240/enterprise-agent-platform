# API Contract

## General Conventions

- External APIs use REST over HTTP JSON.
- Internal Agent API uses HTTP JSON.
- Protected APIs require JWT.
- Every request should carry or receive a `trace_id`.
- All timestamps use ISO 8601.
- IDs are UUID strings unless stated otherwise.
- Frontend must not call Python Agent Service directly.

## Common Headers

```text
Authorization: Bearer <jwt>
X-Trace-Id: <trace_id>
Content-Type: application/json
```

If `X-Trace-Id` is absent, backend creates one and returns it.

## Common Success Response

```json
{
  "trace_id": "trace_001",
  "data": {}
}
```

## Common List Response

```json
{
  "trace_id": "trace_001",
  "data": [],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100
  }
}
```

## Common Error Response

```json
{
  "trace_id": "trace_001",
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Invalid request body",
    "details": {}
  }
}
```

## Auth API

### POST /api/v1/auth/login

Request:

```json
{
  "username": "finance_user",
  "password": "password"
}
```

Response:

```json
{
  "trace_id": "trace_001",
  "data": {
    "access_token": "jwt",
    "token_type": "Bearer",
    "expires_in": 7200,
    "user": {
      "id": "user_001",
      "username": "finance_user",
      "display_name": "Finance User"
    }
  }
}
```

### GET /api/v1/auth/me

Returns current user, roles, and permissions.

## Business App API

### GET /api/v1/business-apps

Response:

```json
{
  "trace_id": "trace_001",
  "data": [
    {
      "code": "finance",
      "name": "Finance Center",
      "description": "Operating data reporting and analysis",
      "icon": "chart",
      "sort_order": 10,
      "status": "active"
    }
  ]
}
```

### GET /api/v1/business-apps/{code}

Returns a single business app.

### GET /api/v1/business-apps/registry

Returns read-only Business App registry rows for platform operations. Protected by `business_app:read`.

Query:

```text
status optional
```

### GET /api/v1/domain-policies

Returns read-only Domain Policy rows for platform operations. Protected by `business_app:read`.

Query:

```text
business_app_code optional
status optional
```

## Workflow Template API

### GET /api/v1/workflow-templates

Query:

```text
business_app_code optional
status optional
page optional
page_size optional
```

### GET /api/v1/business-apps/{code}/workflow-templates

Returns active workflow templates for a business app.

### GET /api/v1/workflow-templates/{id}

Returns template detail including `definition_json`.

## Workflow Instance API

### POST /api/v1/workflow-instances

Request:

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

Response:

```json
{
  "trace_id": "trace_001",
  "data": {
    "id": "wf_001",
    "business_app_code": "finance",
    "workflow_template_key": "finance_operating_report",
    "workflow_template_version": "1.0.0",
    "graph_key": "finance_operating_report_graph",
    "title": "2026-05 Operating Data Report",
    "status": "draft",
    "trace_id": "trace_001"
  }
}
```

### GET /api/v1/workflow-instances

Query:

```text
business_app_code optional
status optional
created_by optional
page optional
page_size optional
```

### GET /api/v1/workflow-instances/{id}

Returns workflow detail.

### POST /api/v1/workflow-instances/{id}/start

Starts a draft workflow.

Response:

```json
{
  "trace_id": "trace_001",
  "data": {
    "id": "wf_001",
    "status": "running"
  }
}
```

### POST /api/v1/workflow-instances/{id}/cancel

Request:

```json
{
  "reason": "Created by mistake"
}
```

### POST /api/v1/workflow-instances/{id}/retry

Retries the failed workflow from the failed retryable node.

Request:

```json
{
  "node_instance_id": "node_002"
}
```

### GET /api/v1/workflow-instances/{id}/nodes

Returns node instances.

### GET /api/v1/workflow-instances/{id}/events

Returns workflow events and audit timeline.

## File API

### POST /api/v1/files

Multipart upload.

Fields:

```text
workflow_instance_id
business_app_code
file_role
file
```

Response:

```json
{
  "trace_id": "trace_001",
  "data": {
    "id": "file_001",
    "original_filename": "finance_2026_05.csv",
    "content_type": "text/csv",
    "size_bytes": 10240,
    "file_role": "source"
  }
}
```

### GET /api/v1/files/{id}

Returns file metadata.

### GET /api/v1/files/{id}/download-url

Returns a temporary download URL if authorized.

## Approval Task API

### GET /api/v1/approval-tasks

Query:

```text
business_app_code optional
status optional
assignee_user_id optional
assignee_role optional
page optional
page_size optional
```

### GET /api/v1/approval-tasks/{id}

Returns approval task detail and related workflow summary.

### POST /api/v1/approval-tasks/{id}/approve

Request:

```json
{
  "comment": "Report looks reasonable."
}
```

### POST /api/v1/approval-tasks/{id}/reject

Request:

```json
{
  "comment": "Revenue data requires correction."
}
```

## Agent Registry API

### GET /api/v1/agents

Returns registered agents.

### POST /api/v1/agents

Admin-only. Creates an agent registration.

### PATCH /api/v1/agents/{agent_id}

Admin-only. Updates status, schema, or metadata.

## Tool Registry API

### GET /api/v1/tools

Returns registered tools.

### POST /api/v1/tools

Admin-only. Creates a tool registration.

### PATCH /api/v1/tools/{tool_id}

Admin-only. Updates tool metadata or status.

## Agent Run Log API

### GET /api/v1/agent-run-logs

Query:

```text
workflow_instance_id optional
node_instance_id optional
graph_key optional
agent_id optional
status optional
page optional
page_size optional
```

### GET /api/v1/agent-run-logs/{run_id}

Returns one run log.

## Audit Log API

### GET /api/v1/audit-logs

Query:

```text
business_app_code optional
actor_user_id optional
resource_type optional
resource_id optional
action optional
start_time optional
end_time optional
page optional
page_size optional
```

### GET /api/v1/audit-logs/stats

Returns filtered audit totals, status/action buckets, and recent key actions. Requires `audit:read`.

### GET /api/v1/audit-logs/export

Exports the same filtered audit log data as CSV. Requires `audit:read` and is capped at 10,000 rows.

## Configuration Governance API

All endpoints require JWT authentication. `GET`, `POST`, `submit`, and `deprecate` require `configuration:manage`; publishing requires `configuration:approve`. The author of a change cannot publish it.

### GET /api/v1/configuration-versions

Optional query filters: `resource_type`, `resource_key`, `status`.

### POST /api/v1/configuration-versions

Creates a draft configuration version. `resource_type` must be one of `business_app`, `workflow_template`, `agent`, `tool`, or `domain_policy`.

```json
{
  "resource_type": "workflow_template",
  "resource_key": "operating_report",
  "version": "2.0.0",
  "snapshot_json": {"nodes": [], "edges": []},
  "change_summary": "Add an approved domain-neutral validation step."
}
```

### POST /api/v1/configuration-versions/{id}/submit

Moves a draft to `pending_approval`.

### POST /api/v1/configuration-versions/{id}/approve

Moves a pending version to `published`. A user cannot approve their own change.

### POST /api/v1/configuration-versions/{id}/deprecate

Moves a published version to `deprecated`.

## Platform Observability API

### GET /api/v1/platform-observability/summary

Requires `observability:read`. Returns workflow and agent-run status counts, recent agent-run failures, average duration, reported model cost, and derived warning alerts. The response is business-domain neutral.

## Workflow Reliability

### POST /api/v1/workflow-instances

The optional `Idempotency-Key` header is scoped to the authenticated user and limited to 128 characters. Repeating the same key returns the original workflow instance rather than creating a duplicate execution.

## Tenant Identity

JWT responses and `GET /api/v1/auth/me` expose the authenticated `tenant_id`. The tenant is signed into the token and injected by authentication middleware; clients must not supply a tenant identity header to select another tenant.

## Internal Agent API

### POST /internal/v1/agent-runs

Called by Go Agent Gateway only.

Request:

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {
    "file_id": "file_001"
  },
  "context": {
    "user_id": "user_001",
    "department_id": "finance",
    "tenant_id": "default"
  }
}
```

Response:

```json
{
  "run_id": "run_001",
  "graph_key": "finance_operating_report_graph",
  "status": "succeeded",
  "output": {
    "summary": "Revenue increased by 8.2%.",
    "warnings": [],
    "result_file_id": "file_099"
  },
  "usage": {
    "model": "qwen-plus",
    "prompt_tokens": 1200,
    "completion_tokens": 600,
    "cost": 0.03
  },
  "error": null
}
```

## Error Codes

| Code | Meaning |
|---|---|
| UNAUTHORIZED | Missing or invalid token |
| FORBIDDEN | Permission denied |
| VALIDATION_FAILED | Invalid request |
| RESOURCE_NOT_FOUND | Resource does not exist |
| WORKFLOW_INVALID_STATE | Invalid state transition |
| WORKFLOW_TEMPLATE_NOT_FOUND | Template not found |
| GRAPH_NOT_FOUND | Graph not found |
| AGENT_NOT_ALLOWED | Agent is not allowed |
| TOOL_NOT_ALLOWED | Tool is not allowed |
| DOMAIN_POLICY_VIOLATION | Domain policy violation |
| APPROVAL_NOT_PENDING | Approval task is not pending |
| AGENT_RUN_FAILED | Agent graph execution failed |
| INTERNAL_ERROR | Unexpected server error |
