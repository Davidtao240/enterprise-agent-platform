# 新业务域接入 SOP v1.0

> 适用对象：平台工程师、解决方案架构师、客户交付团队
> 版本：v1.0
> 最后更新：2026-08-19
> 前置条件：平台已完成 M8（Agent Package + Skill Marketplace + Connector Protocol + Knowledge Base）

## 概述

本 SOP 定义在不修改平台核心代码（`go-platform/internal/`、`agent-service/app/core/`）的前提下，将新业务域（如采购、HR、法务、IT、客服）接入平台的完整流程。

### 核心原则

- **平台业务域中立（Business-Domain Neutral）**：平台不硬编码任何业务域逻辑，所有业务域通过配置和注册接入
- **扩展通过配置 + 注册完成，不写 if/else**：新增业务域不触发核心代码变更
- **每一步均可独立验收和回滚**：出错时可精确定位到具体步骤
- **安全默认拒绝（Default Deny），逐步授权**：新业务域默认无任何跨域权限，按需最小授权
- **所有操作审计可追溯**：从接入第一天起，每个 API 调用、权限变更、数据流转均有审计日志

## 7 步接入流程总览

```
Step 1: 创建 Business App          → 注册业务域元数据，获取唯一编码
Step 2: 绑定 Domain Policy         → 配置跨域访问规则，建立隔离边界
Step 3: 注册 Connector/Sidecar     → 接入外部系统（ERP、OA、第三方服务）
Step 4: 发布 Skill（可选）          → 安装 Shared Skill 或创建 Domain Skill
Step 5: 打包 Agent Package         → 封装 Agent Graph 为可分发包
Step 6: 上架 Agent Gallery          → 发布到 Gallery 供用户发现和使用
Step 7: 验收与归档                 → 完整走通端到端流程，归档所有文档
```

流程图：

```
┌─────────────────────────────────────────────────────────────────┐
│                        新业务域接入流程                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐
│  │ Step1 │──▶│ Step2 │──▶│ Step3 │──▶│ Step4 │──▶│ Step5 │──▶│ Step6 │
│  │Business│  │Domain │  │Sidecar│  │ Skill │  │Package│  │Gallery│
│  │ App   │   │Policy │  │       │  │       │  │       │  │       │
│  └──────┘   └──────┘   └──────┘   └──────┘   └──────┘   └──────┘
│      │                                                        │
│      └────────────────────────────────────────────────────────┐│
│                                                                ┌──────┐       ││
│                                └──────┘       ││
│                                ┌──────┐       ││
│                                │ Step7 │◀──────┘│
│                                │验收归档│        │
│                                └──────┘          │
│                                                   ▼
│                                          [回滚方案] 反序执行
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Step 1: 创建 Business App

### 1.1 准备元数据

在调用 API 前，先准备以下元数据字段：

| 字段 | 类型 | 说明 | 示例（采购） |
|------|------|------|-------------|
| `code` | string | 唯一编码，小写+下划线格式，全局唯一 | `procurement` |
| `name` | string | 业务域显示名称，面向用户 | 采购报价审查 |
| `description` | string | 业务域功能描述，用于 Gallery 展示 | 标准化并比较供应商报价，整理审查证据，并强制进入人工审核 |
| `domain` | string | 所属业务域分类，用于 Gallery 分组和 Policy 匹配 | `procurement` |
| `version` | string | 初始版本号，遵循语义化版本 | `1.0.0` |
| `config.owner` | string | 业务域负责人/团队标识 | `procurement-team` |
| `config.lifecycle` | string | 生命周期阶段：`draft` → `active` → `archived` | `draft` |

**命名规范**：
- `code` 必须全局唯一，建议格式：`[业务域]_[功能描述]`（如 `procurement_quote_review`）
- `domain` 建议与 `code` 的顶级段一致，便于 Policy 匹配
- 避免使用缩写和易混淆的词汇（如 `prc`、`pr`）

### 1.2 API 调用

```bash
# 认证获取 Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password"}' | jq -r '.data.token')

# 创建 Business App
curl -X POST http://localhost:8080/api/v1/business-apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "procurement",
    "name": "采购报价审查",
    "description": "标准化并比较供应商报价，整理审查证据，并强制进入人工审核",
    "domain": "procurement",
    "config": {
      "owner": "procurement-team",
      "lifecycle": "draft"
    }
  }'
```

**预期响应**：

```json
{
  "code": 201,
  "data": {
    "id": "ba_01HZ3K7XR9M2PQRSTUVWXYZ",
    "code": "procurement",
    "name": "采购报价审查",
    "description": "标准化并比较供应商报价，整理审查证据，并强制进入人工审核",
    "domain": "procurement",
    "status": "draft",
    "config": {
      "owner": "procurement-team",
      "lifecycle": "draft"
    },
    "created_at": "2026-08-19T10:30:00Z",
    "updated_at": "2026-08-19T10:30:00Z"
  }
}
```

### 1.3 验收标准

| # | 验收项 | 检查方法 | 通过 |
|---|--------|---------|------|
| 1 | API 返回 201 Created | 查看 HTTP 响应码 | ☐ |
| 2 | Business App 出现在列表中 | `GET /api/v1/business-apps` 确认 | ☐ |
| 3 | 状态为 `draft` | 检查 `status` 字段 | ☐ |
| 4 | 审计日志记录了创建操作 | `GET /api/v1/audit-logs?resource_type=business_app` | ☐ |
| 5 | 其他租户不可见 | 使用其他租户 Token 调用列表 API | ☐ |

### 1.4 常见失败点

| HTTP 状态 | 错误信息 | 原因 | 解决方案 |
|-----------|---------|------|---------|
| 409 Conflict | `code already exists` | `code` 已被其他业务域占用 | 使用不同的 `code`，或调用 `GET /api/v1/business-apps?code=xxx` 先确认 |
| 403 Forbidden | `permission denied: business_app:manage` | 缺少 `business_app:manage` 权限 | 联系管理员在 IAM 中授予权限 |
| 422 Unprocessable | `invalid domain format` | `domain` 字段不符合规范 | 确保为小写+下划线格式 |
| 500 Internal Error | `duplicate key violation` | 底层存储唯一索引冲突 | 检查 `code` 是否已被软删除的记录占用，可使用 `force=true` 参数恢复 |

---

## Step 2: 绑定 Domain Policy

### 2.1 准备元数据

Domain Policy 定义业务域间的访问边界，是平台多租户隔离和最小权限原则的核心机制。

**策略模型**：

```json
{
  "code": "procurement",
  "rules": [
    {
      "source_domain": "procurement",
      "target_domain": "shared",
      "allowed_resources": ["agent", "tool", "skill", "knowledge"],
      "permission_level": "read"
    },
    {
      "source_domain": "procurement",
      "target_domain": "procurement",
      "allowed_resources": ["agent", "tool", "workflow"],
      "permission_level": "write"
    }
  ]
}
```

**关键字段说明**：

| 字段 | 说明 | 可选值 |
|------|------|--------|
| `source_domain` | 发起访问的业务域 | 业务域 code，或 `*`（全局） |
| `target_domain` | 被访问的业务域 | 业务域 code，或 `*`（全局） |
| `allowed_resources` | 允许访问的资源类型 | `agent`, `tool`, `skill`, `workflow`, `knowledge`, `data` |
| `permission_level` | 权限级别 | `read`, `write`, `admin` |

**策略设计原则**：
1. **默认拒绝**：未显式声明的跨域访问一律拒绝
2. **最小权限**：先授予 `read`，确有需要再升级为 `write`
3. **单向授权**：`procurement → shared` 不代表 `shared → procurement`，如需双向需分别配置
4. **资源粒度**：`tool` 权限意味着可访问目标域的所有 Tool，如需细粒度控制应使用 Tool 级 Policy

### 2.2 API 调用

```bash
curl -X POST http://localhost:8080/api/v1/domain-policies \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "business_app_code": "procurement",
    "policy_code": "procurement",
    "rules": [
      {
        "source_domain": "procurement",
        "target_domain": "shared",
        "allowed_resources": ["agent", "tool", "skill", "knowledge"],
        "permission_level": "read"
      },
      {
        "source_domain": "procurement",
        "target_domain": "procurement",
        "allowed_resources": ["agent", "tool", "workflow"],
        "permission_level": "write"
      }
    ]
  }'
