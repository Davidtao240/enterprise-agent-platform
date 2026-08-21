# enterprise-agent-platform

Enterprise multi-agent workflow platform. V1 focuses on the finance operating report workflow:

```text
upload CSV/Excel -> create finance workflow -> run Python finance graph
-> human review -> archive -> audit logs
```

## Local Start

Create a local environment file first:

```bash
cp .env.example .env
```

For a full demo with real LLM output, set `LLM_API_KEY` in `.env`. If the key is missing or the LLM call fails, the Python report agents use a template-based fallback so the local Docker demo can still complete.

### Option A: Full Docker

Start Docker Desktop, then run:

```bash
docker compose up --build
```

Default services:

- Frontend: `http://localhost:3000`
- Go backend: `http://localhost:8080`
- Python agent service: `http://localhost:8000`
- MinIO console: `http://localhost:9001`

Health checks are configured for PostgreSQL (pgvector), Redis, MinIO, Go backend, Python agent service, and frontend. Check status with:

```bash
docker compose ps
```

If image pulling fails with a Docker Hub or CloudFront `EOF` error, retry after the network stabilizes or configure a Docker registry mirror in Docker Desktop. This is an image download problem before the application containers are built.

### Option B: Local Code + Docker Infrastructure

Start only infrastructure:

```bash
docker compose up -d postgres redis minio
```

Start the Go backend:

```bash
cd go-platform
go run ./cmd/server
```

Start the Python agent service:

```bash
cd agent-service
source .venv/bin/activate
uvicorn app.main:app --reload --port 8000
```

Start the frontend:

```bash
cd frontend
npm run dev
```

Local development frontend:

- `http://localhost:5173`

The Go backend and frontend Vite config both support loading the root `.env` during local development.

## V1 Feature Set

Current V1 capabilities:

- JWT login and backend-enforced RBAC.
- Finance V1 workflow demo with file upload, async workflow execution, Python agent graph calls, human approval, archive, and audit logs.
- Workflow detail view with node state, retry/cancel/start actions based on backend permissions, and Agent Run Log detail.
- Audit Logs operation view with trace, actor, business app, resource, action, status, time filters, statistics, and detail JSON.
- RBAC operation view with read-only permission matrix and user role summary.
- Registry operation view with read-only Business App, Domain Policy, Workflow Template, Agent Registry, and Tool Registry lists and JSON detail drawers.
- Local quality gate, Docker build gate, GitHub Actions CI, environment validation, and basic secret scanning.
- Configuration governance API for versioned Business App, Workflow Template, Agent, Tool, and Domain Policy changes. Changes move through draft, approval, publication, and deprecation with audit records.
- Workflow creation idempotency through the optional `Idempotency-Key` request header, plus cross-workflow retry protection.
- Platform observability summary, derived failure/latency alerts, and filtered audit CSV export.
- Tenant identity foundation: existing demo data belongs to a default tenant and JWTs carry the authenticated tenant identity.

## Deployment Readiness

Before a production-like deployment, copy `.env.example` to a deployment-specific secret store or environment manager and set the required variables there. Do not commit real secrets.

Required backend and infrastructure variables:

```text
JWT_SECRET
DB_HOST / DB_PORT / DB_USER / DB_PASSWORD / DB_NAME
REDIS_HOST / REDIS_PORT
MINIO_ENDPOINT / MINIO_ACCESS_KEY / MINIO_SECRET_KEY
AGENT_SERVICE_URL
LLM_PROVIDER / LLM_MODEL / LLM_BASE_URL / LLM_API_KEY
```

Development mode can run without `LLM_API_KEY` because the report agents fall back to template output. Production mode should provide a valid OpenAI-compatible Qwen or DeepSeek key and must not use weak development defaults such as `JWT_SECRET=change-me-in-production`, `DB_PASSWORD=platform_dev`, or `MINIO_SECRET_KEY=minioadmin`.

Run environment checks:

```bash
bash scripts/check-env.sh development
bash scripts/check-env.sh production
```

