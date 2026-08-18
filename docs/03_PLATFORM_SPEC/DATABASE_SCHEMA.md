# Database Schema

> 文档状态：Active Specification
> 更新日期：2026-08-18
> 标注为 M1/M2/M3 Target 的表尚未因此文档自动成为已实现功能，必须通过 migration、repository 和测试落地。

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
| durable_run_id | uuid | no | M1-A additive FK to `agent_runs`; legacy rows remain null |

M1-A compatibility strategy: the synchronous V1 bridge uses the same Go-generated
UUID for `agent_runs.id`, `agent_run_logs.run_id`, and
`agent_run_logs.durable_run_id`. Start and terminal updates are dual-written in a
single PostgreSQL transaction. Existing rows are not backfilled and all V1 list
and detail queries continue to read `agent_run_logs`.

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

## Current: configuration_versions

通用配置治理表当前已支持 `business_app`、`workflow_template`、`agent`、`tool`、`domain_policy`，并使用 `draft → pending_approval → published → deprecated` 生命周期。

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Configuration version id |
| tenant_id | uuid | yes | Tenant boundary |
| resource_type | varchar(64) | yes | Configuration kind |
| resource_key | varchar(128) | yes | Logical identity |
| version | varchar(32) | yes | Semantic version |
| lifecycle_status | varchar(32) | yes | draft, pending_approval, published, deprecated |
| snapshot_json | jsonb | yes | Immutable published snapshot |
| change_summary | text | yes | Change reason |
| created_by / approved_by | uuid | yes/no | Separation of duties |
| published_at / deprecated_at | timestamptz | no | Lifecycle timestamps |
| trace_id | varchar(128) | yes | Governance trace |

M1 Run 的 `configuration_snapshot_json` 应优先保存 Published Configuration Version ID/key/version。Registry 表作为能力发现和当前投影，不替代不可变版本。

M2–M5 需要以领域中立方式扩充受支持的 `resource_type`：

```text
graph
skill
runtime_policy
connector
connector_binding
policy_set
scoring_profile
model_config
eval_definition
```

扩充前需要 migration、`governance.IsSupportedResourceType`、API 校验和测试同步修改。

## Current M1-A: agent_threads

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Thread id |
| tenant_id | uuid | yes | Tenant boundary |
| created_by | uuid | yes | User or service actor |
| business_app_code | varchar(64) | no | Optional business context |
| workflow_instance_id | uuid | no | Optional workflow link |
| title | varchar(255) | no | Display title |
| status | varchar(32) | yes | active, closed, deleted |

## Current M1-A: agent_runs

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Platform run id |
| thread_id | uuid | yes | FK to agent_threads |
| tenant_id | uuid | yes | Tenant boundary |
| trace_id | varchar(128) | yes | Cross-service trace |
| workflow_instance_id | uuid | no | Workflow link |
| node_instance_id | uuid | no | Node link |
| parent_run_id | uuid | no | Optional parent run |
| graph_key | varchar(128) | yes | Graph identity snapshot |
| graph_version | varchar(32) | yes | Immutable version |
| configuration_snapshot_json | jsonb | yes | Agent/Skill/Model/Policy versions, no Secret |
| status | varchar(32) | yes | queued, running, waiting_human, waiting_external, succeeded, failed, cancelled |
| attempt | int | yes | Current execution attempt |
| checkpoint_version | bigint | no | Latest acknowledged version |
| lease_owner | varchar(128) | no | Worker owner |
| lease_expires_at | timestamptz | no | Lease expiry |
| heartbeat_at | timestamptz | no | Last heartbeat |
| deadline_at | timestamptz | no | Runtime deadline |
| budget_json | jsonb | no | Step/token/cost limits |
| output_summary_json | jsonb | no | Sanitized result summary |
| error_json | jsonb | no | Structured error |
| started_at | timestamptz | no | Start time |
| finished_at | timestamptz | no | Finish time |

Indexes/constraints:

- Index `(tenant_id, status, created_at)`.
- Index `(workflow_instance_id, node_instance_id)`.
- Unique platform run `id` and indexed `trace_id`.
- Status/attempt/lease updates use optimistic concurrency or guarded transition.
- Workflow、Node、Parent Run 和兼容 Run Log 使用包含 `tenant_id` 的复合外键；
  数据库拒绝跨 Tenant 拼接 Durable Run 关系。