```

**预期响应**：

```json
{
  "code": 201,
  "data": {
    "id": "dp_01HZ3K8YR7N4QWERTYUIOP",
    "policy_code": "procurement",
    "business_app_code": "procurement",
    "rules": [...],
    "status": "active",
    "created_at": "2026-08-19T10:32:00Z"
  }
}
```

### 2.3 验收标准

| # | 验收项 | 预期结果 | 通过 |
|---|--------|---------|------|
| 1 | `procurement` 域的 Agent 访问 `shared` 域的 Tool（只读） | 允许访问 | ☐ |
| 2 | `procurement` 域的 Agent 访问 `finance` 域的任何资源 | 拒绝访问 | ☐ |
| 3 | `shared` 域的 Agent 访问 `procurement` 域的 Tool | 拒绝访问（反向隔离） | ☐ |
| 4 | `procurement` 域的 Agent 在本域内创建 Tool | 允许（write 权限） | ☐ |
| 5 | `procurement` 域的 Agent 修改 `shared` 域的资源 | 拒绝（只有 read 权限） | ☐ |

### 2.4 测试 Fixture

```python
import pytest
from httpx import AsyncClient

@pytest.mark.asyncio
async def test_domain_policy_isolation(client: AsyncClient, token: str):
    headers = {"Authorization": f"Bearer {token}"}

    # 1. procurement agent 尝试调用 finance 的 tool → 应被拒绝
    resp = await client.post("/api/v1/tools/execute",
        headers=headers,
        json={"tool_code": "finance.calculate_salary", "params": {}}
    )
    assert resp.status_code == 403
    assert resp.json()["code"] == "CROSS_DOMAIN_DENIED"

    # 2. procurement agent 尝试调用 shared 的 read_authorized_file → 应被允许
    resp = await client.post("/api/v1/tools/execute",
        headers=headers,
        json={"tool_code": "shared.read_authorized_file", "params": {"path": "/test"}}
    )
    assert resp.status_code == 200

    # 3. finance agent 尝试调用 procurement 的 query_supplier_master → 应被拒绝
    finance_token = (await client.post("/api/v1/auth/login",
        json={"username": "finance-admin", "password": "password"}
    )).json()["data"]["token"]
    resp = await client.post("/api/v1/tools/execute",
        headers={"Authorization": f"Bearer {finance_token}"},
        json={"tool_code": "procurement.query_supplier_master", "params": {}}
    )
    assert resp.status_code == 403

    # 4. procurement agent 尝试修改 shared 域的资源 → 应被拒绝
    resp = await client.put("/api/v1/shared/tools/read_authorized_file",
        headers=headers,
        json={"name": "modified"}
    )
    assert resp.status_code == 403
```

### 2.5 常见失败点

| HTTP 状态 | 错误信息 | 原因 | 解决方案 |
|-----------|---------|------|---------|
| 409 Conflict | `policy_code already exists` | 同一业务域已存在该 policy | 调用 `GET /api/v1/domain-policies?business_app_code=xxx` 查看已有策略，使用 PATCH 更新 |
| 422 Unprocessable | `invalid rule: source_domain and target_domain must differ for cross-domain access` | 跨域规则的源和目标相同 | 同域访问不需要配置 Policy，默认允许；移除 source==target 的规则 |
| 422 Unprocessable | `invalid permission_level: write requires explicit justification` | 直接申请写权限 | 在 Policy 的 `metadata.justification` 字段中说明业务必要性 |

---

## Step 3: 注册 Connector/Sidecar

### 3.1 何时需要 Connector/Sidecar

| 场景 | 是否需要 | 示例 |
|------|---------|------|
| 从外部系统拉取数据 | ✅ 需要 | 从 ERP 查询供应商主数据、从 OA 获取审批流程 |
| 向外部系统发送数据 | ✅ 需要 | 推送审批结果到 OA、归档合同到文档管理系统 |
| 与第三方服务集成 | ✅ 需要 | OCR 识别、汇率查询、物流追踪 |
| 纯 Agent 内部推理 | ❌ 不需要 | 单轮对话、知识问答 |
| 调用已注册的 Shared Tool | ❌ 不需要 | 使用平台已有的 `parse_excel`、`extract_pdf_text` 等 |

**决策流程**：

```
业务域是否需要与外部系统交互？
├── 是 → 是否可以通过平台已有 Connector 协议实现？
│   ├── 是 → 注册 Sidecar，声明 capability
│   └── 否 → 先注册新的 Connector 类型，再注册 Sidecar
└── 否 → 跳过 Step 3，直接进入 Step 4
```

### 3.2 Sidecar 注册

```bash
curl -X POST http://localhost:8080/api/v1/sidecars \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "procurement-erp-sidecar",
    "base_url": "http://procurement-erp:9000",
    "capabilities": [
      "query_supplier_master",
      "query_procurement_policy",
      "query_fx_snapshot"
    ],
    "business_app_code": "procurement",
    "health_check_path": "/health",
    "manifest_path": "/manifest",
    "auth": {
      "type": "api_key",
      "header": "X-API-Key",
      "secret": "${ERP_API_KEY}"
    },
    "timeout_ms": 5000,
    "retry_policy": {
      "max_retries": 3,
      "backoff_ms": 1000
    }
  }'
