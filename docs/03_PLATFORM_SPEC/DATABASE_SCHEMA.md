# Database Schema

## Principles

- PostgreSQL is the primary database for V1.
- Core tables must remain business-domain neutral.
- V1 finance-specific fields should be stored in generic workflow input or `business_form_data.form_data` JSONB.
- Do not create finance-only platform tables such as `finance_tasks`, `finance_reports`, or `finance_approvals` in V1.
- Specialized business tables may be added only after a scenario becomes mature and stable.

## Common Columns

Unless otherwise stated, platform tables should include:

```text
id UUID primary key
created_at timestamptz not null
updated_at timestamptz not null
deleted_at timestamptz nullable
```

Use soft delete only for configuration and user-facing records where recovery or auditability matters.

## users

Enterprise user accounts.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| username | varchar(64) | yes | Unique |
| display_name | varchar(128) | yes | User display name |
| email | varchar(255) | no | Unique when present |
| password_hash | varchar(255) | yes | Hashed password |
| department_id | uuid | no | FK to departments |
| status | varchar(32) | yes | active, disabled |
| last_login_at | timestamptz | no | Last successful login |

Indexes:

- Unique index on `username`.
- Unique index on `email` where email is not null.
- Index on `department_id`.

## departments

Enterprise organization units.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| parent_id | uuid | no | Self FK |
| code | varchar(64) | yes | Unique |
| name | varchar(128) | yes | Department name |
| status | varchar(32) | yes | active, disabled |

## roles

Role definitions.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| code | varchar(64) | yes | Unique |
| name | varchar(128) | yes | Role name |
| description | text | no | Role description |
| status | varchar(32) | yes | active, disabled |

## permissions

Permission points.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| code | varchar(128) | yes | Unique |
| name | varchar(128) | yes | Permission name |
| resource | varchar(64) | yes | auth, workflow, approval, audit, agent, tool |
| action | varchar(64) | yes | create, read, update, delete, approve, retry |

## user_roles

User-role mapping.

| Column | Type | Required | Notes |
|---|---|---:|---|
| user_id | uuid | yes | FK to users |
| role_id | uuid | yes | FK to roles |

Primary key: `(user_id, role_id)`.

## role_permissions

Role-permission mapping.

| Column | Type | Required | Notes |
|---|---|---:|---|
| role_id | uuid | yes | FK to roles |
| permission_id | uuid | yes | FK to permissions |

Primary key: `(role_id, permission_id)`.

## business_apps

Business entry points.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| code | varchar(64) | yes | Unique, such as finance |
| name | varchar(128) | yes | Display name |
| description | text | no | Description |
| icon | varchar(128) | no | Frontend icon key |
| sort_order | int | yes | Display order |
| status | varchar(32) | yes | active, disabled |

## workflow_templates

Business workflow templates.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| business_app_code | varchar(64) | yes | FK-like reference to business_apps.code |
| workflow_template_key | varchar(128) | yes | Logical template key |
| name | varchar(128) | yes | Display name |
| version | varchar(32) | yes | Semantic version |
| graph_key | varchar(128) | yes | Explicit Python graph route |
| definition_json | jsonb | yes | Nodes and edges |
| status | varchar(32) | yes | draft, active, deprecated, disabled |

Constraints:

- Unique `(workflow_template_key, version)`.
- Index `(business_app_code, status)`.
- Index `graph_key`.

## workflow_instances

One workflow run.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| business_app_code | varchar(64) | yes | Business domain |
| workflow_template_id | uuid | yes | FK to workflow_templates |
| workflow_template_key | varchar(128) | yes | Snapshot for query |
| workflow_template_version | varchar(32) | yes | Snapshot |
| graph_key | varchar(128) | yes | Snapshot |
| title | varchar(255) | yes | Task title |
| status | varchar(32) | yes | draft, running, waiting_review, approved, rejected, archived, failed, cancelled |
| input_json | jsonb | yes | Initial input |
| output_json | jsonb | no | Final output |
| created_by | uuid | yes | FK to users |
| started_at | timestamptz | no | Start time |
| finished_at | timestamptz | no | Finish time |
| trace_id | varchar(128) | yes | Cross-service trace id |

Indexes:

- Index `(business_app_code, status)`.
- Index `(created_by, created_at)`.
- Index `trace_id`.

## workflow_node_instances

Runtime node records.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| workflow_instance_id | uuid | yes | FK to workflow_instances |
| node_key | varchar(128) | yes | Template node id |
| node_type | varchar(64) | yes | file_upload, agent_graph, human_review, system |
| name | varchar(128) | yes | Node name snapshot |
| status | varchar(32) | yes | pending, running, succeeded, failed, skipped, waiting_review, cancelled |
| input_json | jsonb | no | Node input |
| output_json | jsonb | no | Node output |
| error_json | jsonb | no | Error details |
| retry_count | int | yes | Default 0 |
| max_retries | int | yes | Default 0 or template value |
| started_at | timestamptz | no | Start time |
| finished_at | timestamptz | no | Finish time |

Indexes:

- Unique `(workflow_instance_id, node_key)`.
- Index `(workflow_instance_id, status)`.

## graph_registry

Registered Python Agent Graphs.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| graph_key | varchar(128) | yes | Unique |
| business_app_code | varchar(64) | yes | Owning business app |
| name | varchar(128) | yes | Display name |
| version | varchar(32) | yes | Version |
| description | text | no | Description |
| status | varchar(32) | yes | active, disabled, deprecated |

## agent_registry

