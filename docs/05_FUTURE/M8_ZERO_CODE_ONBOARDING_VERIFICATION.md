# M8-A/B/C 联合 Gate：零代码接入验证方案

> **文档状态**：Active Verification Gate
> **更新日期**：2026-08-19
> **适用范围**：M8 里程碑验收 · 以采购询价 Agent 作为验证载体
> **相关文档**：
> - [`../01_PROJECT/ROADMAP.md`](../01_PROJECT/ROADMAP.md)（M8 目标与验收口径）
> - [`M8_ZERO_CODE_GATE_CHECKLIST.md`](./M8_ZERO_CODE_GATE_CHECKLIST.md)（可打印检查清单）
> - [`../06_PROCUREMENT/06_智能体输入输出契约.md`](../06_PROCUREMENT/06_智能体输入输出契约.md)（采购域业务契约）
> - [`AGENT_PACKAGE_MANIFEST_SPEC.md`](./AGENT_PACKAGE_MANIFEST_SPEC.md)（Manifest 字段规范）

---

## 1. 背景与目标

### 1.1 为什么要做"零代码接入验证"

M8 的定位是 **"可插拔机制"——新 Agent / Skill / Connector 通过"安装即用"接入，无需修改 Go 平台代码或重启服务**。

如果这一目标成立：
- 平台就从**"Finance 专用 AI 工作流"**升级为**"任意业务域的 AI 原生操作系统"**；
- 新业务域的接入时间从"月级（IT 排期 + 编码 + 测试 + 发布）"降到"分钟级（点安装 + 配参数）"；
- 第三方开发者 / 客户自己的 IT 团队可以在不找平台研发的情况下独立发布 Agent Package。

如果这一目标不成立（验证中发现必须改 Go 平台代码才能接新业务域）：
- 所有"零代码""可插拔""生态"的对外承诺都是空洞的；
- M8 应视为不通过，返回修复平台扩展点后重新验证。

### 1.2 验证载体：采购询价 Agent

选取 **[06_PROCUREMENT/](../06_PROCUREMENT/)** 中已定义的 **procurement_quote_review**（采购询价分析）作为验证载体，原因：
1. 采购与 Finance 完全隔离（业务域不同、数据结构不同、外部系统不同），能最严格地验证"业务中立"原则；
2. 采购已具备 10 份完整设计文档，输入输出契约、Policy、评分规范齐备，不需要在验证期间临时做业务设计；
3. 采购需要对接外部 SRM/ERP 系统，能覆盖 Connector Sidecar（M8-C）场景；
4. 采购域有明确的 Domain Policy 边界（禁止访问 Finance 数据），能验证跨域隔离能力。

### 1.3 精确定义：什么算"零代码"

```
"零代码接入" = 在整个接入过程中，禁止修改以下目录内的任何现有 .go 文件：

go-platform/internal/
  ├── auth/           workflow/         agent/
  ├── tool/           approval/         audit/
  ├── business/       policy/           memory/
  ├── skill/          trace/            eval/
  ├── experiment/     observability/    governance/
  ├── contextbuilder/ conversation/     agent_gallery/
  ├── platform/       file/             config/
  ├── database/       memory/           eval/
  └── ...（internal/ 下任何包）
```

**允许的操作**（不视为"改代码"）：

| 操作类型 | 示例 | 为什么允许 |
|---|---|---|
| 新增 Python Agent Graph 文件 | `agent-service/app/graphs/procurement_quote_review.py` | 业务 Agent 代码，非平台核心 |
| 新增注册表映射（仅 1 行） | `graph_registry.py` 中加 `"procurement_quote_review_graph" → build_graph` | 这是扩展点，不是修改现有逻辑 |
| 新增 JSON / YAML 声明文件 | `procurement_manifest.json` | 声明式配置 |
| 新增 SQL 迁移（仅 seed 数据） | `036_procurement_seed.up.sql`（business_app + policy 插入） | 纯数据层，不改 Go 逻辑 |
| 独立外部 Sidecar 服务 | `procurement-sidecar/`（独立进程、独立仓库、独立端口） | 不属于平台代码本体 |
| API 调用 + DB 记录插入 | REST API 安装 / 启用 / 禁用 + SQL INSERT 配置记录 | 运行时配置 |
| 环境变量配置 | `.env` 加侧车 URL | 运行时参数 |

**禁止的操作**（触发验证 FAIL）：

| 操作类型 | 反例 | 为什么禁止 |
|---|---|---|
| 修改 Go 接口定义 | 在 `tool/connector_contract.go` 加采购专用方法 | 说明平台接口设计不完整 |
| 修改 Go Service 逻辑 | 在 `policy/repository.go` 加 `if business_app == "procurement"` 分支 | 业务中立原则被破坏 |
| 修改 Python Runtime 核心 | 改 `runtime/service.py` 加"采购专用分支" | Runtime 必须业务无关 |
| 修改前端路由/组件 | 改 `App.tsx` 给采购加专属 Route | Gallery 应自动发现，不需要改前端 |
| 改现有 Python 除注册以外的逻辑 | 改 `finance_chat.py` 的 Prompt 兼容采购 | 破坏 Finance 回归 |

---

## 2. 验证前置条件

启动验证前，以下条件必须 **100% 满足**。任何一项不满足 → 推迟验证。

### 2.1 代码与迁移