```

**注册参数说明**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | Sidecar 唯一名称，建议格式 `[业务域]-[系统名]-sidecar` |
| `base_url` | string | Sidecar 服务的基础 URL |
| `capabilities` | string[] | Sidecar 提供的能力列表，每个 capability 对应一个可被 Agent 调用的 Tool |
| `business_app_code` | string | 关联的 Business App，用于 Domain Policy 匹配 |
| `health_check_path` | string | 健康检查端点路径，平台定时调用 |
| `manifest_path` | string | 元数据端点路径，用于自动发现 capability |
| `auth` | object | 认证配置，支持 `none`/`api_key`/`oauth2`/`mtls` |
| `timeout_ms` | int | 单次请求超时时间（毫秒） |
| `retry_policy` | object | 重试策略 |

### 3.3 实现 Sidecar 协议

以下是一个完整的 Python Sidecar 实现示例（`agent-service/procurement_sidecar.py`），可直接复制使用：

```python
from fastapi import FastAPI, HTTPException, Header, Request
from pydantic import BaseModel, Field
from typing import Any, Optional
from datetime import datetime
import logging

logger = logging.getLogger("procurement-erp-sidecar")

app = FastAPI(
    title="Procurement ERP Sidecar",
    version="1.0.0",
    description="采购 ERP 系统连接器，提供供应商主数据、采购政策、汇率快照查询能力"
)

class ExecuteRequest(BaseModel):
    capability: str = Field(..., description="请求的能力名称")
    params: dict[str, Any] = Field(default_factory=dict, description="能力调用参数")
    tenant_id: str = Field(..., description="租户 ID，用于数据隔离")
    trace_id: str = Field(..., description="链路追踪 ID")
    metadata: Optional[dict[str, Any]] = Field(default=None, description="附加元数据")

class ExecuteResponse(BaseModel):
    success: bool
    data: Any | None = None
    error: str | None = None
    trace_id: str
    latency_ms: int = 0

class HealthResponse(BaseModel):
    status: str
    version: str
    uptime_seconds: int
    last_check: Optional[str] = None

class ManifestResponse(BaseModel):
    version: str
    capabilities: list[dict[str, Any]]
    business_app: str
    server_time: str

@app.post("/execute", response_model=ExecuteResponse)
async def execute(req: ExecuteRequest, request: Request):
    start_time = datetime.utcnow()

    auth_token = request.headers.get("X-API-Key", "")
    if not auth_token:
        raise HTTPException(status_code=401, detail="Missing API key")

    try:
        handler = CAPABILITY_MAP.get(req.capability)
        if handler is None:
            return ExecuteResponse(
                success=False,
                data=None,
                error=f"Unknown capability: {req.capability}",
                trace_id=req.trace_id,
                latency_ms=0
            )

        result = await handler(req)
        elapsed = int((datetime.utcnow() - start_time).total_seconds() * 1000)

        return ExecuteResponse(
            success=True,
            data=result,
            error=None,
            trace_id=req.trace_id,
            latency_ms=elapsed
        )

    except Exception as e:
        elapsed = int((datetime.utcnow() - start_time).total_seconds() * 1000)
        logger.error(f"Capability execution failed: {req.capability}, error: {e}")
        return ExecuteResponse(
            success=False,
            data=None,
            error=str(e),
            trace_id=req.trace_id,
            latency_ms=elapsed
        )

@app.get("/health", response_model=HealthResponse)
async def health():
    return HealthResponse(
        status="ok",
        version="1.0.0",
        uptime_seconds=0,
        last_check=datetime.utcnow().isoformat()
    )

@app.get("/manifest", response_model=ManifestResponse)
async def manifest():
    return ManifestResponse(
        version="1.0.0",
        capabilities=[
            {
                "name": "query_supplier_master",
                "description": "查询供应商主数据，包括名称、资质、评级、历史交付记录",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "supplier_id": {"type": "string", "description": "供应商 ID"},
                        "keyword": {"type": "string", "description": "搜索关键词"},
                        "page_size": {"type": "integer", "default": 20}
                    }
                },
                "output_schema": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "supplier_id": {"type": "string"},
                            "name": {"type": "string"},
                            "rating": {"type": "string"},
                            "delivery_score": {"type": "number"}
                        }
                    }
                }
            },
            {
                "name": "query_procurement_policy",
                "description": "查询采购政策，包括审批流程、预算限制、合规要求",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "category": {"type": "string", "description": "采购品类"},
                        "amount": {"type": "number", "description": "采购金额"}
                    }
                }
            },
            {
                "name": "query_fx_snapshot",
                "description": "查询实时汇率快照，支持多币种",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "from_currency": {"type": "string", "description": "源币种代码"},
                        "to_currency": {"type": "string", "description": "目标币种代码"}
                    }
                }
            }
        ],
        business_app="procurement",
        server_time=datetime.utcnow().isoformat()
    )

async def query_supplier_master(req: ExecuteRequest) -> dict:
    keyword = req.params.get("keyword", "")
    suppliers = [
        {
            "supplier_id": "SUP_001",
            "name": "华东电子科技有限公司",
            "rating": "A",
            "delivery_score": 94.5,
            "financial_health": "stable",
            "compliance_status": "approved",
            "categories": ["electronic_components", "raw_materials"]
        },
        {
            "supplier_id": "SUP_002",
            "name": "南方精密制造股份",
            "rating": "A",
            "delivery_score": 91.2,
            "financial_health": "stable",
            "compliance_status": "approved",
            "categories": ["precision_manufacturing"]
        },
        {
            "supplier_id": "SUP_003",
            "name": "环球物流服务集团",
            "rating": "B",
            "delivery_score": 82.0,
            "financial_health": "watch",
            "compliance_status": "approved",
            "categories": ["logistics", "warehousing"]
        }
    ]
    if keyword:
        suppliers = [s for s in suppliers if keyword in s["name"]]
    return {"suppliers": suppliers, "total": len(suppliers)}

async def query_procurement_policy(req: ExecuteRequest) -> dict:
    category = req.params.get("category", "general")
    amount = req.params.get("amount", 0)
    policies = {
        "approval_level": "L2",
        "budget_limit": 1000000,
        "required_documents": ["quotation", "supplier_certification", "budget_approval"],
        "compliance_checks": ["anti_bribery", "sanctions_screening"],
        "category": category,
        "amount_threshold": amount
    }
    if amount > 500000:
        policies["approval_level"] = "L3"
        policies["required_documents"].append("board_approval")
    return policies

async def query_fx_snapshot(req: ExecuteRequest) -> dict:
    from_cur = req.params.get("from_currency", "CNY")
    to_cur = req.params.get("to_currency", "USD")
    rates = {
        "CNY_USD": 0.1385,
        "CNY_EUR": 0.1265,
        "CNY_JPY": 21.75,
        "USD_CNY": 7.2210,
        "EUR_CNY": 7.9050,
        "JPY_CNY": 0.0460
    }
    key = f"{from_cur}_{to_cur}"
    return {
        "from": from_cur,
        "to": to_cur,
        "rate": rates.get(key, 1.0),
        "timestamp": datetime.utcnow().isoformat(),
        "source": "snapshot"
    }

