# User Stories

## Purpose

This document defines the user-facing workflows for the enterprise multi-agent workflow automation platform.

V1 implements the finance operating report workflow end to end. Future business scenarios must reuse the same platform abstractions: Business App, Workflow Template, Workflow Instance, Agent Gateway, Approval Task, Audit Log, and Agent Run Log.

## Roles

### Platform Admin

Manages users, departments, roles, permissions, business apps, workflow templates, agents, tools, and domain policies.

### Business User

Creates workflow instances, uploads business materials, tracks workflow progress, and views final results.

### Business Reviewer

Reviews AI-generated outputs, approves or rejects approval tasks, and leaves review comments.

### Ops Viewer

Views workflow execution status, agent run logs, audit logs, and failure details for troubleshooting.

## Platform User Stories

### US-001 Login

As an enterprise user, I want to log in with my account so that I can access authorized business apps.

Acceptance criteria:

- User can log in with username and password.
- API returns a JWT token after successful login.
- User permissions are loaded after login.
- Unauthorized users cannot access protected APIs.

### US-002 View Business Apps

As a business user, I want to view available business apps so that I can enter the right business workspace.

Acceptance criteria:

- Frontend loads business apps from `GET /api/v1/business-apps`.
- V1 returns `finance`.
- Frontend does not hard-code finance as the only possible business app.
- Hidden or inactive business apps are not displayed.

### US-003 Create Workflow Instance

As a business user, I want to create a workflow task from a workflow template so that the platform can execute a business process.

Acceptance criteria:

- User selects `business_app_code` and `workflow_template_key`.
- Backend validates that the template belongs to the selected business app.
- Backend creates a `workflow_instance` in `draft` status.
- Backend creates initial `workflow_node_instances` from template definition.
- Audit log records workflow creation.

### US-004 Upload File

As a business user, I want to upload source files so that agents can process structured or unstructured business data.

Acceptance criteria:

- User can upload CSV or Excel files for V1 finance workflow.
- Backend stores file binary in MinIO.
- Backend stores file metadata in `files`.
- File is linked to the workflow instance.
- Audit log records file upload.

### US-005 Start Workflow

As a business user, I want to start a draft workflow so that the workflow engine can execute nodes.

Acceptance criteria:

- Only authorized users can start a workflow.
- Workflow status changes from `draft` to `running`.
- First executable node changes from `pending` to `running`.
- Async job is dispatched through Redis and Asynq.
- Audit log records workflow start.

### US-006 View Workflow Detail

As a business user, I want to view workflow progress so that I can understand current status and next actions.

Acceptance criteria:

- Frontend displays workflow instance status.
- Frontend displays all workflow nodes in execution order.
- Each node displays status, started time, finished time, and failure reason if any.
- Agent graph node displays agent run summary.
- Human review node displays pending approval task when applicable.

### US-007 Retry Failed Node

As an ops viewer or authorized business user, I want to retry failed nodes so that transient failures can be recovered.

Acceptance criteria:

- Only failed retryable nodes can be retried.
- Retry creates a new execution attempt.
- `trace_id` remains stable for the workflow.
- `run_id` changes for each new agent run.
- Audit log records retry action.

### US-008 Approve Task

As a business reviewer, I want to approve an AI-generated result so that the workflow can continue to archive.

Acceptance criteria:

- Reviewer can view report, agent summary, warnings, and source file metadata.
- Reviewer can approve with optional comments.
- Approval task status changes to `approved`.
- Workflow continues to the next node.
- Audit log records approval action.

### US-009 Reject Task

As a business reviewer, I want to reject an AI-generated result so that incorrect or risky outputs are not archived.

Acceptance criteria:

- Reviewer can reject with required comments.
- Approval task status changes to `rejected`.
- Workflow status changes according to template edge rules.
- Rejected result is not archived as final.
- Audit log records rejection action.

### US-010 View Audit Logs

As an admin or ops viewer, I want to view audit logs so that I can trace who did what and when.

Acceptance criteria:

- Audit logs are filterable by workflow, user, action, business app, and time range.
- Logs include actor, action, resource type, resource id, status, trace id, and timestamp.
- Critical actions always create audit logs.

## V1 Finance User Stories

### F-US-001 Create Finance Operating Report Task

As a finance user, I want to create a monthly operating data report task so that AI agents can analyze business data.

Acceptance criteria:

- User enters title, month, department, and optional description.
- User uploads CSV or Excel operating data.
- Workflow template is `finance_operating_report`.
- Workflow instance is created under business app `finance`.

### F-US-002 Run Finance Agent Analysis

As a finance user, I want the system to analyze uploaded data so that I can receive a structured report draft.

Acceptance criteria:

- Workflow engine executes `agent_graph` node.
- Go Agent Gateway calls Python graph `finance_operating_report_graph`.
- Python graph runs data extraction, schema mapping, validation, finance analysis, report generation, and review summary.
- Agent output is structured JSON.
- Agent run logs include run id, trace id, status, token usage, duration, and error if failed.

### F-US-003 Review Finance Report

As a finance manager, I want to review the AI-generated finance report so that only confirmed reports are archived.

Acceptance criteria:

- Finance manager sees key metrics, summary, warnings, and report preview.
- Finance manager can approve or reject.
- Approved reports are archived.
- Rejected reports remain traceable with rejection comments.

## Future Scenario Placeholders

Future scenarios must be added one by one after V1. Each future scenario must define its own workflow template, graph key, business form schema, domain agents, tools, domain policy, and custom result view if needed.

Planned scenarios:

- HR onboarding material review.
- Legal contract review and approval.
- Procurement request approval.
- IT service ticket handling.
- Customer service ticket quality review.