Agent definitions.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| agent_id | varchar(128) | yes | Unique |
| name | varchar(128) | yes | Display name |
| domain | varchar(64) | yes | finance, hr, legal, shared |
| reusable_scope | varchar(32) | yes | domain_only, shared |
| capabilities_json | jsonb | yes | Capability list |
| input_schema_json | jsonb | yes | JSON schema |
| output_schema_json | jsonb | yes | JSON schema |
| endpoint | varchar(255) | no | Agent service endpoint |
| status | varchar(32) | yes | active, disabled |

## tool_registry

Tool definitions.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| tool_id | varchar(128) | yes | Unique |
| name | varchar(128) | yes | Display name |
| domain | varchar(64) | yes | finance, hr, legal, shared |
| risk_level | varchar(32) | yes | low, medium, high |
| is_shared | boolean | yes | Default false |
| input_schema_json | jsonb | yes | JSON schema |
| output_schema_json | jsonb | yes | JSON schema |
| status | varchar(32) | yes | active, disabled |

## agent_tool_permissions

Agent-to-tool authorization.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| agent_id | varchar(128) | yes | Agent id |
| tool_id | varchar(128) | yes | Tool id |
| business_app_code | varchar(64) | yes | Scope |
| status | varchar(32) | yes | active, disabled |

Constraints:

- Unique `(agent_id, tool_id, business_app_code)`.

## domain_policies

Business domain isolation policy.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| business_app_code | varchar(64) | yes | Unique |
| allowed_agent_domains | jsonb | yes | Domain list |
| allowed_tool_domains | jsonb | yes | Domain list |
| allow_shared_agents | boolean | yes | Default true |
| allow_shared_tools | boolean | yes | Default true |
| high_risk_requires_review | boolean | yes | Default true |
| status | varchar(32) | yes | active, disabled |

## approval_tasks

Human approval records.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| workflow_instance_id | uuid | yes | FK to workflow_instances |
| node_instance_id | uuid | yes | FK to workflow_node_instances |
| business_app_code | varchar(64) | yes | Business domain |
| title | varchar(255) | yes | Approval title |
| status | varchar(32) | yes | pending, approved, rejected, cancelled, expired |
| assignee_role | varchar(64) | no | Required reviewer role |
| assignee_user_id | uuid | no | Specific reviewer |
| decision_by | uuid | no | Reviewer user id |
| decision_comment | text | no | Approval comment |
| decided_at | timestamptz | no | Decision time |

## audit_logs

Business audit logs.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| trace_id | varchar(128) | yes | Trace id |
| actor_user_id | uuid | no | Actor |
| business_app_code | varchar(64) | no | Business domain |
| action | varchar(128) | yes | Action code |
| resource_type | varchar(64) | yes | workflow, approval, file, agent, tool |
| resource_id | varchar(128) | yes | Resource id |
| status | varchar(32) | yes | succeeded, failed |
| detail_json | jsonb | no | Details |
| ip_address | varchar(64) | no | Client IP |
| user_agent | text | no | Client UA |

Indexes:

- Index `trace_id`.
- Index `(business_app_code, created_at)`.
- Index `(actor_user_id, created_at)`.

## agent_run_logs

Agent and graph execution logs.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| run_id | varchar(128) | yes | Unique run id |
| trace_id | varchar(128) | yes | Trace id |
| workflow_instance_id | uuid | yes | FK to workflow_instances |
| node_instance_id | uuid | yes | FK to workflow_node_instances |
| business_app_code | varchar(64) | yes | Business domain |
| graph_key | varchar(128) | yes | Graph key |
| agent_id | varchar(128) | no | Agent id |
| status | varchar(32) | yes | succeeded, failed, retrying, cancelled |
| input_summary_json | jsonb | no | Sanitized summary |
| output_summary_json | jsonb | no | Sanitized summary |
| usage_json | jsonb | no | Model, tokens, cost |
| error_json | jsonb | no | Error details |
| started_at | timestamptz | no | Start time |
| finished_at | timestamptz | no | Finish time |
| duration_ms | int | no | Duration |

## files

Uploaded and generated files.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| workflow_instance_id | uuid | no | FK to workflow_instances |
| business_app_code | varchar(64) | yes | Business domain |
| storage_bucket | varchar(128) | yes | MinIO bucket |
| storage_key | varchar(512) | yes | Object key |
| original_filename | varchar(255) | yes | Original file name |
| content_type | varchar(128) | yes | MIME type |
| size_bytes | bigint | yes | File size |
| file_role | varchar(64) | yes | source, generated_report, attachment |
| uploaded_by | uuid | no | FK to users |
| checksum | varchar(128) | no | Optional checksum |

## business_form_data

Scenario-specific structured form data.

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| workflow_instance_id | uuid | yes | FK to workflow_instances |
| business_app_code | varchar(64) | yes | Business domain |
| form_key | varchar(128) | yes | Form key |
| form_data | jsonb | yes | Scenario data |
| schema_version | varchar(32) | yes | Form schema version |

## Finance JSONB Example

```json
{
  "month": "2026-05",
  "department": "Finance Center",
  "revenue": 1200000,
  "cost": 760000,
  "gross_profit": 440000,
  "net_profit": 310000,
  "customer_count": 860,
  "order_count": 1430
}
```

## Future JSONB Examples

HR onboarding:

```json
{
  "candidate_name": "Alice Zhang",
  "position": "Backend Engineer Intern",
  "resume_file_id": "file_001",
  "material_file_ids": ["file_002", "file_003"]
}
```

Legal contract review:

```json
{
  "counterparty": "Example Supplier Ltd.",
  "contract_amount": 500000,
  "contract_file_id": "file_101",
  "contract_type": "procurement"
}
```

Procurement request:

```json
{
  "item_name": "GPU Server",
  "budget_amount": 200000,
  "supplier_ids": ["supplier_001", "supplier_002"],
  "required_date": "2026-06-30"
}
```