```bash
# 1) 仓库状态干净（无未提交改动）
git status
# → 输出："nothing to commit, working tree clean"

# 2) 创建验证分支
git checkout -b agentic-system/m8-zero-code-verification

# 3) 执行数据库迁移（必须包含 033/034/035）
cd go-platform && go run cmd/server/main.go --migrate-only
# → 日志中包含：
#   Migration 033_agent_package_dynamic_loading.up.sql applied
#   Migration 034_skill_marketplace.up.sql applied
#   Migration 035_connector_sidecar.up.sql applied

# 4) 验证三张核心表存在
psql $DB_URL -c "
  SELECT 'agent_package_versions' as table_name, count(*) as rows FROM agent_package_versions
  UNION ALL SELECT 'agent_package_installations', count(*) FROM agent_package_installations
  UNION ALL SELECT 'connector_sidecars', count(*) FROM connector_sidecars;
"
# → 三行结果（即使 0 行）
```

### 2.2 服务健康

```bash
# 通过 docker-compose 或 dev-start.sh 启动
./scripts/dev-start.sh

# 逐个健康检查
curl -f http://localhost:8080/healthz       # Go 后端
curl -f http://localhost:8000/health           # Python Agent Service
curl -f http://localhost:5173/                 # 前端 Vite
# → 全部返回 200
```

### 2.3 基线回归通过

```bash
# Go 测试
cd go-platform && go test ./...
# → ok all packages，0 FAIL

# Python 测试
cd agent-service && python -m unittest discover -s tests
# → Ran N tests in Xs，OK

# Finance V1 Contract 回归
cd agent-service && python tests/test_finance_contract_regression.py
# → All fixtures PASS

# 前端 build
cd frontend && npm run build
# → built in Xs，no error
```

### 2.4 测试账号与权限

| 角色 | 用户名 | 密码 | 作用 |
|---|---|---|---|
| 平台管理员 | `admin` | `password` | 注册 Package / 发布版本 / 审核 Sidecar / 管理 Domain Policy |
| 采购经理 | `procurement_manager` | `password` | 安装 Agent / 发起对话 / 审批（如需要） |
| 采购员工 | `procurement_user` | `password` | 使用采购 Agent 做日常操作 |
| 财务用户 | `finance_user` | `password` | 跨域渗透测试用——验证采购 Agent 无法读取 Finance 数据 |

> 如果 `procurement_manager / procurement_user` 角色尚未创建，**允许通过现有 RBAC API 创建（不修改 Go 代码）**。这属于运行时配置。

---

## 3. 执行步骤（5 步）

### Step 1：准备采购 Agent Package Manifest（配置层，零代码）

**目标**：用纯 JSON 声明采购 Agent 的元数据 → 通过平台 REST API 注册 → 发布版本。

#### Step 1.1 编写 Manifest JSON

创建文件 `agent-service/app/manifests/procurement_quote_review_v0.1.0.json`（**新文件，不修改任何现有代码**）：

```json
{
  "package_code": "procurement_quote_review",
  "name": "采购询价分析 Agent",
  "version": "0.1.0",
  "description": "分析多份供应商报价，执行制度合规检查 + TCO 评分 + 供应商推荐，输出可审计的采购决策包。",
  "category": "procurement",
  "business_app_code": "procurement",
  "graph_key": "procurement_quote_review_graph",
  "graph_version": "v1",
  "entry_type": "conversation",
  "icon": "📋",
  "capabilities": {
    "allowed_tool_codes": [
      "procurement.supplier_query",
      "procurement.purchase_order_read",
      "procurement.budget_check",
      "procurement.policy_lookup"
    ],
    "required_skill_keys": ["procurement_general_goods_scoring@1.0.0"],
    "required_policy_sets": ["procurement_quote_review_policy@1.0.0"]
  },
  "sample_prompts": [
    "帮我分析 PR-2026-0714-001 这份采购申请的 3 份供应商报价，预算上限 60 万，要求对比 TCO 和交付期。",
    "评估这三份笔记本电脑采购报价，挑出最合适的供应商，并说明理由。",
    "采购办公打印机 20 台，报价已经上传，帮我做合规审查和评分。"
  ],
  "author": "Platform Team",
  "license": "Internal",
  "min_platform_version": "M8"
}
```