## Current M1-A: agent_run_steps

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Step id |
| tenant_id | uuid | yes | Tenant boundary; must match the parent Run |
| run_id | uuid | yes | FK to agent_runs |
| sequence | bigint | yes | Monotonic within run |
| attempt | int | yes | Run attempt |
| step_type | varchar(32) | yes | model, tool, checkpoint, interrupt, system |
| name | varchar(128) | no | Runtime node/capability name |
| status | varchar(32) | yes | pending, running, succeeded, failed, cancelled |
| input_summary_json | jsonb | no | Redacted summary |
| output_summary_json | jsonb | no | Redacted summary |
| usage_json | jsonb | no | Token/cost/duration |
| error_json | jsonb | no | Structured error |

Unique `(run_id, sequence)`.

M1-B Runtime 的 `step.started/completed/failed` Event 会在同一个 Go
PostgreSQL 事务中创建或完成对应 Step；恢复投递复用稳定 `step_id`，不会新增
重复 Step。

## Current M1-B: agent_checkpoints

保存受治理索引，不要求把完整 Python Graph State 复制到 Go：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Checkpoint metadata id |
| tenant_id | uuid | yes | Tenant boundary; must match parent Run |
| run_id | uuid | yes | Run link |
| version | bigint | yes | Monotonic version |
| backend | varchar(32) | yes | Python checkpointer type |
| checkpoint_ref | text | yes | Opaque reference, no Secret |
| state_hash | varchar(128) | no | Integrity/dedup |
| created_at | timestamptz | yes | Saved time |

Unique `(run_id, version)`.

## Current M1-B: agent_interrupts

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | varchar(128) | yes | Opaque LangGraph Interrupt id |
| tenant_id | uuid | yes | Tenant boundary; must match parent Run |
| run_id | uuid | yes | Run link |
| step_id | uuid | no | Origin step |
| checkpoint_version | bigint | yes | Resume boundary |
| kind | varchar(32) | yes | human, external, input_required |
| status | varchar(32) | yes | pending, resumed, cancelled, expired |
| resume_schema_json | jsonb | yes | Allowed resume payload |
| expires_at | timestamptz | no | Expiry |
| resumed_by | uuid | no | Actor |
| resume_idempotency_key | varchar(255) | no | Applied Resume identity |
| resumed_at | timestamptz | no | Resume time |

## Current M1-A: runtime_events

| Column | Type | Required | Notes |
|---|---|---:|---|
| event_id | varchar(128) | yes | Idempotency identity |
| tenant_id | uuid | yes | Tenant boundary; must match the parent Run |
| run_id | uuid | yes | Run link |
| sequence | bigint | yes | Monotonic sequence |
| attempt | int | yes | Attempt guard; stale attempts are rejected |
| event_type | varchar(64) | yes | Runtime event type |
| payload_json | jsonb | no | Sanitized payload |
| checkpoint_version | bigint | no | Associated checkpoint |
| occurred_at | timestamptz | yes | Runtime time |
| consumed_at | timestamptz | no | Go apply time |

Unique `event_id`; unique `(run_id, sequence)` when Runtime guarantees sequence uniqueness.

## M2 Target: tool_calls

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Tool call id |
| tenant_id | uuid | yes | Tenant boundary |
| run_id | uuid | yes | Run link |
| step_id | uuid | no | Step link |
| tool_id | varchar(128) | yes | Tool identity |
| tool_version | varchar(32) | yes | Immutable version |
| connector_binding_id | uuid | conditional | 外部系统工具必填；内置工具（如 parse_csv）为空 |
| policy_version | varchar(64) | yes | Policy snapshot |
| risk_level | varchar(32) | yes | Risk decision |
| status | varchar(32) | yes | requested, pending_approval, executing, succeeded, failed, indeterminate, cancelled |
| idempotency_key | varchar(255) | yes | Side-effect dedup |
| input_hash | varchar(128) | yes | Approval/audit binding |
| input_summary_json | jsonb | no | Redacted |
| approval_task_id | uuid | no | High-risk approval |
| external_request_id | varchar(255) | no | Provider request |
| external_object_id | varchar(255) | no | ERP/ticket object |
| verification_json | jsonb | no | Verify/reconcile result |
| output_summary_json | jsonb | no | Redacted |
| error_json | jsonb | no | Structured error |

