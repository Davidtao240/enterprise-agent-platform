#!/bin/bash
# ── Enterprise Agent Platform: Local Dev Start ──
#
# ⚠️  推荐使用 Docker 模式彻底解决代理问题：
#     ./scripts/dev-docker.sh
#
# 本地模式说明：
#   当系统配置了 HTTP 代理（如 127.0.0.1:7890）时，
#   浏览器访问 localhost:5173 会被代理拦截导致 502 错误。
#   本脚本通过设置 no_proxy 环境变量绕过本地流量。
#
#   如果本脚本无法解决代理干扰，请使用 Docker 模式。

set -e

# ── 配置 no_proxy，防止系统代理拦截本地开发流量 ──
LOCAL_PROXY_BYPASS="localhost,127.0.0.1,0.0.0.0,::1"
if [ -n "$no_proxy" ]; then
  export no_proxy="$no_proxy,$LOCAL_PROXY_BYPASS"
  export NO_PROXY="$NO_PROXY,$LOCAL_PROXY_BYPASS"
else
  export no_proxy="$LOCAL_PROXY_BYPASS"
  export NO_PROXY="$LOCAL_PROXY_BYPASS"
fi
echo "Proxy bypass: no_proxy=$no_proxy"

echo "=== 基础设施 (Docker) ==="
docker compose -f docker-compose.yml up -d postgres redis minio

echo "=== 等待 PostgreSQL ==="
until docker compose exec postgres pg_isready -U platform; do
  sleep 1
done

echo "=== 启动 Go Backend ==="
cd go-platform
go run ./cmd/server &
GO_PID=$!

echo "=== 启动 Python Agent Service ==="
cd ../agent-service
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000 &
AGENT_PID=$!

echo "=== 启动 Frontend ==="
cd ../frontend
npm install
npm run dev &
FRONTEND_PID=$!

echo ""
echo "=== 所有服务已启动 (本地模式) ==="
echo "  前端:  http://localhost:5173"
echo "  Go API: http://localhost:8080"
echo "  Agent:  http://localhost:8000"
echo "  MinIO:  http://localhost:9001"
echo ""
echo "  如果遇到网络错误，请使用 Docker 模式："
echo "    ./scripts/dev-docker.sh"
echo ""
echo "按 Ctrl+C 停止所有服务。"

trap "kill $GO_PID $AGENT_PID $FRONTEND_PID 2>/dev/null; docker compose -f docker-compose.yml stop" EXIT
wait