CAPABILITY_MAP = {
    "query_supplier_master": query_supplier_master,
    "query_procurement_policy": query_procurement_policy,
    "query_fx_snapshot": query_fx_snapshot,
}
```

### 3.4 本地测试 Sidecar

```bash
# 安装依赖
pip install fastapi uvicorn pydantic httpx

# 启动 Sidecar
uvicorn procurement_sidecar:app --host 0.0.0.0 --port 9000

# 测试健康检查
curl http://localhost:9000/health

# 测试 manifest
curl http://localhost:9000/manifest

# 测试能力调用
curl -X POST http://localhost:9000/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "capability": "query_supplier_master",
    "params": {"keyword": "电子"},
    "tenant_id": "tenant_001",
    "trace_id": "trace_001"
  }'

# 测试未知 capability → 应返回错误
curl -X POST http://localhost:9000/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "capability": "unknown_capability",
    "params": {},
    "tenant_id": "tenant_001",
    "trace_id": "trace_002"
  }'
```

### 3.5 验收标准

| # | 验收项 | 预期结果 | 通过 |
|---|--------|---------|------|
| 1 | Sidecar `/health` 返回 200 | `{"status": "ok", ...}` | ☐ |
| 2 | Sidecar `/manifest` 返回正确的能力列表 | 包含 3 个 capability | ☐ |
| 3 | 调用已注册的 capability 成功 | 返回正确的数据结构 | ☐ |
| 4 | 调用未注册的 capability 返回错误 | 返回 `success: false` + 错误信息 | ☐ |
| 5 | 平台定时健康检查通过 | 查看平台日志确认 health check 被触发 | ☐ |
| 6 | 平台能读取 manifest 并完成自动注册 | 调用 `GET /api/v1/sidecars/{name}` 确认 | ☐ |
| 7 | 认证机制生效 | 无 API Key 的请求返回 401 | ☐ |

---

## Step 4: 发布 Skill

### 4.1 Skill 类型

| 类型 | 说明 | 生命周期 | 示例 |
|------|------|---------|------|
| **Shared Skill** | 跨业务域通用，由平台团队维护 | 随平台版本更新 | 文件解析（parse_excel）、OCR（ocr_extract）、实体解析（entity_resolution） |
| **Domain Skill** | 业务域专用，由业务团队维护 | 随业务域迭代更新 | 采购 TCO 计算（calculate_quote_tco）、HR 薪资计算（calc_salary_tax） |

**选择策略**：
- 通用能力 → 申请注册为 Shared Skill，供所有业务域使用
- 业务专用能力 → 创建 Domain Skill，归属特定业务域
- 不确定时 → 先创建为 Domain Skill，后续可升级为 Shared Skill

### 4.2 从 Marketplace 安装 Shared Skill

```bash
# 查看可用 Skills 列表
curl http://localhost:8080/api/v1/skill-marketplace \
  -H "Authorization: Bearer $TOKEN"

# 按分类筛选
curl "http://localhost:8080/api/v1/skill-marketplace?category=document" \
  -H "Authorization: Bearer $TOKEN"

# 安装 Skill 到业务域
curl -X POST http://localhost:8080/api/v1/skill-marketplace/install \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_code": "parse_excel",
    "business_app_code": "procurement"
  }'

# 批量安装
for skill in parse_excel parse_csv extract_pdf_text validate_structured_record; do
  curl -X POST http://localhost:8080/api/v1/skill-marketplace/install \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"skill_code\": \"$skill\", \"business_app_code\": \"procurement\"}"
done
```

### 4.3 创建 Domain Skill

如果 Marketplace 没有所需 Skill，可以创建自定义 Domain Skill：

```bash
curl -X POST http://localhost:8080/api/v1/skills \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "calculate_quote_tco",
    "name": "报价 TCO 计算",
    "description": "计算供应商报价的总拥有成本（Total Cost of Ownership），包含采购成本、物流成本、安装成本、维护成本",
    "category": "procurement",
    "domain": "procurement",
    "entry_point": "agent-service/app/skills/calculate_quote_tco.py",
    "input_schema": {
      "type": "object",
      "properties": {
        "base_price": {"type": "number", "description": "基础报价（单位：元）"},
        "quantity": {"type": "number", "description": "采购数量"},
        "logistics_cost": {"type": "number", "description": "物流成本（百分比）"},
        "installation_cost": {"type": "number", "description": "安装成本（百分比）"},
        "maintenance_cost_yearly": {"type": "number", "description": "年度维护成本（百分比）"},
        "lifetime_years": {"type": "integer", "description": "产品生命周期（年）"},
        "discount_rate": {"type": "number", "description": "折现率", "default": 0.05}
      },
      "required": ["base_price", "quantity"]
    },
    "output_schema": {
      "type": "object",
      "properties": {
        "tco": {"type": "number", "description": "总拥有成本"},
        "breakdown": {
          "type": "object",
          "properties": {
            "procurement": {"type": "number"},
            "logistics": {"type": "number"},
            "installation": {"type": "number"},
            "maintenance_pv": {"type": "number", "description": "维护成本现值"}
          }
        }
      }
    }
  }'
```

**Skill 实现代码示例**（`agent-service/app/skills/calculate_quote_tco.py`）：

```python
from pydantic import BaseModel
from typing import Optional
import math

class CalculateTCOInput(BaseModel):
    base_price: float
    quantity: float
    logistics_cost: float = 0.05
    installation_cost: float = 0.10
    maintenance_cost_yearly: float = 0.02
    lifetime_years: int = 5
    discount_rate: float = 0.05

class CalculateTCOOutput(BaseModel):
    tco: float
    breakdown: dict[str, float]

def execute(input: CalculateTCOInput) -> CalculateTCOOutput:
    procurement_cost = input.base_price * input.quantity
    logistics_cost = procurement_cost * input.logistics_cost
    installation_cost = procurement_cost * input.installation_cost

    maintenance_pv = 0.0
    for year in range(1, input.lifetime_years + 1):
        maintenance_year = procurement_cost * input.maintenance_cost_yearly
        present_value = maintenance_year / ((1 + input.discount_rate) ** year)
        maintenance_pv += present_value

    tco = procurement_cost + logistics_cost + installation_cost + maintenance_pv

    return CalculateTCOOutput(
        tco=round(tco, 2),
        breakdown={
            "procurement": round(procurement_cost, 2),
            "logistics": round(logistics_cost, 2),
            "installation": round(installation_cost, 2),
            "maintenance_pv": round(maintenance_pv, 2)
        }
    )
