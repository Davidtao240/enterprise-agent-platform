# PowerShell: Local development startup script for Windows.

$ErrorActionPreference = "Stop"

Write-Host "=== Starting Infrastructure ==="
docker compose -f docker-compose.yml up -d postgres redis minio qdrant

Write-Host "=== Waiting for PostgreSQL ==="
Start-Sleep -Seconds 5

Write-Host ""
Write-Host "=== All services started ==="
Write-Host "Infrastructure only (use separate terminals for app servers):"
Write-Host "  cd go-platform && go run ./cmd/server"
Write-Host "  cd agent-service && pip install -r requirements.txt && uvicorn app.main:app --reload --port 8000"
Write-Host "  cd frontend && npm install && npm run dev"
Write-Host ""
Write-Host "Frontend:  http://localhost:5173"
Write-Host "Go API:    http://localhost:8080"
Write-Host "Agent Svc: http://localhost:8000"
Write-Host "MinIO:     http://localhost:9001"
