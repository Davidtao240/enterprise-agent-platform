# enterprise-agent-platform

Enterprise multi-agent workflow platform. V1 focuses on the finance operating report workflow:

```text
upload CSV/Excel -> create finance workflow -> run Python finance graph
-> human review -> archive -> audit logs
```

## Local Start

```powershell
docker compose up --build
```

Default services:

- Frontend: `http://localhost:3000`
- Go backend: `http://localhost:8080`
- Python agent service: `http://localhost:8000`

Seed users all use password `password`:

- `finance_user`
- `finance_manager`
- `admin`

## Finance V1 Demo

Run the scripted smoke flow after `docker compose up --build`:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/demo-finance-v1.ps1
```

Or use the curl-based script:

```bash
bash scripts/demo-finance-v1-curl.sh
```

The script performs:

```text
login -> upload CSV -> create workflow with file_id -> start workflow
-> wait for pending approval -> approve -> wait for archived
-> query audit logs
```

## Verification Commands

Go backend:

```powershell
cd go-platform
go test ./...
```

Python agent service:

```powershell
cd agent-service
python -m compileall app
python -m unittest discover -s tests
```

Frontend build requires dependencies:

```powershell
cd frontend
npm install
npm run build
```
