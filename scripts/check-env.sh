#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-development}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${EAP_ENV_FILE:-$ROOT_DIR/.env}"

if [ -f "$ENV_FILE" ]; then
  while IFS='=' read -r key value; do
    if [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] && [ -z "${!key:-}" ]; then
      export "$key=$value"
    fi
  done < <(grep -Ev '^\s*(#|$)' "$ENV_FILE")
fi

required=(
  JWT_SECRET
  DB_HOST
  DB_PORT
  DB_USER
  DB_PASSWORD
  DB_NAME
  REDIS_HOST
  REDIS_PORT
  MINIO_ENDPOINT
  MINIO_ACCESS_KEY
  MINIO_SECRET_KEY
  AGENT_SERVICE_URL
  LLM_PROVIDER
  LLM_MODEL
  LLM_BASE_URL
)

optional_dev=(
  LLM_API_KEY
)

missing=()
for key in "${required[@]}"; do
  if [ -z "${!key:-}" ]; then
    missing+=("$key")
  fi
done

if [ "${#missing[@]}" -gt 0 ]; then
  printf 'Missing required environment variables for %s mode:\n' "$MODE" >&2
  printf '  - %s\n' "${missing[@]}" >&2
  if [ ! -f "$ENV_FILE" ]; then
    printf 'No environment file was loaded. Create .env from .env.example or set EAP_ENV_FILE.\n' >&2
  fi
  exit 1
fi

if [ "$MODE" = "production" ] || [ "$MODE" = "prod" ]; then
  if [ -z "${LLM_API_KEY:-}" ]; then
    echo "Missing required environment variable for production mode: LLM_API_KEY" >&2
    exit 1
  fi
  if [ "${LLM_API_KEY:-}" = "your-api-key-here" ]; then
    echo "LLM_API_KEY must not use the example placeholder in production mode." >&2
    exit 1
  fi
  if [ "${JWT_SECRET:-}" = "change-me-in-production" ]; then
    echo "JWT_SECRET must not use the development default in production mode." >&2
    exit 1
  fi
  if [ "${JWT_SECRET:-}" = "change-me" ]; then
    echo "JWT_SECRET is too weak for production mode." >&2
    exit 1
  fi
  if [ "${DB_PASSWORD:-}" = "platform_dev" ]; then
    echo "DB_PASSWORD must not use the development default in production mode." >&2
    exit 1
  fi
  if [ "${MINIO_SECRET_KEY:-}" = "minioadmin" ]; then
    echo "MINIO_SECRET_KEY must not use the development default in production mode." >&2
    exit 1
  fi
  if [ -z "${INTERNAL_SERVICE_TOKEN:-}" ]; then
    echo "Missing required environment variable for production mode: INTERNAL_SERVICE_TOKEN" >&2
    echo "Runtime V2 service authentication will fail closed (503), breaking Durable Run event delivery." >&2
    exit 1
  fi
  if [ "${INTERNAL_SERVICE_TOKEN:-}" = "replace-with-a-random-service-token" ]; then
    echo "INTERNAL_SERVICE_TOKEN must not use the example placeholder in production mode." >&2
    exit 1
  fi
  if [ "${INTERNAL_SERVICE_TOKEN:-}" = "m1-local-acceptance-token" ]; then
    echo "INTERNAL_SERVICE_TOKEN must not use the documented local acceptance token in production mode." >&2
    exit 1
  fi
  echo "Warning: V1 seed demo users use the development password. Rotate or remove demo accounts before production traffic." >&2
else
  for key in "${optional_dev[@]}"; do
    if [ -z "${!key:-}" ]; then
      echo "Warning: $key is not set. Development mode will rely on template fallback where supported." >&2
    fi
  done
fi

echo "Environment check passed for $MODE mode."
