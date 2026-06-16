# Workflow State Machine

## Purpose

This document defines the platform-level workflow state machine. It must be reusable across finance, HR, legal, procurement, IT service, customer service, and future scenarios.

Business differences are represented by workflow templates, node definitions, edges, agent graphs, and domain policies. They must not be hard-coded into the Workflow Engine.

## Workflow Instance Status

| Status | Meaning |
|---|---|
| draft | Created but not started |
| running | Workflow is executing |
| waiting_review | Workflow is waiting for human approval |
| approved | Human approval has approved the main result |
| rejected | Human approval has rejected the main result |
| archived | Final result has been archived |
| failed | Workflow cannot continue due to an error |
| cancelled | Workflow was cancelled by an authorized user |

## Workflow Node Status

| Status | Meaning |
|---|---|
| pending | Node has not started |
| running | Node is executing |
| succeeded | Node completed successfully |
| failed | Node failed |
| skipped | Node was skipped by edge condition |
| waiting_review | Node created an approval task and is waiting |
| cancelled | Node was cancelled |

## Node Types

| Type | Responsibility |
|---|---|
| file_upload | Collect files or attachments |
| agent_graph | Call Python Agent Graph through Go Agent Gateway |
| human_review | Create approval task and wait for decision |
| system | Archive, notify, write final result, or other backend actions |

## Workflow Lifecycle

```text
draft
-> running
-> waiting_review
-> approved
-> archived
```

Failure path:

```text
running
-> failed
```

Rejection path:

```text
waiting_review
-> rejected
```

Cancellation path:

```text
draft | running | waiting_review
-> cancelled
```

## Start Rules

- Only `draft` workflows can be started.
- User must have workflow start permission for the business app.
- Required input must be present before start.
- Starting a workflow changes workflow status to `running`.
- Starting a workflow schedules the first executable node.
- Start action must create an audit log.

## Node Execution Rules

1. Workflow Engine reads `workflow_templates.definition_json`.
2. It finds nodes whose incoming conditions are satisfied.
3. It creates or updates `workflow_node_instances`.
4. It runs nodes according to node type.
5. It persists every state transition.
6. It writes audit logs for critical actions.

The engine must interpret templates. It must not contain business-specific `if finance`, `if hr`, or `if legal` branches.

## file_upload Node

- Usually completed by user action before workflow start.
- If required file is missing, workflow cannot start.
- On success, node status becomes `succeeded`.
- Uploaded file metadata is stored in `files`.

## agent_graph Node

Execution flow:

```text
pending
-> running
-> succeeded | failed
```

Rules:

- Node must contain `graph_key`.
- Go Agent Gateway validates graph, domain policy, and permissions.
- Python Agent Service returns structured JSON.
- Go backend validates output before updating node status.
- Agent run must be recorded in `agent_run_logs`.

## human_review Node

Execution flow:

```text
pending
-> waiting_review
-> succeeded | failed
```

Rules:

- Node creates an `approval_task`.
- Workflow status changes to `waiting_review`.
- Approval moves node to `succeeded`.
- Rejection moves node according to template edge rules.
- Decision must be recorded in audit logs.

## system Node

Used for backend-controlled actions such as archive or notification.

Rules:

- System node must be idempotent.
- System node must write audit logs for final state changes.
- Archive node sets workflow status to `archived`.

## Retry Rules

- Only `failed` nodes can be retried.
- Node must be marked retryable by template or node type.
- `retry_count` must be less than `max_retries`.
- Retrying a node creates a new execution attempt.
- `trace_id` remains the same.
- Agent `run_id` changes for every retry.
- Retry action must create an audit log.

## Cancellation Rules

- `draft`, `running`, and `waiting_review` workflows can be cancelled by authorized users.
- Running async jobs should be cancelled if possible.
- Active pending nodes become `cancelled`.
- Pending approval tasks become `cancelled`.
- Archived workflows cannot be cancelled.

## Rejection Rules

- Rejection requires a comment.
- Rejection records `decision_by`, `decision_comment`, and `decided_at`.
- The workflow template determines whether rejection ends the workflow or returns to a previous node.
- V1 finance rejection ends the workflow as `rejected`.

## Archive Rules

- Only approved or otherwise completed workflows can be archived.
- Archived workflows are read-only.
- Archive action stores final output and report file metadata.
- Archive action must write audit logs.

## Audit Triggers

Audit logs must be created for:

- User login.
- Workflow creation.
- Workflow start.
- File upload.
- Agent graph execution request.
- Node failure.
- Node retry.
- Approval task creation.
- Approval or rejection decision.
- Workflow cancellation.
- Workflow archive.
- Agent or tool configuration changes.

## V1 Finance Example

Template flow:

```text
upload
-> agent_graph
-> human_review
-> archive
```

Status example:

```text
workflow: draft
upload: pending

workflow: running
upload: succeeded
agent_graph: running

workflow: waiting_review
agent_graph: succeeded
human_review: waiting_review

workflow: archived
human_review: succeeded
archive: succeeded
```