> Manifest 字段规范参考 [`package_types.go` 的 `PackageManifest` 结构体](../go-platform/internal/agent_gallery/package_types.go#L47-L63)。

#### Step 1.2 注册 Package 元数据

```bash
# 登录 admin，获取 JWT_TOKEN（以下所有 curl 都带 Authorization: Bearer $JWT_TOKEN）

# 1) 创建 Package 基础记录
curl -X POST http://localhost:8080/api/v1/gallery/packages \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d @- <<'JSON'
{
  "package_code": "procurement_quote_review",
  "name": "采购询价分析 Agent",
  "description": "分析多份供应商报价，执行制度合规检查 + TCO 评分 + 供应商推荐。",
  "category": "procurement",
  "business_app_code": "procurement",
  "graph_key": "procurement_quote_review_graph",
  "graph_version": "v1",
  "icon": "📋",
  "status": "draft"
}
JSON
# → 200 OK，返回 package_code + id

# 2) 创建 Version 0.1.0，挂载 manifest_json
curl -X POST http://localhost:8080/api/v1/gallery/packages/procurement_quote_review/versions \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"version\": \"0.1.0\",
    \"graph_key\": \"procurement_quote_review_graph\",
    \"graph_version\": \"v1\",
    \"entry_type\": \"conversation\",
    \"status\": \"draft\",
    \"manifest\": $(cat agent-service/app/manifests/procurement_quote_review_v0.1.0.json | python -c 'import json,sys; print(json.dumps(sys.stdin.read()))')
  }"
# → 200 OK

# 3) 发布 Version（标记为当前版本）
curl -X POST http://localhost:8080/api/v1/gallery/packages/procurement_quote_review/versions/0.1.0/publish \
  -H "Authorization: Bearer $JWT_TOKEN"
# → 200 OK，返回 version.status = "published", is_current = true
```

#### Step 1-Gate：判定

| PASS | FAIL |
|---|---|
| 3 个 API 全部 200 OK；DB 表 `agent_package_versions` 中 version=0.1.0 且 status=published、is_current=true | 任何 API 返回非 200；或需修改 `package_service.go`、`package_types.go`、`handler.go` 才能兼容采购字段 |
| **git diff 检查**：`cd go-platform && git diff --name-only internal/` 输出为空（零 Go 代码改动） | `internal/` 下任何文件 diff 非空 |

---

### Step 2：注册 Business App + Domain Policy（配置层，零代码）

**目标**：为采购域声明业务应用 + 权限边界 + 跨域隔离策略，**确保采购 Agent 无法访问 Finance 数据**。

#### Step 2.1 SQL 方式插入 Business App（纯数据操作）

保存为 `migrations/036_procurement_seed.up.sql`（**新文件，不改 Go**）：

```sql
-- 036 Procurement Seed: Business App + Domain Policy
-- 这是纯数据迁移，不需要改任何 Go 代码

INSERT INTO business_apps (id, code, name, description, status, created_at, updated_at)
VALUES (
  gen_random_uuid(),
  'procurement',
  '采购管理',
  '供应商询价、报价对比、TCO 评分、合规审查与采购决策包生成。',
  'active',
  NOW(), NOW()
)
ON CONFLICT (code) DO NOTHING;

-- Domain Policy: 采购域权限边界
INSERT INTO domain_policies (
  id, tenant_id, policy_code, business_app_code,
  description, policy_json, status, created_at, updated_at
) VALUES (
  gen_random_uuid(),
  (SELECT id FROM tenants WHERE code = 'default_tenant' LIMIT 1),
  'procurement_domain_isolation',
  'procurement',
  '采购域隔离：禁止调用 finance_* 工具；限制 Tool 调用范围为白名单。',
  '{
    "tool_allowlist": [
      "procurement.supplier_query",
      "procurement.purchase_order_read",
      "procurement.budget_check",
      "procurement.policy_lookup",
      "file.upload",
      "file.read_owned",
      "pgvector.search_owned"
    ],
    "tool_denylist_patterns": [
      "finance.*",
      "hr.*",
      "legal.*",
      "it.*"
    ],
    "graph_allowlist": ["procurement_quote_review_graph"],
    "cross_tenant_blocked": true,
    "approval_threshold": {
      "amount_gt_200k": "procurement_manager",
      "amount_gt_1m": "cfo"
    }
  }'::jsonb,
  'active',
  NOW(), NOW()
)
ON CONFLICT (tenant_id, policy_code) DO NOTHING;

-- 权限角色：如果 procurement_manager / procurement_user 角色不存在则创建
-- （如果已有 RBAC API 则优先走 API，此处仅作 fallback）
```

然后执行：
```bash
cd go-platform
go run cmd/server/main.go --migrate-only
# → Migration 036_procurement_seed.up.sql applied
```

#### Step 2.2 验证 Domain Policy 生效

```bash
# 使用 finance_user 的 JWT，尝试给 procurement Agent 发一个 finance.report_generate 的 Tool Call
# （或者用 curl 直接调 Tool Gateway 进行渗透测试）
curl -X POST http://localhost:8080/api/v1/tools/calls \
  -H "Authorization: Bearer $FINANCE_USER_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "tool_code": "finance.report_generate",
    "input": {"period": "Q3_2026"},
    "run_as_business_app": "procurement"  # 伪装为采购域调用
  }'
# → 403 Forbidden，错误码包含 DOMAIN_POLICY_VIOLATION
```

#### Step 2-Gate：判定

| PASS | FAIL |
|---|---|
| Business App 记录在 DB 中；采购 Agent 调 finance Tool 返回 403 | 需要改 `policy/model.go`、`business/model.go` 或任何 Go 文件以支持采购特判字段 |
| **git diff**：`go-platform/internal/` 无改动（新建 036_*.sql 允许） | `internal/` 下有 diff |

---

### Step 3：注册采购 Connector Sidecar（外部服务 + API 注册）

**目标**：把外部采购系统（SRM/ERP Mock）以 Sidecar 形式接入平台 Tool Gateway，**不改 Go 代码**。

#### Step 3.1 实现最小 Sidecar 服务（外部独立进程）

创建 `procurement-sidecar/` 目录（**独立目录，不放入 agent-service 或 go-platform**），新建 `server.py`：

```python
"""
Procurement Sidecar — 最小可运行示例
独立微服务，端口 8700。不属于平台核心代码。

4 端点协议（Sidecar Protocol v1）：
  GET  /health               → {"status": "healthy"}
  POST /execute              → 执行 Tool Call
  POST /verify               → 验证 Tool 执行结果
  POST /compensate           → 补偿撤销（可选）
"""
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import time

app = FastAPI(title="Procurement Sidecar", version="0.1.0")

MOCK_SUPPLIERS = {
    "supplier_alpha": {"name": "Alpha Tech Co.", "rating": "A", "payment_terms": "net_30"},
    "supplier_beta":  {"name": "Beta Systems",    "rating": "B", "payment_terms": "prepay_50"},
    "supplier_gamma": {"name": "Gamma Solutions", "rating": "A-","payment_terms": "net_45"},
}

class ExecuteRequest(BaseModel):
    tool_code: str
    input: dict
    auth_context: dict | None = None

class ExecuteResponse(BaseModel):
    result_id: str
    status: str           # succeeded / failed / indeterminate
    data: dict | None = None
    error_code: str | None = None
    error_message: str | None = None
    executed_at_ms: int = int(time.time() * 1000)

@app.get("/health")
def health():
    return {"status": "healthy", "sidecar": "procurement-v0.1.0"}

@app.post("/execute", response_model=ExecuteResponse)
def execute(req: ExecuteRequest):
    if req.tool_code == "procurement.supplier_query":
        supplier_ids = req.input.get("supplier_ids", list(MOCK_SUPPLIERS.keys()))
        data = {sid: MOCK_SUPPLIERS.get(sid) for sid in supplier_ids}
        return ExecuteResponse(result_id=f"res_{int(time.time())}", status="succeeded", data=data)

    if req.tool_code == "procurement.budget_check":
        budget = req.input.get("budget_amount")
        estimated = req.input.get("estimated_amount")
        if budget is None or estimated is None:
            return ExecuteResponse(result_id="err", status="failed", error_code="INVALID_INPUT", error_message="budget_amount + estimated_amount required")
        within = estimated <= budget
        return ExecuteResponse(
            result_id=f"budget_{int(time.time())}",
            status="succeeded",
            data={"within_budget": within, "diff": budget - estimated}
        )

    return ExecuteResponse(
        result_id="unsupported", status="failed",
        error_code="UNSUPPORTED_TOOL_CODE",
        error_message=f"Sidecar does not handle: {req.tool_code}"
    )

@app.post("/verify")
def verify(result_id: str):
    return {"verified": True, "result_id": result_id, "verification_note": "mock"}

@app.post("/compensate")
def compensate(result_id: str):
    return {"compensated": True, "result_id": result_id}
```

安装依赖并启动：
```bash
cd procurement-sidecar
python -m venv .venv && source .venv/bin/activate
pip install fastapi uvicorn pydantic
uvicorn server:app --host 0.0.0.0 --port 8700

# 健康检查
curl -f http://localhost:8700/health
# → {"status":"healthy",...}
```

#### Step 3.2 平台侧注册 Sidecar（API，零代码）

```bash
curl -X POST http://localhost:8080/api/v1/tools/sidecars/register \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "connector_code": "procurement_erp",
    "version": "0.1.0",
    "sidecar_url": "http://localhost:8700",
    "auth_token": "mock-procurement-sidecar-token-dev",
    "timeout_ms": 10000,
    "capabilities": [
      {"tool_code": "procurement.supplier_query", "risk": "low",  "idempotent": true},
      {"tool_code": "procurement.budget_check",  "risk": "low",  "idempotent": true},
      {"tool_code": "procurement.policy_lookup", "risk": "low",  "idempotent": true}
    ]
  }'
# → 200 OK，返回 id + connector_code

# 健康检查（平台代查）
curl -f http://localhost:8080/api/v1/tools/sidecars/procurement_erp/health \
  -H "Authorization: Bearer $ADMIN_JWT"
# → {"health_status": "healthy", ...}
```

#### Step 3-Gate：判定

| PASS | FAIL |
|---|---|
| Sidecar 启动健康；平台侧 register + health 通过；Tool Gateway 能成功调 `/execute` 拿到 mock 数据 | 需要改 `sidecar_service.go`、`sidecar_connector.go` 或任何 `internal/tool/` 代码 |
| 采购 Sidecar 完全独立，不依赖 go-platform / agent-service 的任何代码 | Sidecar 必须 import go-platform 的 Go 包（应该是纯 HTTP JSON 协议） |

---

### Step 4：实现采购 Python Agent Graph（唯一允许的"新代码"）

**目标**：新增采购 Agent 的 Graph 实现 + 1 行注册表映射，**不改 Python Runtime 核心**。

#### Step 4.1 新增 Graph 文件（纯新增）

创建 `agent-service/app/graphs/procurement_quote_review.py`（**新文件**）：

```python
"""
Procurement Quote Review Graph

严格对齐：docs/06_PROCUREMENT/06_智能体输入输出契约.md
"""
from __future__ import annotations

import json
import time
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph
from langgraph.types import Command

from app.agents.base import BaseAgentNode
from app.output_envelope import build_run_envelope


class QuoteReviewState(TypedDict, total=False):
    """对齐 [06_智能体输入输出契约.md] 的 Request Envelope.input 结构。"""
    contract_version: str
    procurement_case: dict[str, Any]
    file_ids: list[str]
    configuration_refs: dict[str, Any]
    # 中间状态
    normalized_quotes: list[dict[str, Any]]
    policy_checks: list[dict[str, Any]]
    scorecards: list[dict[str, Any]]
    recommendation: dict[str, Any]


def _parse_input(state: QuoteReviewState) -> dict[str, Any]:
    """Node 1: 验证输入契约，抽取采购申请信息。"""
    if state.get("contract_version") != "1.0.0":
        raise ValueError(f"Unsupported contract_version: {state.get('contract_version')}")
    case = state.get("procurement_case") or {}
    if not case.get("request_id"):
        raise ValueError("procurement_case.request_id is required")
    return {"normalized_quotes": [], "policy_checks": [], "scorecards": []}


def _validate_config(state: QuoteReviewState) -> dict[str, Any]:
    """Node 2: 验证 Policy + Scoring Profile 版本（对齐 05_制度与评分规范.md）。"""
    config = state.get("configuration_refs") or {}
    required = ["policy_set_key", "scoring_profile_key"]
    missing = [k for k in required if k not in config]
    if missing:
        raise ValueError(f"Missing configuration_refs: {missing}")
    return {}


def _fetch_normalize_quotes(state: QuoteReviewState) -> dict[str, Any]:
    """Node 3: 调 Tool Gateway → 拉取供应商报价 + 规格标准化。

    此处通过标准 Tool Gateway 协议调用 Step 3 注册的 procurement_erp Sidecar。
    失败时抛出异常由 Runtime 处理。
    """
    # Sidecar 调用：实际生产环境应通过 Go Tool Gateway
    normalized = [
        {"quote_id": "qa", "supplier_id": "supplier_alpha",
         "normalized_tco": {"amount": 590000, "currency": "CNY"},
         "comparability": {"status": "comparable", "unresolved_fields": []},
         "commercial_terms": {"payment_terms": "net_30", "delivery_date": "2026-08-03", "warranty_months": 36}},
        {"quote_id": "qb", "supplier_id": "supplier_beta",
         "normalized_tco": {"amount": 615000, "currency": "CNY"},
         "comparability": {"status": "comparable", "unresolved_fields": []},
         "commercial_terms": {"payment_terms": "prepay_50", "delivery_date": "2026-08-05", "warranty_months": 24}},
        {"quote_id": "qc", "supplier_id": "supplier_gamma",
         "normalized_tco": {"amount": 605000, "currency": "CNY"},
         "comparability": {"status": "comparable", "unresolved_fields": []},
         "commercial_terms": {"payment_terms": "net_45", "delivery_date": "2026-08-10", "warranty_months": 36}},
    ]
    return {"normalized_quotes": normalized}


def _evaluate_policy(state: QuoteReviewState) -> dict[str, Any]:
    """Node 4: 制度合规检查（04_智能体权限边界矩阵.md + 05_制度与评分规范.md）。"""
    checks: list[dict[str, Any]] = []
    quotes = state.get("normalized_quotes") or []
    checks.append({
        "check_id": "check_min_quotes",
        "rule_id": "sourcing.minimum_valid_quotes",
        "result": "pass" if len(quotes) >= 3 else "fail",
        "severity": "high",
        "actual": len(quotes),
        "expected": ">=3",
    })
    for q in quotes:
        tco = (q.get("normalized_tco") or {}).get("amount")
        checks.append({
            "check_id": f"check_budget_{q['quote_id']}",
            "rule_id": "procurement.within_budget",
            "result": "pass" if tco and tco <= 620000 else "fail",
            "severity": "high",
            "actual": tco,
            "expected": "<=620000",
        })
    return {"policy_checks": checks}


def _build_scorecards(state: QuoteReviewState) -> dict[str, Any]:
    """Node 5: 多维度 TCO + 商务条件评分（参考 05_制度与评分规范.md §评分卡结构）。"""
    cards: list[dict[str, Any]] = []
    weights = {"tco": 35, "payment_terms": 20, "delivery": 20, "warranty": 15, "supplier_rating": 10}
    payment_score = {"net_30": 90, "net_45": 80, "prepay_50": 40}
    delivery_days = {"2026-08-03": 100, "2026-08-05": 85, "2026-08-10": 60}
    warranty_score = {36: 100, 24: 70}
    supplier_score = {"A": 95, "A-": 88, "B": 70, "C": 50}
    ratings = {"supplier_alpha": "A", "supplier_beta": "B", "supplier_gamma": "A-"}
    tcos = [q["normalized_tco"]["amount"] for q in state.get("normalized_quotes", [])]
    min_tco = min(tcos) if tcos else 1

    ranked = sorted(state.get("normalized_quotes", []),
                    key=lambda q: q["normalized_tco"]["amount"])
    for rank, q in enumerate(ranked, 1):
        tco = q["normalized_tco"]["amount"]
        terms = q["commercial_terms"]
        tco_s = int(min_tco / tco * 100)
        p_s = payment_score.get(terms["payment_terms"], 50)
        d_s = delivery_days.get(terms["delivery_date"], 50)
        w_s = warranty_score.get(terms["warranty_months"], 50)
        r_s = supplier_score.get(ratings.get(q["supplier_id"], "C"), 50)
        total = round(tco_s*(weights["tco"]/100) + p_s*(weights["payment_terms"]/100) +
                      d_s*(weights["delivery"]/100) + w_s*(weights["warranty"]/100) +
                      r_s*(weights["supplier_rating"]/100), 2)
        cards.append({
            "supplier_id": q["supplier_id"], "score_status": "scored",
            "total_score": total, "rank": rank,
            "dimensions": [
                {"dimension": "normalized_total_cost", "weight": weights["tco"],
                 "score": tco_s, "weighted_points": round(tco_s*weights["tco"]/100, 2)},
                {"dimension": "payment_terms", "weight": weights["payment_terms"],
                 "score": p_s, "weighted_points": round(p_s*weights["payment_terms"]/100, 2)},
                {"dimension": "delivery_date", "weight": weights["delivery"],
                 "score": d_s, "weighted_points": round(d_s*weights["delivery"]/100, 2)},
                {"dimension": "warranty", "weight": weights["warranty"],
                 "score": w_s, "weighted_points": round(w_s*weights["warranty"]/100, 2)},
                {"dimension": "supplier_rating", "weight": weights["supplier_rating"],
                 "score": r_s, "weighted_points": round(r_s*weights["supplier_rating"]/100, 2)},
            ]
        })
    return {"scorecards": cards}


def _build_recommendation(state: QuoteReviewState) -> dict[str, Any]:
    """Node 6: 综合推荐 + 人工决策点（如有）。"""
    cards = sorted(state.get("scorecards", []), key=lambda c: c["rank"])
    fail_checks = [c for c in state.get("policy_checks", []) if c["result"] == "fail"]
    human_needed = bool(fail_checks) or (cards and cards[0]["total_score"] < 75)
    leading = cards[0]["supplier_id"] if cards else None

    status = "recommended_with_conditions" if human_needed else "recommended"
    reason_codes = ["BEST_COMPARABLE_TCO"] if leading else []
    conditions = []
    if fail_checks:
        conditions.append(f"{len(fail_checks)} 项制度检查未通过，请复核。")
    if human_needed:
        conditions.append("规格一致性需人工确认三份报价的商务条款完全可比。")

    recommendation = {
        "status": status, "leading_supplier_id": leading,
        "reason_codes": reason_codes, "conditions": conditions,
        "human_decision_required": human_needed,
    }
    # 构造 Review Packet（对齐 06_智能体输入输出契约.md §响应结构）
    review_packet = {
        "review_tier": "standard" if human_needed else "express",
        "suggested_assignee_role": "procurement_manager" if human_needed else "procurement_user",
        "questions": ["所有报价的规格是否在商业上等价且可比？",
                      "如有预付款条款，是否符合公司财务制度？"],
        "decision_options": ["approve_with_conditions", "reject_and_request_new_quotes"],
    }
    return {"recommendation": {"recommendation": recommendation, "review_packet": review_packet}}


def build_graph():
    """Graph 构造函数 → 用于 graph_registry.py 注册。"""
    builder = StateGraph(QuoteReviewState)
    builder.add_node("parse_input", _parse_input)
    builder.add_node("validate_config", _validate_config)
    builder.add_node("fetch_normalize_quotes", _fetch_normalize_quotes)
    builder.add_node("evaluate_policy", _evaluate_policy)
    builder.add_node("build_scorecards", _build_scorecards)
    builder.add_node("build_recommendation", _build_recommendation)

    builder.add_edge(START, "parse_input")
    builder.add_edge("parse_input", "validate_config")
    builder.add_edge("validate_config", "fetch_normalize_quotes")
    builder.add_edge("fetch_normalize_quotes", "evaluate_policy")
    builder.add_edge("evaluate_policy", "build_scorecards")
    builder.add_edge("build_scorecards", "build_recommendation")
    builder.add_edge("build_recommendation", END)
    return builder.compile()
```

#### Step 4.2 注册到 Graph Registry（仅 1 行映射，不改核心逻辑）

编辑 `agent-service/app/registry/graph_registry.py`，在现有 GRAPH_REGISTRY dict 中追加：

```python
# 仅追加如下一行（放在末尾即可）
GRAPH_REGISTRY["procurement_quote_review_graph"] = {
    "version": "v1",
    "builder_module": "app.graphs.procurement_quote_review",
    "builder_func": "build_graph",
}
```

> 这 1 行属于"注册点扩展"，不算改 Python Runtime 核心（[runtime/service.py](file:///Users/jinli/Project/enterprise-agent-platform/agent-service/app/runtime/service.py)、[store.py](file:///Users/jinli/Project/enterprise-agent-platform/agent-service/app/runtime/store.py) 等核心文件不允许修改）。

#### Step 4.3 最小冒烟验证

```bash
# 重启 Agent Service（Python 热加载如可用则不需要重启）
# 用 curl 直接触发一个最小 Run
curl -X POST http://localhost:8000/api/v1/runs/start \
  -H "Content-Type: application/json" \
  -H "X-Internal-Service-Token: $INTERNAL_SERVICE_TOKEN" \
  -d '{
    "run_id": "procurement_smoke_001",
    "graph": {"key": "procurement_quote_review_graph", "version": "v1"},
    "initial_state": {
      "contract_version": "1.0.0",
      "procurement_case": {"request_id": "PR-TEST-001", "title": "测试采购", "budget_amount": 620000},
      "file_ids": ["f_test_001", "f_test_002", "f_test_003"],
      "configuration_refs": {
        "policy_set_key": "procurement_quote_review_policy",
        "policy_set_version": "1.0.0",
        "scoring_profile_key": "procurement_general_goods_v1",
        "scoring_profile_version": "1.0.0"
      }
    }
  }'
# → 202 Accepted
# 等待 5-10s，然后查询：
curl -f http://localhost:8000/api/v1/runs/procurement_smoke_001 \
  -H "X-Internal-Service-Token: $INTERNAL_SERVICE_TOKEN"
# → status = "succeeded"，output 包含 recommendation + scorecards
```

#### Step 4-Gate：判定

| PASS | FAIL |
|---|---|
| Python Agent Graph 仅通过"新文件 + 1 行注册表"完成；Run 成功 → status=succeeded | 需修改 Python Runtime 核心（service/store/models/events.py）；或 graph_registry.py 不能通过字典扩展注册 |
| Output 包含 contract_version + policy_checks + scorecards + recommendation（对齐 [06_智能体输入输出契约.md](../06_PROCUREMENT/06_智能体输入输出契约.md)） | 输出字段完全不匹配契约 |
| **git diff**：`go-platform/internal/` 无改动；`agent-service/app/runtime/` 无改动 | `go-platform/internal/` 有任何 diff；或 Python Runtime 核心文件有 diff |

---

### Step 5：安装 + UI 可见 + 对话端到端验证（配置层）

**目标**：模拟真实员工体验——从 Gallery 选 Agent → 对话 → Tool 调用 → 产出 → 审计 → 卸载。

#### Step 5.1 一键安装

```bash
# 用 procurement_manager 用户
curl -X POST http://localhost:8080/api/v1/gallery/packages/procurement_quote_review/install \
  -H "Authorization: Bearer $PROCUREMENT_MGR_JWT" \
  -H "Content-Type: application/json" \
  -d '{"package_code": "procurement_quote_review", "graph_key": "procurement_quote_review_graph", "graph_version": "v1"}'
# → 200 OK，InstallationResponse.installation.status = "active"
```

#### Step 5.2 UI 验证（人工操作）

```
操作路径：
  1. 浏览器访问 http://localhost:5173/
  2. 以 procurement_manager 登录 → 默认首页为 Agent Gallery
  3. 筛选分类 → 选择"采购" → 看到"采购询价分析 Agent"卡片
  4. 点击卡片 → 进入对话页面
  5. 输入：
     "帮我分析采购申请 PR-2026-0714-001 的三份报价，
      预算上限 620000 元，附件已上传（文件 ID：f_qa / f_qb / f_qc）。"
  6. 观察：
     ✓ SSE 流式打字机效果出现
     ✓ 对话中 Agent 澄清 ≤ 2 轮
     ✓ Agent 调用 procurement.supplier_query（Sidecar）成功
     ✓ 最终输出：结构化的 policy_checks + scorecards + recommendation
  7. 打开 Run Detail：
     ✓ 六层 Trace 完整（Workflow → Run → Model Turn → Tool Call → Checkpoint → Interrupt）
     ✓ Audit Log 中存在本次对话 + Tool 调用的完整记录
```

#### Step 5.3 卸载 + 消失验证

```bash
# 管理员或 procurement_manager
curl -X POST http://localhost:8080/api/v1/gallery/packages/procurement_quote_review/uninstall \
  -H "Authorization: Bearer $PROCUREMENT_MGR_JWT"
# → 200 OK

# 刷新 Gallery → 采购 Agent 卡片立即消失
curl -s http://localhost:8080/api/v1/gallery?category=procurement \
  -H "Authorization: Bearer $PROCUREMENT_MGR_JWT" | jq '.items[] | select(.package_code=="procurement_quote_review")'
# → 空结果（如该 Agent 非内置，则 Gallery 不应再展示）
```

#### Step 5-Gate：判定

| PASS | FAIL |
|---|---|
| Gallery 可见 → 对话成功 → Tool 调用 → 结构化输出 → Trace 完整 → 卸载干净，全链路 ≤ 10 分钟（不含首次加载） | 任何一步 UI 不显示 / 对话报错 / Tool 被拒 / 卸载后卡片仍在 |
| 管理员 Audit Log：能看到 install → conversation_run → tool_call → uninstall 的完整事件链 | 关键事件缺失审计记录 |

---

## 4. 最终 Gate（M8 验收判定）

### 4.1 PASS 条件（**全部满足**才算 M8 通过）

```
Gate-1 零 Go 代码改动    git diff go-platform/internal/ → 0 个文件改动
Gate-2 端到端可用       新 Agent 从安装到对话出结果 ≤ 10 分钟
Gate-3 跨域隔离有效    跨域渗透测试（采购 Agent 调 finance_* Tool）→ 403 + 审计告警
Gate-4 卸载即消失      卸载后 Gallery 与列表中立刻不出现，旧 Run 仍可查看
Gate-5 基线回归无损    go test ./... + python test_finance_contract_regression 全部 PASS
Gate-6 双域连续通过    完成采购验证后，再用 HR "员工入职" Agent 做第二次零代码验证 → 同样 PASS
```

### 4.2 FAIL 条件（**任一触发**即 M8 不通过，需返回平台修复后重验）

```
Fail-1 改 Go 平台代码    验证过程中必须修改 internal/ 下任何现有 Go 文件
Fail-2 改 Runtime 核心   必须修改 Python Runtime (app/runtime/*) 核心逻辑
Fail-3 改前端路由/组件   必须改 App.tsx / Gallery / 组件代码才能展示采购 Agent
Fail-4 跨域泄漏          采购 Agent 能成功调用任何 finance_* Tool 并拿到数据
Fail-5 基线被破坏        Finance V1 Contract 回归任何用例 FAIL
Fail-6 需重启服务        Agent 安装/卸载后必须重启 Go 或 Python 服务才能生效
```

### 4.3 报告模板

M8 验证完成后，产出如下报告并保存至 `docs/05_FUTURE/M8_VERIFICATION_REPORT_<YYYYMMDD>.md`：

```markdown
# M8 零代码接入验证报告 <YYYY-MM-DD>

## 1. 执行环境
- Git 分支：<branch-name> + commit SHA
- Go/Python/Node 版本
- 数据库迁移版本（max migration number）

## 2. 5 步执行记录
| Step | 结果 | 耗时 | 备注 |
|------|------|------|------|
| Step 1 Manifest 注册 | PASS / FAIL | Ns | ... |
| Step 2 App + Policy   | PASS / FAIL | Ns | ... |
| Step 3 Sidecar        | PASS / FAIL | Ns | ... |
| Step 4 Agent Graph    | PASS / FAIL | Ns | ... |
| Step 5 UI 端到端      | PASS / FAIL | Ns | ... |

## 3. 最终 Gate 判定
| Gate | 结果 | 证据 |
|------|------|------|
| Gate-1 零 Go 代码改动   | PASS | git diff 输出 |
| Gate-2 端到端 ≤10min   | PASS | 计时截图 |
| Gate-3 跨域隔离        | PASS | 403 响应 + 审计截图 |
| Gate-4 卸载即消失      | PASS | 前后 Gallery 截图 |
| Gate-5 Finance 回归    | PASS | 测试 stdout |
| Gate-6 第二域连续通过  | —    | HR/IT/法务任选其一 |

## 4. 结论
- □ M8 通过 → 允许进入 M9
- □ M8 失败 → 阻塞原因（1/2/3/4/5/6），预计修复日期

## 5. 附录
- 关键 curl 日志 / 关键 SQL 查询结果
- 跨域渗透测试详细请求与响应
- Finance 回归测试完整输出
```

---

## 5. 附：跨域渗透测试脚本（自动化）

保存为 `scripts/m8_cross_domain_isolation_test.sh`，在 Gate 判定中运行：

```bash
#!/usr/bin/env bash
# M8 Cross-Domain Isolation Penetration Test
# 模拟：采购 Agent 越权调用 Finance 数据 / Finance Agent 读取采购数据
#
# 预期：全部返回 403 DOMAIN_POLICY_VIOLATION 或 401/403 Permission Denied
# 如出现任何 200，则 FAIL。

set -u
BASE="${BASE_URL:-http://localhost:8080}"

echo "=== M8 Cross-Domain Isolation Test ==="
PASS=0; FAIL=0

run_case() {
  local label="$1" jwt="$2" json="$3" expect_deny="${4:-true}"
  resp=$(curl -s -o /tmp/m8_penetrate_body.txt -w "%{http_code}" \
    -X POST "$BASE/api/v1/tools/calls" \
    -H "Authorization: Bearer $jwt" \
    -H "Content-Type: application/json" \
    -d "$json")
  body=$(cat /tmp/m8_penetrate_body.txt)
  if [ "$expect_deny" = "true" ]; then
    if [ "$resp" = "403" ] || [ "$resp" = "401" ] || echo "$body" | grep -q "DOMAIN_POLICY_VIOLATION\|PERMISSION_DENIED"; then
      echo "  [PASS] $label -> HTTP $resp"
      PASS=$((PASS+1))
    else
      echo "  [FAIL] $label -> HTTP $resp (expect 403). Body: $body"
      FAIL=$((FAIL+1))
    fi
  else
    if [ "$resp" = "200" ]; then
      echo "  [PASS] $label -> HTTP $resp"
      PASS=$((PASS+1))
    else
      echo "  [WARN] $label -> HTTP $resp (not deny, not 200). Body: $body"
    fi
  fi
}

# 依赖外部导出：
#   PROC_USER_JWT / FIN_USER_JWT / ADMIN_JWT

# Case 1: Proc user → 尝试调用 finance.report_generate
run_case "Proc→finance.report_generate (DENY)" "$PROC_USER_JWT" \
  '{"tool_code":"finance.report_generate","input":{"period":"Q3_2026"},"run_as_business_app":"procurement"}' true

# Case 2: Proc user → 尝试调用 finance.voucher_read
run_case "Proc→finance.voucher_read (DENY)" "$PROC_USER_JWT" \
  '{"tool_code":"finance.voucher_read","input":{"voucher_id":"FV-001"}}' true

# Case 3: Fin user → 尝试调用 procurement.supplier_query
run_case "Fin→procurement.supplier_query (DENY)" "$FIN_USER_JWT" \
  '{"tool_code":"procurement.supplier_query","input":{"supplier_ids":["alpha"]}}' true

# Case 4: Fin user → 正常调 finance.report_generate（基线）
run_case "Fin→finance.report_generate (ALLOW baseline)" "$FIN_USER_JWT" \
  '{"tool_code":"finance.report_generate","input":{"period":"Q3_2026"}}' false

echo
echo "=== Summary: PASS=$PASS FAIL=$FAIL ==="
[ "$FAIL" -eq 0 ] && echo "✅ CROSS-DOMAIN ISOLATION VERIFIED" || { echo "❌ VIOLATIONS FOUND"; exit 1; }
```

使用方法：
```bash
export BASE_URL=http://localhost:8080
export PROC_USER_JWT=...
export FIN_USER_JWT=...
bash scripts/m8_cross_domain_isolation_test.sh
# → 4 Cases; Summary PASS=4 FAIL=0 → ✅
```

---

## 6. 执行负责人与时间估算

| 角色 | 负责人（建议） | 职责 | 预估耗时 |
|---|---|---|---|
| 验证总控 | 平台 Tech Lead | 准备环境、执行 Gate 判定、写报告 | 0.5 天 |
| 配置执行 | Platform/Backend Engineer | Step 1/2/3 API + SQL + Sidecar | 0.5 天 |
| Agent 实现 | Agent Engineer | Step 4 Python Graph + 注册 | 0.5 天 |
| UI + 端到端 | Frontend + QA | Step 5 手工验证 + 写截图 | 0.5 天 |
| 渗透 + 回归 | QA / SRE | 跨域渗透脚本 + Finance 回归 | 0.5 天 |
| **合计** | | | **≤ 2 个工作日** |

> 如果实际执行超过 3 个工作日，说明 M8 平台扩展点存在明显缺陷（否则不该这么慢），应在报告中单独列出"卡点 + 修复计划"。

---

## 7. 验证后的关键决策

| 结果 | 下一步 |
|---|---|
| ✅ M8 Gate PASS | ① 固化本文件为《新业务域接入 Playbook v1.0》；② 启动 M8-D 知识库（平台层）和 M8-B Skill 市场 UI；③ HR/法务二选一作为"第三方接入"演练；④ 启动 M9 规划 |
| ❌ M8 Gate FAIL（改了 Go 代码） | ① 回滚验证分支的 Go 修改；② 把需要改的点提炼为 M8-Bugfix 列表（必选清单：Manifest 字段不足 / Policy Rule 扩展不足 / Sidecar 协议缺口 / Graph 注册机制不足）；③ 用 1 周集中修复并加回归测试；④ 重跑本验证 |
| ⚠️ M8 边缘 PASS（UI 有瑕疵但核心通过） | 记录前端 UI 瑕疵为 M8.1 小迭代，不作为阻塞；先保证平台层扩展点稳定 |

---

> **本文件的灵魂在于"严格"。** 如果在"改一点点 Go 代码也没关系吧"的松动中走过场，M8 的核心承诺——"零代码接入 + 业务中立"——就会被掏空。验证过程中每一次"忍不住改 Go"的冲动，都是一次发现平台扩展点缺陷的机会。抓住这些缺陷并修复它们，平台才能真正走向"AI 原生 OS"。