```

### 4.4 验收标准

| # | 验收项 | 预期结果 | 通过 |
|---|--------|---------|------|
| 1 | 安装的 Shared Skill 出现在 `installed` 列表中 | `GET /api/v1/skills?business_app_code=procurement` | ☐ |
| 2 | Domain Skill 创建成功 | 返回 201，`code` 唯一 | ☐ |
| 3 | Skill 的 input/output schema 通过校验 | 无效 schema 会返回 422 | ☐ |
| 4 | Skill 在 Agent 调用时能被正确发现和加载 | Agent 日志中可见 Skill 加载记录 | ☐ |
| 5 | Skill 能被 Agent 成功调用 | 端到端测试中 Skill 返回正确结果 | ☐ |

---

## Step 5: 打包 Agent Package

### 5.1 Agent Package 目录结构

```
procurement_quote_review/
├── package.json           # 包元数据（必填）
├── graph.py               # LangGraph 实现（必填）
├── agent.py               # Agent 入口（必填）
├── skills/                # 引用的 Domain Skill 实现
│   └── calculate_quote_tco.py
├── config/                # 配置文件
│   └── scoring_profile.yaml
├── knowledge/             # 领域知识库（可选）
│   └── procurement_policies.md
└── tests/                 # 测试用例
    ├── test_quote_review.py
    └── conftest.py
```

### 5.2 package.json

package.json 是 Agent Package 的核心描述文件，平台通过它理解 Agent 的能力、依赖和运行时需求：

```json
{
  "name": "procurement-quote-review",
  "version": "1.0.0",
  "description": "采购报价审查 Agent Package：标准化供应商报价，计算 TCO，生成审查报告",
  "author": "procurement-team",
  "license": "proprietary",
  "business_app": "procurement",
  "graph_key": "procurement_quote_review_graph",
  "entry_point": "agent.py:ProcurementQuoteReviewAgent",
  "capabilities": [
    "document_extraction",
    "entity_resolution",
    "policy_retrieval",
    "supplier_comparison",
    "tco_calculation",
    "risk_assessment",
    "report_generation"
  ],
  "required_skills": [
    "parse_excel",
    "parse_csv",
    "extract_pdf_text",
    "validate_structured_record",
    "calculate_quote_tco"
  ],
  "required_tools": [
    "shared.read_authorized_file",
    "procurement.query_supplier_master",
    "procurement.query_procurement_policy",
    "procurement.query_fx_snapshot"
  ],
  "permissions": [
    "agent:read",
    "workflow:read",
    "knowledge:read"
  ],
  "runtime": {
    "language": "python",
    "version": "3.11",
    "dependencies": [
      {"name": "langgraph", "version": ">=0.0.20"},
      {"name": "pydantic", "version": ">=2.0"},
      {"name": "httpx", "version": ">=0.25"}
    ]
  },
  "ui": {
    "icon": "📋",
    "color": "#2563eb",
    "category": "procurement"
  }
}
```

### 5.3 Agent 入口实现（agent.py）

```python
from langgraph.graph import StateGraph, END
from typing import TypedDict, Annotated
import operator

class QuoteReviewState(TypedDict):
    messages: Annotated[list, operator.add]
    quotes: list[dict]
    normalized_quotes: list[dict]
    supplier_info: list[dict]
    tco_results: list[dict]
    comparison_report: dict
    risk_flags: list[str]
    review_complete: bool

def extract_documents(state: QuoteReviewState) -> dict:
    """从对话中提取报价文档"""
    return {
        "messages": state["messages"],
        "quotes": [
            {"supplier": "华东电子", "unit_price": 12.50, "currency": "CNY"},
            {"supplier": "南方精密", "unit_price": 13.80, "currency": "CNY"},
            {"supplier": "环球物流", "unit_price": 11.20, "currency": "USD"}
        ]
    }

def normalize_quotes(state: QuoteReviewState) -> dict:
    """标准化报价（统一币种、格式）"""
    fx_rates = {"CNY": 1.0, "USD": 7.22, "EUR": 7.90}
    normalized = []
    for q in state["quotes"]:
        rate = fx_rates.get(q["currency"], 1.0)
        normalized.append({
            **q,
            "base_cny_price": round(q["unit_price"] * rate, 2),
            "normalized_currency": "CNY"
        })
    return {"normalized_quotes": normalized}

def fetch_supplier_info(state: QuoteReviewState) -> dict:
    """查询供应商背景信息"""
    return {
        "supplier_info": [
            {"supplier": "华东电子", "rating": "A", "delivery_score": 94.5},
            {"supplier": "南方精密", "rating": "A", "delivery_score": 91.2},
            {"supplier": "环球物流", "rating": "B", "delivery_score": 82.0}
        ]
    }

def calculate_tco(state: QuoteReviewState) -> dict:
    """计算各供应商 TCO"""
    tco_results = []
    for q in state["normalized_quotes"]:
        base = q["base_cny_price"]
        tco = base * (1 + 0.05 + 0.10) + base * 0.02 * 4.3295
        tco_results.append({
            "supplier": q["supplier"],
            "unit_price_cny": base,
            "tco": round(tco, 2)
        })
    return {"tco_results": tco_results}

def assess_risks(state: QuoteReviewState) -> dict:
    """评估风险"""
    flags = []
    for info in state["supplier_info"]:
        if info["rating"] == "B":
            flags.append(f"供应商 {info['supplier']} 评级为 B，需关注财务健康状况")
        if info["delivery_score"] < 85:
            flags.append(f"供应商 {info['supplier']} 交付评分低于 85")
    return {"risk_flags": flags}

def generate_report(state: QuoteReviewState) -> dict:
    """生成对比报告"""
    sorted_results = sorted(state["tco_results"], key=lambda x: x["tco"])
    best = sorted_results[0]
    return {
        "comparison_report": {
            "ranking": sorted_results,
            "best_value": best["supplier"],
            "recommendation": f"推荐 {best['supplier']}，TCO 最低为 ¥{best['tco']}",
            "risk_flags": state["risk_flags"]
        },
        "review_complete": True
    }

def should_continue(state: QuoteReviewState) -> str:
    if state.get("review_complete"):
        return END
    if not state.get("messages"):
        return END
    return "extract"

graph = StateGraph(QuoteReviewState)
graph.add_node("extract", extract_documents)
graph.add_node("normalize", normalize_quotes)
graph.add_node("fetch_supplier", fetch_supplier_info)
graph.add_node("calculate_tco", calculate_tco)
graph.add_node("assess_risks", assess_risks)
graph.add_node("generate_report", generate_report)

graph.set_entry_point("extract")
graph.add_edge("extract", "normalize")
graph.add_edge("normalize", "fetch_supplier")
graph.add_edge("fetch_supplier", "calculate_tco")
graph.add_edge("calculate_tco", "assess_risks")
graph.add_edge("assess_risks", "generate_report")
graph.add_conditional_edges("generate_report", should_continue)

ProcurementQuoteReviewAgent = graph.compile()
```

### 5.4 注册 Package

```bash
# 上传 Package 到制品仓库
# （此处示例使用 HTTP 上传，实际环境可能使用对象存储或制品仓库）
curl -X PUT https://packages.internal/upload/procurement-quote-review-1.0.0.tar.gz \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@procurement_quote_review.tar.gz"

# 注册 Package
curl -X POST http://localhost:8080/api/v1/agent-package-registrations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "package_code": "procurement-quote-review",
    "version": "1.0.0",
    "manifest_url": "https://packages.internal/procurement-quote-review/1.0.0/manifest.json",
    "business_app_code": "procurement",
    "changelog": "初始版本：采购报价标准化与比较"
  }'
