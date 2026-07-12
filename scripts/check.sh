#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WITH_DOCKER="${WITH_DOCKER:-0}"

echo "== Go tests =="
(cd "$ROOT_DIR/go-platform" && go test ./...)

echo "== Python compile =="
(cd "$ROOT_DIR/agent-service" && .venv/bin/python -m compileall app)

echo "== Python tests =="
(cd "$ROOT_DIR/agent-service" && .venv/bin/python -m unittest discover -s tests)

echo "== Frontend build =="
(cd "$ROOT_DIR/frontend" && npm run build)

echo "== Security check =="
(cd "$ROOT_DIR" && bash scripts/security-check.sh)

if [ "$WITH_DOCKER" = "1" ]; then
  echo "== Docker Compose config =="
  (cd "$ROOT_DIR" && docker compose config --quiet)

  echo "== Docker build =="
  (cd "$ROOT_DIR" && docker compose build go-backend agent-service frontend)
fi

echo "== All checks passed =="
