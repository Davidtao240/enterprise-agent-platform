# PowerShell: Local development startup script for Windows.

$ErrorActionPreference = "Stop"

# ── Configure no_proxy to prevent system proxy from intercepting localhost
# traffic. Without this, system proxy (e.g. 127.0.0.1:7890) returns 502 on
# http://localhost:5173 and the frontend shows "net work error".
$localBypass = "localhost,127.0.0.1,0.0.0.0"
if ($env:no_proxy) {
  $env:no_proxy = "$env:no_proxy,$localBypass"
  $env:NO_PROXY = "$env:NO_PROXY,$localBypass"
} else {
  $env:no_proxy = $localBypass
  $env:NO_PROXY = $localBypass
}
Write-Host "Proxy bypass: no_proxy=$env:no_proxy"

Write-Host "=== Starting Infrastructure ==="
docker compose -f docker-compose.yml up -d postgres redis minio

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
