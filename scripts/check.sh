#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "== Go tests =="
(cd "$ROOT_DIR/go-platform" && go test ./...)

echo "== Python compile =="
(cd "$ROOT_DIR/agent-service" && .venv/bin/python -m compileall app)

echo "== Python tests =="
(cd "$ROOT_DIR/agent-service" && .venv/bin/python -m unittest discover -s tests)

echo "== Frontend build =="
(cd "$ROOT_DIR/frontend" && npm run build)

echo "== All checks passed =="
