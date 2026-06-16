#!/bin/bash
# Local development startup script.

set -e

echo "=== Starting Infrastructure ==="
docker compose -f docker-compose.yml up -d postgres redis minio qdrant

echo "=== Waiting for PostgreSQL ==="
until docker compose exec postgres pg_isready -U platform; do
  sleep 1
done

echo "=== Running Migrations ==="
# TODO: Replace with golang-migrate CLI call
echo "Skipping (golang-migrate not installed locally)"

echo "=== Starting Go Backend ==="
cd go-platform
go run ./cmd/server &
GO_PID=$!

echo "=== Starting Python Agent Service ==="
cd ../agent-service
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000 &
AGENT_PID=$!

echo "=== Starting Frontend ==="
cd ../frontend
npm install
npm run dev &
FRONTEND_PID=$!

echo ""
echo "=== All services started ==="
echo "Frontend:  http://localhost:5173"
echo "Go API:    http://localhost:8080"
echo "Agent Svc: http://localhost:8000"
echo "MinIO:     http://localhost:9001"
echo ""
echo "Press Ctrl+C to stop all services."

trap "kill $GO_PID $AGENT_PID $FRONTEND_PID 2>/dev/null; docker compose stop" EXIT
wait