```

**预期响应**：

```json
{
  "code": 201,
  "data": {
    "registration_id": "pkg_reg_01HZ3K9ZS7A5BCDEFGHIJKLMN",
    "package_code": "procurement-quote-review",
    "version": "1.0.0",
    "status": "pending_verification",
    "created_at": "2026-08-19T10:35:00Z"
  }
}
```

### 5.5 验证 Package

```bash
# 触发自动验证
curl -X POST "http://localhost:8080/api/v1/agent-package-registrations/procurement-quote-review/verify" \
  -H "Authorization: Bearer $TOKEN"

# 查看验证结果
curl "http://localhost:8080/api/v1/agent-package-registrations/procurement-quote-review" \
  -H "Authorization: Bearer $TOKEN"
```

**验证规则**：
1. `package.json` 中的 `required_skills` 必须全部已在平台注册
2. `package.json` 中的 `required_tools` 必须全部已在平台注册
3. `package.json` 中的 `business_app` 必须为有效 Business App
4. 所有依赖的 Skill 必须已安装到该 Business App

### 5.6 验收标准

| # | 验收项 | 预期结果 | 通过 |
|---|--------|---------|------|
| 1 | Package 注册成功 | 状态为 `pending_verification` | ☐ |
| 2 | 验证通过 | 状态变为 `verified` | ☐ |
| 3 | Package 信息与 `package.json` 一致 | 对比关键字段 | ☐ |
| 4 | 依赖的 Skills 全部已注册 | 无缺失依赖 | ☐ |
| 5 | 依赖的 Tools 全部已注册 | 无缺失依赖 | ☐ |
| 6 | Agent 入口能被正确加载 | 导入 `agent.py` 不报错 | ☐ |

---

## Step 6: 上架 Agent Gallery

### 6.1 创建版本

```bash
curl -X POST "http://localhost:8080/api/v1/agent-packages/procurement-quote-review/versions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.0.0",
    "release_notes": "初始版本：采购报价标准化与比较。支持多供应商报价 TCO 计算、风险评估和对比报告生成。",
    "is_stable": true,
    "supported_configs": {
      "max_suppliers": {"type": "integer", "default": 10, "description": "最大支持的供应商数量"},
      "default_currency": {"type": "string", "default": "CNY", "description": "默认标准化币种"},
      "enable_risk_assessment": {"type": "boolean", "default": true, "description": "是否启用风险评估"}
    }
  }'
```

### 6.2 发布版本

```bash
# 发布到 Gallery（用户可见但标记为 beta）
curl -X POST "http://localhost:8080/api/v1/agent-packages/procurement-quote-review/versions/1.0.0/publish" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "release_channel": "stable",
    "gallery_visible": true,
    "requires_approval": false
  }'
```

**发布渠道**：

| 渠道 | 说明 | 可见性 |
|------|------|--------|
| `dev` | 开发测试版 | 仅开发者可见 |
| `beta` | 公测版 | 标注"Beta"，所有用户可见 |
| `stable` | 稳定版 | 正式发布，无特殊标记 |
| `lts` | 长期支持版 | 正式发布，标记"LTS" |

### 6.3 安装到租户

```bash
# 安装到当前租户
curl -X POST http://localhost:8080/api/v1/agent-package-installations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "package_code": "procurement-quote-review",
    "version": "1.0.0",
    "auto_update": true,
    "config_overrides": {
      "max_suppliers": 20
    }
  }'

# 验证安装
curl "http://localhost:8080/api/v1/agent-package-installations?package_code=procurement-quote-review" \
  -H "Authorization: Bearer $TOKEN"
```

### 6.4 验收标准

| # | 验收项 | 预期结果 | 通过 |
|---|--------|---------|------|
| 1 | Agent 出现在 Gallery 列表中 | `GET /api/v1/agent-gallery` 可见 | ☐ |
| 2 | 分类正确 | `category` 显示为 `procurement` | ☐ |
| 3 | 版本信息正确 | 版本号和 Release Notes 正确显示 | ☐ |
| 4 | 用户可以启动对话 | `POST /api/v1/conversations` 创建成功 | ☐ |
| 5 | 配置覆盖生效 | `max_suppliers` 覆盖为 20 | ☐ |

---

## Step 7: 验收与归档

### 7.1 端到端验收清单

按以下清单逐项验收，确保每个环节都已通过：

| # | 验收项 | 操作 | 预期结果 | 通过 |
|---|--------|------|---------|------|
| 1 | 创建 Business App | `POST /api/v1/business-apps` | 返回 201，状态 `draft` | ☐ |
| 2 | 绑定 Domain Policy | `POST /api/v1/domain-policies` | 跨域访问被正确隔离 | ☐ |
| 3 | 注册 Sidecar | `POST /api/v1/sidecars` | health 检查通过，manifest 正确 | ☐ |
| 4 | 安装/创建 Skill | Skill Marketplace + 创建 Domain Skill | Skill 可被 Agent 发现 | ☐ |
| 5 | 打包 Agent Package | 注册 + 验证 | Package 注册+验证通过 | ☐ |
| 6 | 上架 Gallery | 创建版本 + 发布 | Agent 出现在列表中 | ☐ |
| 7 | 创建对话 | `POST /api/v1/conversations` | 用户可以开始对话 | ☐ |
| 8 | 端到端运行 | 发送消息到对话 | Agent 执行成功，输出结构化结果 | ☐ |
| 9 | 审计日志 | `GET /api/v1/audit-logs` | 所有操作可追溯 | ☐ |
| 10 | AVR 数据 | `GET /api/v1/avr/records` | 对话时长、Agent 产出工时可查 | ☐ |

### 7.2 归档文档

将以下文档归档到 `docs/06_PROCUREMENT/`（对应业务域文档目录）：

- [ ] Business App 元数据（API 请求/响应体）
- [ ] Domain Policy 配置（完整的 rules JSON）
- [ ] Sidecar 注册信息（注册请求体 + manifest）
- [ ] Agent Package manifest（package.json 全文）
- [ ] 测试报告（自动化测试 + 手动测试截图）
- [ ] 验收清单签署（本清单签字确认）
- [ ] 已知问题和后续优化计划

### 7.3 回滚方案

如果上线后出现问题，按反序执行回滚操作：

```bash
# 回滚 Step 6: 从 Gallery 下架
curl -X PATCH "http://localhost:8080/api/v1/agent-package-installations/{installation_id}" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "disabled"}'

curl -X POST "http://localhost:8080/api/v1/agent-packages/procurement-quote-review/versions/1.0.0/depublish" \
  -H "Authorization: Bearer $TOKEN"

# 回滚 Step 5: 取消 Package 注册
curl -X POST "http://localhost:8080/api/v1/agent-package-registrations/procurement-quote-review/reject" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason": "Rollback: critical issue"}'