Unique `(tenant_id, idempotency_key)`.

## M2-D 已实现: connector_bindings / credential_secrets

M2-D 已通过 migration 020 落地"平台内置加密"作为 Secret Provider 的第一种实现：

`connector_bindings`（已实现，M3-A 将补列）：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| tenant_id | uuid | yes | Tenant boundary |
| business_app_code | varchar(64) | yes | Business app scope |
| connector_code | varchar(128) | yes | 必须存在于 connector_registry（M3-A 起 FK 校验） |
| name | varchar(256) | yes | Display name |
| status | varchar(16) | yes | active / disabled |
| config_json | jsonb | yes | 非敏感配置（endpoint/timeout 等） |
| credential_ref | varchar(128) | no | `secret:<uuid>`，永不存明文 |
| environment | varchar(16) | M3-A 补 | mock / sandbox / shadow / production，发布阶段门禁依据 |
| allowed_capabilities | jsonb | M3-A 补 | Binding 级 capability 白名单 |
| connector_version | varchar(32) | M3-A 补 | 绑定的 registry 版本，不可变 |
| created_at / updated_at | timestamptz | yes | — |

`credential_secrets`（已实现）：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key，即 credential_ref 中的 uuid |
| cipher_text | text | yes | base64(nonce + ciphertext + tag)，AES-256-GCM |
| key_hint | varchar(64) | yes | 密钥版本提示（轮换用，非密钥本身；M3 前补真实写入） |
| algorithm | varchar(32) | yes | 默认 AES-256-GCM |
| created_at / updated_at | timestamptz | yes | — |

> Secret Provider 抽象保留：内置加密（当前）与外部 Vault（未来）通过 credential_ref 前缀区分（`secret:` 为内置；预留 `vault:`）。切换 Provider 不改变 Binding 与 ToolCall 契约。

## M3 Target: connector_registry

Connector 的注册与版本治理（M3-A 落地）：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| connector_code | varchar(128) | yes | 逻辑标识（如 enterprise_db_read_connector） |
| version | varchar(32) | yes | 语义化版本，不可变 |
| connector_type | varchar(64) | yes | db_read / ticket / erp / mock |
| capabilities_json | jsonb | yes | 能力清单（name + input/output schema） |
| auth_type | varchar(32) | yes | none / api_key / oauth2 / basic |
| health_check_json | jsonb | no | 健康检查契约配置 |
| release_stage | varchar(32) | yes | mock_fixture / sandbox_readonly / shadow / human_approved_write / limited_canary / production |
| status | varchar(16) | yes | draft / active / deprecated |
| created_at / updated_at | timestamptz | yes | — |

Unique `(connector_code, version)`。Connector 实现替换供应商不改 Runtime 核心，靠此表注册。

## M3-B: webhook_events（已实现，migration 022）

Webhook Inbox（签名校验后落库，去重与乱序由消费端处理）：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| connector_code | varchar(128) | yes | 来源 Connector |
| external_event_id | varchar(255) | yes | 供应商事件 ID（去重键之一） |
| signature_valid | boolean | yes | 签名校验结果，false 的事件不进入处理 |
| payload_json | jsonb | yes | 原始事件体（不可信数据，消费时防注入） |
| received_at | timestamptz | yes | 接收时间 |
| processed_at | timestamptz | no | 消费完成时间；NULL 表示待处理 |
| process_error | text | no | 最近一次处理失败原因 |

Unique `(connector_code, external_event_id)`。

## M3-C: connector_outbox（已实现，migration 023）

