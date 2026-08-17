#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tracked_env_files="$(git ls-files --cached --others --exclude-standard | grep -E '(^|/)\.env($|\.local$|\.production$|\.prod$|\.development$|\.dev$)' || true)"
if [ -n "$tracked_env_files" ]; then
  echo "Tracked or unignored .env-style files are not allowed:" >&2
  echo "$tracked_env_files" >&2
  exit 1
fi

scan_files="$(git ls-files --cached --others --exclude-standard | grep -Ev '(^|/)(go\.sum|package-lock\.json)$' | grep -Ev '^docs/' || true)"

if [ -n "$scan_files" ]; then
  if echo "$scan_files" | xargs grep -nE '(sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{20,})' >/tmp/eap-secret-hits.txt 2>/dev/null; then
    echo "Potential real secret detected in tracked files:" >&2
    cat /tmp/eap-secret-hits.txt >&2
    rm -f /tmp/eap-secret-hits.txt
    exit 1
  fi
fi
rm -f /tmp/eap-secret-hits.txt

if grep -R "JWT_SECRET=change-me-in-production" .github scripts docker-compose.yml README.md .env.example >/dev/null 2>&1; then
  : # documented development placeholder is allowed
fi

if env \
  EAP_ENV_FILE=/dev/null \
  JWT_SECRET=change-me-in-production \
  DB_HOST=postgres \
  DB_PORT=5432 \
  DB_USER=platform \
  DB_PASSWORD=platform_dev \
  DB_NAME=enterprise_agent_platform \
  REDIS_HOST=redis \
  REDIS_PORT=6379 \
  MINIO_ENDPOINT=minio:9000 \
  MINIO_ACCESS_KEY=minioadmin \
  MINIO_SECRET_KEY=minioadmin \
  AGENT_SERVICE_URL=http://agent-service:8000 \
  LLM_PROVIDER=qwen \
  LLM_MODEL=qwen-plus \
  LLM_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1 \
  LLM_API_KEY=dummy \
  bash scripts/check-env.sh production >/tmp/eap-weak-prod-check.txt 2>&1; then
  echo "Production environment check accepted weak defaults unexpectedly." >&2
  cat /tmp/eap-weak-prod-check.txt >&2
  rm -f /tmp/eap-weak-prod-check.txt
  exit 1
fi
rm -f /tmp/eap-weak-prod-check.txt

if env \
  EAP_ENV_FILE=/dev/null \
  JWT_SECRET=strong-jwt-secret-for-token-check \
  DB_HOST=postgres \
  DB_PORT=5432 \
  DB_USER=platform \
  DB_PASSWORD=strong-db-password \
  DB_NAME=enterprise_agent_platform \
  REDIS_HOST=redis \
  REDIS_PORT=6379 \
  MINIO_ENDPOINT=minio:9000 \
  MINIO_ACCESS_KEY=strong-minio-access \
  MINIO_SECRET_KEY=strong-minio-secret \
  AGENT_SERVICE_URL=http://agent-service:8000 \
  LLM_PROVIDER=qwen \
  LLM_MODEL=qwen-plus \
  LLM_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1 \
  LLM_API_KEY=strong-llm-key \
  INTERNAL_SERVICE_TOKEN=replace-with-a-random-service-token \
  bash scripts/check-env.sh production >/tmp/eap-weak-token-check.txt 2>&1; then
  echo "Production environment check accepted the example INTERNAL_SERVICE_TOKEN placeholder." >&2
  cat /tmp/eap-weak-token-check.txt >&2
  rm -f /tmp/eap-weak-token-check.txt
  exit 1
fi
rm -f /tmp/eap-weak-token-check.txt

echo "Security check passed."