`scripts/check-env.sh` automatically loads the root `.env` when present and only fills variables that are not already exported. Set `EAP_ENV_FILE=/path/to/env` to validate another file, or export deployment variables directly in CI/production checks.

Deployment checklist:

- `docker compose config --quiet` succeeds.
- `docker compose build go-backend agent-service frontend` succeeds.
- `bash scripts/check.sh` succeeds.
- `bash scripts/security-check.sh` succeeds.
- `JWT_SECRET` is strong and deployment-specific.
- `LLM_API_KEY` is configured for production-like demos.
- Demo accounts have been removed, disabled, or rotated away from the development password.
- Database backups are scheduled and restore-tested.
- MinIO object storage uses persistent volumes and a backup policy.
- Redis queue data persistence is configured according to recovery requirements.
- Audit logs and application logs have retention and access controls.
- Tenant identities, backup scope, and data-retention policies have been reviewed for the deployment.
- An external OIDC identity provider is configured before enabling federated SSO; this repository currently uses local JWT login as its self-contained demo mode.
- Docker images are built from a reviewed commit and tagged immutably for release.
- Docker image pulls are stable; if Docker Hub or CloudFront returns `EOF`, retry on a stable network or configure a registry mirror.

## Demo Users

Seed users are for local development and scripted demos only. They all use password `password` and must not be used for production traffic:

- `admin`
- `finance_user`
- `finance_manager`
- `ops_viewer`

## Finance V1 Demo

Run the scripted smoke flow after the backend and agent service are available:

```bash
bash scripts/demo-finance-v1-curl.sh
```

On Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/demo-finance-v1.ps1
```

The script performs:

```text
login -> upload CSV -> create workflow with file_id -> start workflow
-> wait for pending approval -> approve -> wait for archived
-> query audit logs
```

Both scripts use the V1.2 login response field `access_token` and RBAC-protected APIs.

## Operations Views

After login, available menu items depend on backend RBAC permissions:

- `Audit Logs`: trace search, actor/resource filters, status/action statistics, and detail JSON.
- `RBAC`: read-only role-permission matrix and user-role-permission summary.
- `Registry`: read-only Business App, Domain Policy, Workflow Template, Agent Registry, and Tool Registry views with JSON detail drawers.

These pages are operational views only. Role assignment, permission editing, workflow template editing, domain policy editing, and registry mutation remain out of scope for V1/V2.0 read-only governance.

## V2 Operations APIs

V2.1 exposes versioned configuration governance through `/api/v1/configuration-versions`. Only the five platform extension-point resource types are accepted: `business_app`, `workflow_template`, `agent`, `tool`, and `domain_policy`. Create a draft, submit it, have another authorized user publish it, or deprecate a published version. The API is deliberately domain-neutral and each transition writes an audit record.

V2.2 accepts an optional `Idempotency-Key` header on `POST /api/v1/workflow-instances`. Reusing the same non-empty key for the same user returns the original workflow instead of creating duplicate nodes or agent work.

V2.3 provides `GET /api/v1/platform-observability/summary` for operational counters and derived alerts, and `GET /api/v1/audit-logs/export` for a filtered CSV export (capped at 10,000 rows).

V2.4 establishes a `tenants` table and includes `tenant_id` in every issued JWT. The platform remains in local-JWT demo mode until a real OIDC provider is configured; do not treat the default demo tenant as a production SSO implementation.

## Quality Check

Run the full local verification gate:

```bash
bash scripts/check.sh
```

This executes:

```bash
cd go-platform && go test ./...
cd agent-service && .venv/bin/python -m compileall app
cd agent-service && .venv/bin/python -m unittest discover -s tests
cd frontend && npm run build
bash scripts/security-check.sh
```

Run Docker build verification as part of the same local gate:

```bash
WITH_DOCKER=1 bash scripts/check.sh
```

CI runs the same language-level gates and a Docker build job through GitHub Actions in `.github/workflows/ci.yml`.

The security check scans tracked and unignored files for accidentally committed `.env` files, common real-secret patterns, and weak production defaults. Ignored local files such as `.env` are not scanned.

You can also run each command independently when debugging a single layer.