跨系统写操作不假设分布式强事务，Outbox 驱动 Saga/Compensation：

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| tool_call_id | uuid | yes | 外键关联 tool_calls(id)（副作用溯源） |
| tenant_id | uuid | yes | Tenant boundary |
| connector_code | varchar(128) | yes | 目标 Connector |
| operation | varchar(64) | yes | 业务操作（如 erp_purchase_request） |
| payload_json | jsonb | yes | 已审批的不可变请求体 |
| state | varchar(32) | yes | pending / sent / confirmed / compensate_pending / compensated / failed |
| external_request_id | varchar(255) | no | 供应商请求 ID |
| external_object_id | varchar(255) | no | 供应商业务单据 ID（补偿定位用） |
| attempts | int | yes | 投递尝试次数 |
| next_attempt_at | timestamptz | no | 下次投递时间（退避） |
| last_error | text | no | 最近失败原因 |
| created_at / updated_at | timestamptz | yes | — |

索引：`idx_connector_outbox_dispatchable`（state=pending 的 next_attempt_at 部分索引）、`idx_connector_outbox_state_scan`（state in sent/compensate_pending 的 updated_at）、`idx_connector_outbox_tool_call`。

外部请求通过 `tool_call_id` 关联 Run/Step/Audit，满足 M3 验收的溯源要求。

## M5 Target: eval_runs

记录被评估 `run_id`、dataset/evaluator version、dimension scores、outcome、evidence、cost 和时间。Eval 记录与生产 Run 分离，不覆盖原执行事实。

## M4 已实现: agent_memory (migration 024)

Agent 分层记忆存储，支持 Run/Thread/User/Team/Domain 五个层级，用于 Context Builder 上下文构建。

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| scope | varchar(32) | yes | 'run', 'thread', 'user', 'team', 'domain' |
| scope_id | varchar(255) | yes | 对应的 ID，如 run_id, user_id |
| tenant_id | uuid | yes | Tenant boundary |
| content | jsonb | yes | 记忆内容 (结构化数据) |
| acl | jsonb | no | 访问控制列表，为空/[] 表示仅创建者可见 |
| created_by | varchar(128) | yes | 创建者身份，ACL 为空时可见性判定依据 |
| expires_at | timestamptz | no | 过期时间，NULL 表示不过期 |
| created_at / updated_at | timestamptz | yes | — |
| deleted_at | timestamptz | no | 软删除 |

索引：`idx_agent_memory_scope` (scope, scope_id)、`idx_agent_memory_tenant` (tenant_id)、`idx_agent_memory_expires` (expires_at)。

## M4 已实现: skill_registry (migration 025)

Skill 版本化注册，支持 Draft -> Review -> Published -> Deprecated 生命周期。
附带 `skill:manage` 权限点（授予 platform_admin 与 business_reviewer）。

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| skill_code | varchar(64) | yes | 唯一业务标识，如 "finance_report_gen" |
| version | varchar(32) | yes | 语义化版本，如 "1.0.0" |
| status | varchar(32) | yes | 'draft', 'review', 'published', 'deprecated' |
| config_json | jsonb | yes | Skill 配置 (Prompt, Tools, Model Params) |
| created_by | varchar(128) | yes | 创建者 |
| reviewed_by | varchar(128) | no | 审核者 |
| published_at | timestamptz | no | 发布时间 |
| created_at / updated_at | timestamptz | yes | — |

Unique `(skill_code, version)` 保证版本不可变。

## M5 Target: trace_events

全链路追踪事件存储，覆盖 Workflow -> Run -> Model Turn -> Tool Call -> Checkpoint -> Interrupt 六层。

| Column | Type | Required | Notes |
|---|---|---:|---|
| id | uuid | yes | Primary key |
| trace_id | varchar(128) | yes | 唯一 Trace ID (如 agent_run_id) |
| layer | varchar(16) | yes | 'L1', 'L2', 'L3', 'L4', 'L5', 'L6' |
| parent_id | uuid | no | 父事件 ID，建立层级关系 |
| event_type | varchar(64) | yes | 'start', 'end', 'error', 'pause', 'resume' 等 |
| payload_json | jsonb | no | 事件详情 (LLM 输入/输出、工具参数等) |
| timestamp | timestamptz | yes | 事件发生时间 |
| duration_ms | int | no | 事件耗时 |
| tenant_id | uuid | yes | Tenant boundary |
| metadata_json | jsonb | no | 扩展元数据 |

索引：`idx_trace_events_trace_id` (trace_id)、`idx_trace_events_layer_timestamp` (layer, timestamp)、`idx_trace_events_tenant` (tenant_id)。
