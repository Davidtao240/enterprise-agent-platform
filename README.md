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

Health checks are configured for PostgreSQL, Redis, MinIO, Qdrant, Go backend, Python agent service, and frontend. Check status with:

```bash
docker compose ps
```

If image pulling fails with a Docker Hub or CloudFront `EOF` error, retry after the network stabilizes or configure a Docker registry mirror in Docker Desktop. This is an image download problem before the application containers are built.

### Option B: Local Code + Docker Infrastructure

Start only infrastructure:

```bash
docker compose up -d postgres redis minio qdrant
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

## Demo Users

Seed users all use password `password`:

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
```

You can also run each command independently when debugging a single layer.
