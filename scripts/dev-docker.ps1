# ── Enterprise Agent Platform: Docker Dev Start (Windows PowerShell) ──
#
# 彻底解决系统代理干扰问题的官方启动方式。
# 将所有服务放入 Docker 容器网络，服务间通过容器名通信，
# 浏览器仅通过 localhost:5173 访问（Docker 端口映射），
# 请求不经过宿主机 HTTP 代理。
#
# 用法：
#   .\scripts\dev-docker.ps1 [-Rebuild]

param(
  [switch]$Rebuild
)

$ErrorActionPreference = "Stop"

# 切换到项目根目录
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

# 检查 .env 文件
if (-not (Test-Path ".env")) {
  Write-Host "⚠️  .env 文件不存在，正在从 .env.example 复制..."
  Copy-Item .env.example .env
  Write-Host "✅ 已创建 .env，请填写必要的密钥后重新启动。"
  exit 1
}

# 检查必要的密钥变量
$requiredVars = @("JWT_SECRET", "INTERNAL_SERVICE_TOKEN", "TOOL_SECRET_ENCRYPTION_KEY")
$envContent = Get-Content ".env" -Raw
$missing = @()

foreach ($var in $requiredVars) {
  $pattern = "(?m)^$var\s*=\s*(.+)$"
  $match = [regex]::Match($envContent, $pattern)
  if (-not $match.Success -or $match.Groups[1].Value -match "<SET-A") {
    $missing += $var
  }
}

if ($missing.Count -gt 0) {
  Write-Host "❌ .env 中缺少必要的密钥变量：$($missing -join ', ')"
  Write-Host "   请编辑 .env 文件，设置这些变量为强随机值后重试。"
  exit 1
}

Write-Host "=== 企业智能体平台 (Docker 开发模式) ==="
Write-Host ""
Write-Host "架构说明："
Write-Host "  浏览器 -> localhost:5173 -> Docker 端口映射 -> 容器内部网络 -> go-backend:8080"
Write-Host "  所有流量在 Docker bridge 网络内流转，完全绕过系统代理。"
Write-Host ""

if ($Rebuild) {
  Write-Host "=== 重建所有镜像 ==="
  docker compose -f docker-compose.dev.yml build --no-cache
}

Write-Host "=== 启动所有服务 ==="
docker compose -f docker-compose.dev.yml up -d

Write-Host ""
Write-Host "=== 等待服务就绪 ==="

# 等待 Go Backend 健康
$maxWait = 60
$wait = 0
while ($wait -lt $maxWait) {
  try {
    $response = docker compose -f docker-compose.dev.yml exec -T go-backend wget -qO- http://localhost:8080/health 2>&1
    if ($LASTEXITCODE -eq 0) {
      Write-Host "✅ Go Backend 已就绪"
      break
    }
  } catch {}
  Start-Sleep -Seconds 2
  $wait += 2
  Write-Host -NoNewline "."
}

if ($wait -ge $maxWait) {
  Write-Host ""
  Write-Host "⚠️  Go Backend 启动超时，但容器可能仍在初始化。"
  Write-Host "   使用 'docker compose -f docker-compose.dev.yml logs -f go-backend' 查看日志。"
}

Write-Host ""
Write-Host "=== 所有服务状态 ==="
docker compose -f docker-compose.dev.yml ps

Write-Host ""
Write-Host "🎉  启动完成！访问地址："
Write-Host ""
Write-Host "  前端工作台:  http://localhost:5173"
Write-Host "  Go API:      http://localhost:8080"
Write-Host "  Agent Svc:   http://localhost:8000"
Write-Host "  MinIO 控制台: http://localhost:9001"
Write-Host ""
Write-Host "  测试账号（密码均为 'password'）："
Write-Host "    admin / finance_user / finance_manager / ops_viewer"
Write-Host ""
Write-Host "  停止服务：docker compose -f docker-compose.dev.yml down"
Write-Host "  查看日志：docker compose -f docker-compose.dev.yml logs -f"
Write-Host "  进入前端容器：docker compose -f docker-compose.dev.yml exec frontend-dev sh"