# 回滚 Step 4: 卸载 Skill
curl -X POST "http://localhost:8080/api/v1/skill-marketplace/uninstall" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_code": "parse_excel", "business_app_code": "procurement"}'

# 回滚 Step 3: 移除 Sidecar
curl -X DELETE "http://localhost:8080/api/v1/sidecars/procurement-erp-sidecar" \
  -H "Authorization: Bearer $TOKEN"

# 回滚 Step 2: 移除 Domain Policy
curl -X DELETE "http://localhost:8080/api/v1/domain-policies/procurement" \
  -H "Authorization: Bearer $TOKEN"

# 回滚 Step 1: 归档 Business App（不可删除，归档后可恢复）
curl -X PATCH "http://localhost:8080/api/v1/business-apps/procurement" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"config": {"lifecycle": "archived"}}'
```

**回滚策略**：
- 回滚应按反序执行（Step 7 → Step 1），避免残留依赖
- 每回滚一步都需观察 10 分钟，确认无异常后再继续
- Business App 不应物理删除，而是归档为 `archived` 状态，保留审计追溯
- 如为紧急故障，可先禁用 installation（Step 6），待问题排查后再决定是否完全回滚

---

## 采购场景完整示例

### 前置条件

- ✅ 平台已部署并运行（`http://localhost:8080`）
- ✅ 管理员账号已创建（`admin` / `password`）
- ✅ 已获取业务需求文档（参考 `docs/06_PROCUREMENT/`）
- ✅ 采购 ERP Sidecar 已开发完成（参考 Step 3.3）
- ✅ 采购报价审查 Agent Package 已打包完成（参考 Step 5.2/5.3）

### 执行步骤（可直接复制执行）

```bash
# ============================================================
# Step 0: 认证
# ============================================================
echo "=== Step 0: 获取认证 Token ==="
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password"}' | jq -r '.data.token')

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "❌ 认证失败，请检查用户名和密码"
  exit 1
fi
echo "✅ 认证成功"

# ============================================================
# Step 1: 创建 Business App
# ============================================================
echo ""
echo "=== Step 1: 创建采购业务域 ==="
curl -X POST http://localhost:8080/api/v1/business-apps \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "procurement",
    "name": "采购报价审查",
    "description": "标准化并比较供应商报价，整理审查证据，并强制进入人工审核",
    "domain": "procurement",
    "config": {
      "owner": "procurement-team",
      "lifecycle": "draft"
    }
  }' | jq .

echo "✅ Step 1 完成"

# ============================================================
# Step 2: 绑定 Domain Policy
# ============================================================
echo ""
echo "=== Step 2: 绑定 Domain Policy ==="
curl -X POST http://localhost:8080/api/v1/domain-policies \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "business_app_code": "procurement",
    "policy_code": "procurement",
    "rules": [
      {
        "source_domain": "procurement",
        "target_domain": "shared",
        "allowed_resources": ["agent", "tool", "skill", "knowledge"],
        "permission_level": "read"
      },
      {
        "source_domain": "procurement",
        "target_domain": "procurement",
        "allowed_resources": ["agent", "tool", "workflow"],
        "permission_level": "write"
      }
    ]
  }' | jq .

echo "✅ Step 2 完成"

# ============================================================
# Step 3: 注册采购 ERP Sidecar
# ============================================================
echo ""
echo "=== Step 3: 注册采购 ERP Sidecar ==="
curl -X POST http://localhost:8080/api/v1/sidecars \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "procurement-erp-sidecar",
    "base_url": "http://procurement-erp:9000",
    "capabilities": [
      "query_supplier_master",
      "query_procurement_policy",
      "query_fx_snapshot"
    ],
    "business_app_code": "procurement",
    "health_check_path": "/health",
    "manifest_path": "/manifest"
  }' | jq .

echo "✅ Step 3 完成"

# ============================================================
# Step 4: 发布 Skill
# ============================================================
echo ""
echo "=== Step 4: 发布 Skills ==="

# 4a. 安装 Shared Skills
echo "--- 安装 Shared Skill: parse_excel ---"
curl -X POST http://localhost:8080/api/v1/skill-marketplace/install \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_code": "parse_excel", "business_app_code": "procurement"}' | jq .

echo "--- 安装 Shared Skill: parse_csv ---"
curl -X POST http://localhost:8080/api/v1/skill-marketplace/install \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_code": "parse_csv", "business_app_code": "procurement"}' | jq .

echo "--- 安装 Shared Skill: extract_pdf_text ---"
curl -X POST http://localhost:8080/api/v1/skill-marketplace/install \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_code": "extract_pdf_text", "business_app_code": "procurement"}' | jq .

# 4b. 创建 Domain Skill (calculate_quote_tco)
echo "--- 创建 Domain Skill: calculate_quote_tco ---"
curl -X POST http://localhost:8080/api/v1/skills \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "calculate_quote_tco",
    "name": "报价 TCO 计算",
    "description": "计算供应商报价的总拥有成本",
    "category": "procurement",
    "domain": "procurement",
    "entry_point": "agent-service/app/skills/calculate_quote_tco.py"
  }' | jq .

echo "✅ Step 4 完成"

# ============================================================
# Step 5: 注册 Agent Package
# ============================================================
echo ""
echo "=== Step 5: 注册 Agent Package ==="
curl -X POST http://localhost:8080/api/v1/agent-package-registrations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "package_code": "procurement-quote-review",
    "version": "1.0.0",
    "manifest_url": "https://packages.internal/procurement-quote-review/1.0.0/manifest.json",
    "business_app_code": "procurement"
  }' | jq .

# 触发验证
echo "--- 触发 Package 验证 ---"
sleep 2
curl -X POST "http://localhost:8080/api/v1/agent-package-registrations/procurement-quote-review/verify" \
  -H "Authorization: Bearer $TOKEN" | jq .

echo "✅ Step 5 完成"

# ============================================================
# Step 6: 上架并安装
# ============================================================
echo ""
echo "=== Step 6: 上架 Agent Gallery ==="

# 6a. 发布版本
curl -X POST "http://localhost:8080/api/v1/agent-packages/procurement-quote-review/versions/1.0.0/publish" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"release_channel": "stable"}' | jq .

# 6b. 安装到租户
curl -X POST http://localhost:8080/api/v1/agent-package-installations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_code": "procurement-quote-review", "version": "1.0.0"}' | jq .

echo "✅ Step 6 完成"

# ============================================================
# Step 7: 端到端验证
# ============================================================
echo ""
echo "=== Step 7: 端到端验证 ==="

# 7a. 查看 Gallery 确认 Agent 可见
echo "--- 7a: 查看 Agent Gallery ---"
curl "http://localhost:8080/api/v1/agent-gallery?category=procurement" \
  -H "Authorization: Bearer $TOKEN" | jq .

# 7b. 创建对话
echo "--- 7b: 创建对话 ---"
CONVERSATION=$(curl -s -X POST http://localhost:8080/api/v1/conversations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_package_code": "procurement-quote-review",
    "title": "测试采购报价审查"
  }')
echo "$CONVERSATION" | jq .

CONVERSATION_ID=$(echo "$CONVERSATION" | jq -r '.data.id')
echo "对话 ID: $CONVERSATION_ID"

# 7c. 发送消息
echo "--- 7c: 发送测试消息 ---"
curl -X POST "http://localhost:8080/api/v1/conversations/$CONVERSATION_ID/messages" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content": "请分析这份供应商报价对比表，计算 TCO 并给出推荐"}' | jq .

# 7d. 查看对话结果（等待 Agent 处理完成）
echo "--- 等待 Agent 处理 (10秒) ---"
sleep 10

echo "--- 7d: 查看对话消息 ---"
curl "http://localhost:8080/api/v1/conversations/$CONVERSATION_ID/messages" \
  -H "Authorization: Bearer $TOKEN" | jq .

# 7e. 查看审计日志
echo "--- 7e: 查看审计日志 ---"
curl "http://localhost:8080/api/v1/audit-logs?resource_type=agent_package&limit=20" \
  -H "Authorization: Bearer $TOKEN" | jq .

# 7f. 查看 AVR 数据
echo "--- 7f: 查看 AVR 工时数据 ---"
curl "http://localhost:8080/api/v1/avr/records?conversation_id=$CONVERSATION_ID" \
  -H "Authorization: Bearer $TOKEN" | jq .

echo ""
echo "============================================================"
echo "🎉 采购业务域接入完成！"
echo "============================================================"
```

