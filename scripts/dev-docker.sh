#!/bin/bash
# ── Enterprise Agent Platform: Docker Dev Start ──
#
# 彻底解决系统代理干扰问题的官方启动方式。
# 将所有服务放入 Docker 容器网络，服务间通过容器名通信，
# 浏览器仅通过 localhost:5173 访问（Docker 端口映射），
# 请求不经过宿主机 HTTP 代理。
#
# 用法：
#   ./scripts/dev-docker.sh [--rebuild]
#     --rebuild  强制重建镜像（代码有变更时使用）

set -e

cd "$(dirname "$0")/.."

REBUILD=false
if [[ "$1" == "--rebuild" ]]; then
  REBUILD=true
fi

# 检查 .env 文件
if [ ! -f ".env" ]; then
  echo "⚠️  .env 文件不存在，正在从 .env.example 复制..."
  cp .env.example .env
  echo "✅ 已创建 .env，请填写必要的密钥后重新启动。"
  exit 1
fi

# 检查必要的密钥变量
REQUIRED_VARS=("JWT_SECRET" "INTERNAL_SERVICE_TOKEN" "TOOL_SECRET_ENCRYPTION_KEY")
MISSING=()
for var in "${REQUIRED_VARS[@]}"; do
  if ! grep -q "^${var}=[^[:space:]]" .env || grep -q "<SET-A" .env; then
    MISSING+=("$var")
  fi
done

if [ ${#MISSING[@]} -gt 0 ]; then
  echo "❌ .env 中缺少必要的密钥变量：${MISSING[*]}"
  echo "   请编辑 .env 文件，设置这些变量为强随机值后重试。"
  exit 1
fi

echo "=== 企业智能体平台 (Docker 开发模式) ==="
echo ""
echo "架构说明："
echo "  浏览器 → localhost:5173 → Docker 端口映射 → 容器内部网络 → go-backend:8080"
echo "  所有流量在 Docker bridge 网络内流转，完全绕过系统代理。"
echo ""

if [ "$REBUILD" = true ]; then
  echo "=== 重建所有镜像 ==="
  docker compose -f docker-compose.dev.yml build --no-cache
fi

echo "=== 启动所有服务 ==="
docker compose -f docker-compose.dev.yml up -d

echo ""
echo "=== 等待服务就绪 ==="
echo "  (这可能需要 30-60 秒，取决于首次构建)"

# 等待 Go Backend 健康
MAX_WAIT=60
WAIT=0
while [ $WAIT -lt $MAX_WAIT ]; do
  if docker compose -f docker-compose.dev.yml exec -T go-backend wget -qO- http://localhost:8080/health >/dev/null 2>&1; then
    echo "✅ Go Backend 已就绪"
    break
  fi
  sleep 2
  WAIT=$((WAIT + 2))
  echo -n "."
done

if [ $WAIT -ge $MAX_WAIT ]; then
  echo ""
  echo "⚠️  Go Backend 启动超时，但容器可能仍在初始化。"
  echo "   使用 'docker compose -f docker-compose.dev.yml logs -f go-backend' 查看日志。"
fi

echo ""
echo "=== 所有服务状态 ==="
docker compose -f docker-compose.dev.yml ps

echo ""
echo "🎉  启动完成！访问地址："
echo ""
echo "  前端工作台:  http://localhost:5173"
echo "  Go API:      http://localhost:8080"
echo "  Agent Svc:   http://localhost:8000"
echo "  MinIO 控制台: http://localhost:9001"
echo ""
echo "  测试账号（密码均为 'password'）："
echo "    admin / finance_user / finance_manager / ops_viewer"
echo ""
echo "  停止服务：docker compose -f docker-compose.dev.yml down"
echo "  查看日志：docker compose -f docker-compose.dev.yml logs -f"
echo "  进入前端容器：docker compose -f docker-compose.dev.yml exec frontend-dev sh"