---

## 扩展能力验收探针

每完成一个新业务域接入后，用以下问题自检平台扩展性。如果任何一个问题的答案偏离预期，说明平台扩展能力存在 bug，需要修复后才能接入下一个业务域。

| # | 探针问题 | 期望答案 | 检查方法 |
|---|---------|---------|---------|
| 1 | 接入过程中是否需要修改 `go-platform/internal/` 下的任何 Go 文件？ | ❌ 不应该 | `git diff --name-only HEAD` 不应包含此路径下的文件 |
| 2 | 接入过程中是否需要修改 `agent-service/app/core/` 下的任何 Python 文件？ | ❌ 不应该 | `git diff --name-only HEAD` 不应包含此路径下的文件 |
| 3 | 是否新增了任何业务域硬编码的 `if/else` 或 `switch` 分支？ | ❌ 不应该 | 在代码中搜索新业务域的 code 字符串 |
| 4 | Domain Policy 是否默认拒绝了所有跨域访问？ | ✅ 应该 | 尝试未授权的跨域访问应返回 403 |
| 5 | Tool 是否默认只有 read-only 权限？ | ✅ 应该 | 尝试写入操作应被阻止 |
| 6 | Shared Agent 是否能跨域获取非授权 Tool？ | ❌ 不应该 | Shared Agent 只能访问本域显式授权的 Tool |
| 7 | 审计日志是否完整记录了所有操作？ | ✅ 应该 | `GET /api/v1/audit-logs` 应有完整记录 |
| 8 | 回滚过程中是否存在"无法删除"的残留数据？ | ❌ 不应该 | 每一步回滚应无残留 |

**扩展能力评分**（每通过一项得 1 分）：
- 7-8 分：平台扩展能力优秀，可以接入下一个业务域
- 5-6 分：平台扩展能力良好，需要修复个别问题后继续
- 3-4 分：平台扩展能力存在明显缺陷，需系统性修复
- 0-2 分：平台扩展能力不足，需重构核心架构

---

## 附录

### A. 错误码速查

| HTTP 状态 | 系统错误码 | 含义 | 排查方向 |
|-----------|-----------|------|---------|
| 401 | `UNAUTHORIZED` | 未认证 | 检查 Token 是否有效、是否过期 |
| 403 | `FORBIDDEN` | 无权限 | 检查角色（Role）和权限（Permission）配置 |
| 403 | `CROSS_DOMAIN_DENIED` | 跨域访问被拒绝 | 检查 Domain Policy 是否正确配置 |
| 409 | `ALREADY_EXISTS` | 资源冲突 | 检查资源 code 是否已被占用 |
| 422 | `VALIDATION_ERROR` | 参数校验失败 | 检查请求体格式、必填字段、schema 合规性 |
| 500 | `INTERNAL_ERROR` | 服务器内部错误 | 查看后端日志，检查 Sidecar 健康状态 |
| 503 | `SERVICE_UNAVAILABLE` | 依赖服务不可用 | 检查 Sidecar/Connector 是否正常运行 |

### B. 相关 API 文档

| API | 方法 | 路径 | 用途 |
|-----|------|------|------|
| Business App | `POST` | `/api/v1/business-apps` | 创建业务域 |
| Business App | `GET` | `/api/v1/business-apps` | 列表查询 |
| Domain Policy | `POST` | `/api/v1/domain-policies` | 绑定跨域策略 |
| Domain Policy | `DELETE` | `/api/v1/domain-policies/{code}` | 移除策略 |
| Sidecar | `POST` | `/api/v1/sidecars` | 注册 Sidecar |
| Sidecar | `GET` | `/api/v1/sidecars/{name}` | 查询 Sidecar 状态 |
| Skill Marketplace | `GET` | `/api/v1/skill-marketplace` | 查看可用 Skill |
| Skill Marketplace | `POST` | `/api/v1/skill-marketplace/install` | 安装 Skill |
| Skill Marketplace | `POST` | `/api/v1/skill-marketplace/uninstall` | 卸载 Skill |
| Skill | `POST` | `/api/v1/skills` | 创建 Domain Skill |
| Agent Package | `POST` | `/api/v1/agent-package-registrations` | 注册 Package |
| Agent Package | `POST` | `/api/v1/agent-package-registrations/{code}/verify` | 验证 Package |
| Agent Package | `POST` | `/api/v1/agent-package-registrations/{code}/reject` | 拒绝/回滚 Package |
| Agent Gallery | `GET` | `/api/v1/agent-gallery` | 浏览 Gallery |
| Agent Gallery | `POST` | `/api/v1/agent-packages/{code}/versions/{version}/publish` | 发布版本 |
| Agent Gallery | `POST` | `/api/v1/agent-packages/{code}/versions/{version}/depublish` | 下架版本 |
| Installation | `POST` | `/api/v1/agent-package-installations` | 安装到租户 |
| Installation | `PATCH` | `/api/v1/agent-package-installations/{id}` | 启用/禁用 |
| Conversation | `POST` | `/api/v1/conversations` | 创建对话 |
| Conversation | `POST` | `/api/v1/conversations/{id}/messages` | 发送消息 |
| Audit Logs | `GET` | `/api/v1/audit-logs` | 查询审计日志 |
| AVR Records | `GET` | `/api/v1/avr/records` | 查询 Agent 工时数据 |

### C. 版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| v1.0 | 2026-08-19 | 初始版本，包含 7 步接入流程、采购场景完整示例、扩展能力验收探针 |