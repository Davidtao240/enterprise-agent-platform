# POST /api/v1/workflow-instances 完整笔记

> 本笔记梳理"根据模板创建工作流实例"的完整链路，覆盖 HTTP → Go → PostgreSQL 的全字段流转。

---

## 一、思维链总览

```
我要创建一个工作流实例 → 谁来创建？(JWT) → 用什么蓝图？(Template) → 蓝图里有什么？(Nodes+Edges)
→ 怎么实例化？(Instance + NodeInstances) → 怎么防重复？(Idempotency-Key) → 怎么追踪？(trace_id)
→ 最终写到哪里？(PostgreSQL 3张表) → 返回什么？(201 + 实例摘要)
```

---

## 二、手画调用链

```
POST /api/v1/workflow-instances
│
├─ Header: Authorization: Bearer <JWT>
├─ Header: Idempotency-Key: <可选，≤128字符>
├─ Header: X-Trace-Id: <可选，透传>
├─ Body: { business_app_code, workflow_template_key, title, input }
│
▼ ① TraceMiddleware          [platform/response.go]
│   生成/提取 trace_id → c.Set("trace_id")
│
▼ ② AuthMiddleware            [auth/middleware.go]
│   解析 JWT → c.Set("user_id"), c.Set("tenant_id"), c.Set("username")
│
▼ ③ RequirePermission("workflow:create")  [auth/middleware.go]
│   查 RBAC → user_id 是否有 workflow:create 权限
│
▼ ④ Handler.CreateInstance     [workflow/handler.go#L59]
│   c.ShouldBindJSON(&req)    ← Body 反序列化
│   c.GetString("user_id")    ← 从 Context 取
│   c.GetString("tenant_id")  ← 从 Context 取
│   c.GetHeader("Idempotency-Key")
│
▼ ⑤ Service.CreateInstance     [workflow/service.go#L118]
│   ├─ 幂等检查: FindInstanceByIdempotencyKey(tenantID, userID, key)
│   │   → 命中则直接返回已有实例
│   ├─ FindTemplateByBusinessAndKey(business_app_code, template_key)
│   │   → SELECT ... WHERE business_app_code=$1 AND workflow_template_key=$2 AND status='active'
│   ├─ engine.ParseDefinition(tmpl.DefinitionJSON)
│   │   → json.Unmarshal → TemplateDefinition{Nodes, Edges}
│   │   → ValidateDefinition: 检查 node id 唯一、edge 引用合法
│   ├─ traceID = uuid.New()                           ← Workflow Trace 生成
│   ├─ inst = &Instance{ Status:"draft", TraceID:traceID, TenantID:tenantID, ... }
│   ├─ repo.CreateInstance(inst)
│   │   → INSERT INTO workflow_instances (...) RETURNING id, created_at, updated_at
│   │   → PostgreSQL 生成 UUID（gen_random_uuid）
│   ├─ repo.CreateNodeInstances(inst.ID, def.Nodes)
│   │   → 遍历每个 TemplateNode，INSERT INTO workflow_node_instances
│   │   → 每条记录 status='pending'，UUID 由 PG 生成
│   │   → agent_graph 节点默认 max_retries=3
│   └─ auditLog("workflow_instance_created")
│       → INSERT INTO audit_logs
│
▼ ⑥ HTTP Response              [workflow/handler.go#L78]
│   201 Created
│   {
│     "trace_id": "<X-Trace-Id>",
│     "data": {
│       "id", "business_app_code", "workflow_template_key",
│       "workflow_template_version", "graph_key", "title",
│       "status": "draft", "trace_id"
│     }
│   }
```

---

## 三、学习问题逐项解答

### Q1: 路由注册在哪里，需要什么权限？

**注册位置：** `cmd/server/main.go#L199`

```go
protected.POST("/workflow-instances", require("workflow:create"), workflowHandler.CreateInstance)
```

- 权限码：`workflow:create`
- 中间件链顺序：`TraceMiddleware → AuthMiddleware → RequirePermission("workflow:create") → Handler`

### Q2: JWT 提供哪些 user_id、tenant_id？

**解析位置：** `auth/middleware.go#L41-L74`

JWT claims 结构（签发时）：

| Claim Key | 说明 | 注入到 gin.Context 的 key |
|-----------|------|--------------------------|
| `sub` | 用户 ID | `user_id` |
| `username` | 用户名 | `username` |
| `tenant_id` | 租户 ID | `tenant_id` |
| `exp` | 过期时间 | — |
| `iat` | 签发时间 | — |

Handler 通过 `c.GetString("user_id")` / `c.GetString("tenant_id")` 获取。

**user_id 本质：** `users` 表的主键 UUID，用户注册时 PG 生成，登录时签入 JWT 的 `sub` 字段。

**tenant_id 本质：** 租户隔离标识符，用户创建时关联到某个租户，登录时签入 JWT 的 `tenant_id` 字段。Repository 查询时自动加 `WHERE tenant_id = $1` 实现租户隔离。

**完整生命周期：**
```
用户注册 → INSERT INTO users (id=gen_random_uuid(), tenant_id='tenant-123')
用户登录 → JWT 签发 claims["sub"]=user.ID, claims["tenant_id"]=user.TenantID
后续请求 → AuthMiddleware 解析 JWT → c.Set("user_id"), c.Set("tenant_id")
Handler  → c.GetString("user_id"), c.GetString("tenant_id")
Service  → Instance.CreatedBy = userID, Instance.TenantID = tenantID
Repository → INSERT (created_by, tenant_id) + 查询时 WHERE tenant_id = $1
```

### Q3: Handler 读取哪些 Header 和 Request Body？

**位置：** `workflow/handler.go#L59-L82`

| 来源 | 字段 | 用途 |
|------|------|------|
| Header `Authorization` | `Bearer <JWT>` | 认证（由 AuthMiddleware 处理） |
| Header `Idempotency-Key` | 幂等键 | 防重复创建，≤128 字符 |
| Header `X-Trace-Id` | 追踪 ID | 透传给响应（由 TraceMiddleware 处理） |
| Context `user_id` | 用户 ID | 记录 createdBy |
| Context `tenant_id` | 租户 ID | 租户隔离 |
| Body `business_app_code` | 业务域 | 查模板 + 写入实例 |
| Body `workflow_template_key` | 模板 Key | 查模板 |
| Body `title` | 实例标题 | 写入实例 |
| Body `input` | 用户输入 | JSON 序列化后写入 input_json |

**Request Body 结构：** `workflow/model.go#L122-L127`

```go
type CreateInstanceRequest struct {
    BusinessAppCode     string         `json:"business_app_code" binding:"required"`
    WorkflowTemplateKey string         `json:"workflow_template_key" binding:"required"`
    Title               string         `json:"title" binding:"required"`
    Input               map[string]any `json:"input"`
}
```

### Q4: Service 如何查询 Template？

**位置：** `workflow/repository.go#L26-L39`

```sql
SELECT id, business_app_code, workflow_template_key, name, version,
       graph_key, definition_json, status, created_at, updated_at
FROM workflow_templates
WHERE business_app_code = $1
  AND workflow_template_key = $2
  AND status = 'active'
  AND deleted_at IS NULL
ORDER BY created_at DESC LIMIT 1
```

- 只查 `active` 状态的模板
- 按 `created_at DESC` 取最新版本
- 两个条件定位：`business_app_code` + `workflow_template_key`

### Q5: definition_json 如何解析为 Nodes 和 Edges？

**位置：** `workflow/engine.go#L74-L118`

```go
func (e *Engine) ParseDefinition(definitionJSON string) (*TemplateDefinition, error) {
    var def TemplateDefinition
    json.Unmarshal([]byte(definitionJSON), &def)
    e.ValidateDefinition(&def)  // 校验完整性
    return &def, nil
}
```

**数据结构：** `workflow/model.go#L99-L120`

```go
type TemplateDefinition struct {
    Nodes []TemplateNode `json:"nodes"`
    Edges []TemplateEdge `json:"edges"`
}

type TemplateNode struct {
    ID         string `json:"id"`          // 如 "upload"
    Type       string `json:"type"`        // file_upload / agent_graph / human_review / system
    Name       string `json:"name"`        // 显示名
    Required   bool   `json:"required"`
    GraphKey   string `json:"graph_key"`   // agent_graph 节点关联的 Python Graph
    Role       string `json:"role"`        // human_review 节点的审批角色
    MaxRetries int    `json:"max_retries"`
}

type TemplateEdge struct {
    From string `json:"from"`  // 源节点 ID
    To   string `json:"to"`    // 目标节点 ID
    When string `json:"when"`  // 条件: succeeded / approved / rejected / failed
}
```

**校验规则（ValidateDefinition）：**
- 至少有一个 node
- node id 不能为空且唯一
- edge 的 from/to 必须指向已存在的 node

**注意：** Edges 存储在 `workflow_templates.definition_json` 中，不会单独写入 `workflow_node_instances` 表。引擎在运行时从模板定义中解析 Edges 来决定节点流转。

### Q6: 如何创建一条 workflow_instances？

**位置：** `workflow/repository.go#L108-L118`

```sql
INSERT INTO workflow_instances
  (business_app_code, workflow_template_id, workflow_template_key,
   workflow_template_version, graph_key, title, status,
   input_json, created_by, trace_id, idempotency_key, tenant_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id, created_at, updated_at
```

- `id`：PostgreSQL 自动生成 `gen_random_uuid()`
- `status`：Go 代码写入 `"draft"`
- `trace_id`：Go 代码 `uuid.New().String()` 生成
- `input_json`：用户输入 `json.Marshal(req.Input)` 序列化
- `created_at` / `updated_at`：PostgreSQL 默认 `now()`

### Q7: 如何创建多条 workflow_node_instances？

**位置：** `workflow/repository.go#L256-L272`

```go
func (r *Repository) CreateNodeInstances(ctx context.Context, workflowInstanceID string, nodes []TemplateNode) error {
    for _, node := range nodes {
        maxRetries := node.MaxRetries
        if maxRetries == 0 && node.Type == NodeTypeAgentGraph {
            maxRetries = 3  // agent_graph 默认重试 3 次
        }
        r.pool.Exec(ctx,
            `INSERT INTO workflow_node_instances
             (workflow_instance_id, node_key, node_type, name, status, max_retries)
             VALUES ($1,$2,$3,$4,$5,$6)`,
            workflowInstanceID, node.ID, node.Type, node.Name,
            NodeStatusPending, maxRetries)
    }
    return nil
}
```

- 遍历模板定义中的每个 `TemplateNode`
- 每个节点初始状态为 `pending`
- `agent_graph` 类型节点默认 `max_retries=3`，其他默认 `0`
- `id`：PostgreSQL 自动生成
- `retry_count`：默认 `0`

### Q8: Workflow Trace 和各类数据库 ID 在哪里生成？

| 标识符 | 生成方式 | 生成位置 |
|--------|---------|---------|
| **Workflow Trace ID** | Go `uuid.New().String()` | `workflow/service.go#L138` |
| **workflow_instances.id** | PostgreSQL `gen_random_uuid()` | `migrations/002_workflow_and_agent.up.sql#L22` |
| **workflow_node_instances.id** | PostgreSQL `gen_random_uuid()` | `migrations/002_workflow_and_agent.up.sql#L45` |
| **audit_logs.id** | PostgreSQL `gen_random_uuid()` | migration 中定义 |

### Q9: Idempotency-Key 当前怎样参与创建？

**位置：** `workflow/service.go#L118-L164`

```
1. 如果 Idempotency-Key != "":
   → FindInstanceByIdempotencyKey(tenantID, userID, key)
   → 如果找到已有实例 → 直接返回（不重复创建）

2. 如果没找到 / 创建时冲突:
   → 再次 FindInstanceByIdempotencyKey → 返回已有实例

3. 如果 Idempotency-Key == "":
   → 直接创建，不做幂等检查
```

**SQL 查询：** `workflow/repository.go#L120-L134`

```sql
SELECT ... FROM workflow_instances
WHERE tenant_id = $1 AND created_by = $2 AND idempotency_key = $3
```

注意：幂等键的三元组是 `(tenant_id, created_by, idempotency_key)`，即不同用户/租户的相同 Key 不会冲突。

### Q10: HTTP Response 返回哪些字段？

**位置：** `workflow/handler.go#L78-L82` + `workflow/model.go#L129-L138`

```json
HTTP 201 Created
{
  "trace_id": "xxx-xxx-xxx",
  "data": {
    "id": "uuid-由PG生成",
    "business_app_code": "risk_review",
    "workflow_template_key": "document_review_v1",
    "workflow_template_version": "1.0.0",
    "graph_key": "doc_analysis_graph",
    "title": "用户提交的标题",
    "status": "draft",
    "trace_id": "uuid-由Go生成"
  }
}
```

---

## 四、请求字段 → Go 结构体 → 数据库字段映射表

### 4.1 Request Body → Go Struct → DB（workflow_instances）

| HTTP Body 字段 | Go 结构体字段 | Go 类型 | 数据库字段 | DB 类型 | 字段来源 |
|---------------|-------------|---------|-----------|---------|---------|
| `business_app_code` | `CreateInstanceRequest.BusinessAppCode` | string | `workflow_instances.business_app_code` | VARCHAR(64) | 前端传入 |
| `workflow_template_key` | `CreateInstanceRequest.WorkflowTemplateKey` | string | `workflow_instances.workflow_template_key` | VARCHAR(128) | 前端传入 |
| `title` | `CreateInstanceRequest.Title` | string | `workflow_instances.title` | VARCHAR(255) | 前端传入 |
| `input` | `CreateInstanceRequest.Input` | map[string]any | `workflow_instances.input_json` | JSONB | 前端传入，Go 序列化 |
| —（JWT 解析） | `Service.CreateInstance(userID)` | string | `workflow_instances.created_by` | UUID | JWT `sub` |
| —（JWT 解析） | `Service.CreateInstance(tenantID)` | string | `workflow_instances.tenant_id` | VARCHAR | JWT `tenant_id` |
| —（Go 生成） | `Instance.TraceID` | string | `workflow_instances.trace_id` | VARCHAR(128) | `uuid.New()` |
| —（硬编码） | `Instance.Status` | string | `workflow_instances.status` | VARCHAR(32) | 常量 `"draft"` |
| —（查模板得来） | `Instance.WorkflowTemplateID` | string | `workflow_instances.workflow_template_id` | UUID | `workflow_templates.id` |
| —（查模板得来） | `Instance.WorkflowTemplateVersion` | string | `workflow_instances.workflow_template_version` | VARCHAR(32) | `workflow_templates.version` |
| —（查模板得来） | `Instance.GraphKey` | string | `workflow_instances.graph_key` | VARCHAR(128) | `workflow_templates.graph_key` |
| —（Header 传入） | `Instance.IdempotencyKey` | *string | `workflow_instances.idempotency_key` | VARCHAR | `Idempotency-Key` Header |
| —（PG 生成） | `Instance.ID` | string | `workflow_instances.id` | UUID | `gen_random_uuid()` |
| —（PG 生成） | `Instance.CreatedAt` | time.Time | `workflow_instances.created_at` | TIMESTAMPTZ | `now()` |
| —（PG 生成） | `Instance.UpdatedAt` | time.Time | `workflow_instances.updated_at` | TIMESTAMPTZ | `now()` |

### 4.2 Template Definition → Node Instances → DB（workflow_node_instances）

| TemplateDefinition 字段 | TemplateNode 字段 | 数据库字段 | DB 类型 | 说明 |
|------------------------|------------------|-----------|---------|------|
| `nodes[].id` | `TemplateNode.ID` | `workflow_node_instances.node_key` | VARCHAR(128) | 模板中节点 ID |
| `nodes[].type` | `TemplateNode.Type` | `workflow_node_instances.node_type` | VARCHAR(64) | file_upload / agent_graph / human_review / system |
| `nodes[].name` | `TemplateNode.Name` | `workflow_node_instances.name` | VARCHAR(128) | 显示名 |
| `nodes[].max_retries` | `TemplateNode.MaxRetries` | `workflow_node_instances.max_retries` | INT | agent_graph 默认 3 |
| —（硬编码） | — | `workflow_node_instances.status` | VARCHAR(32) | 固定 `"pending"` |
| —（PG 生成） | — | `workflow_node_instances.id` | UUID | `gen_random_uuid()` |
| —（传入） | — | `workflow_node_instances.workflow_instance_id` | UUID | 关联 workflow_instances.id |
| —（PG 默认） | — | `workflow_node_instances.retry_count` | INT | 默认 0 |

### 4.3 Edges（不持久化为独立表）

| TemplateEdge 字段 | 类型 | 说明 |
|------------------|------|------|
| `from` | string | 源节点 ID |
| `to` | string | 目标节点 ID |
| `when` | string | 条件：succeeded / approved / rejected / failed |

Edges 存储在 `workflow_templates.definition_json` 中，引擎在运行时解析来决定节点流转。

---

## 五、字段出生地分类

### 5.1 数据库本身就定义的（Schema 约束）

| 字段 | DB 定义 | 默认值 | 说明 |
|------|---------|--------|------|
| `id` | `UUID PRIMARY KEY DEFAULT gen_random_uuid()` | 自动生成 UUID | Go 不传，PG 生成 |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | 当前时间 | Go 不传，PG 生成 |
| `updated_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | 当前时间 | Go 不传，PG 生成 |
| `status` | `VARCHAR(32) NOT NULL DEFAULT 'draft'` | `'draft'` | DB 有默认值，Go 显式传 |
| `input_json` | `JSONB NOT NULL DEFAULT '{}'` | `'{}'` | DB 有默认值，Go 显式传 |
| `output_json` | `JSONB` (nullable) | `NULL` | 创建时不存在，归档时写入 |
| `started_at` | `TIMESTAMPTZ` (nullable) | `NULL` | 创建时为空，Start 时写入 |
| `finished_at` | `TIMESTAMPTZ` (nullable) | `NULL` | 创建时为空，完成/取消时写入 |
| `deleted_at` | `TIMESTAMPTZ` (nullable) | `NULL` | 软删除标记 |

### 5.2 前端 HTTP 请求生成的

| 字段 | 前端怎么产生 |
|------|------------|
| `business_app_code` | 前端页面选择业务域 |
| `workflow_template_key` | 前端页面选择模板 |
| `title` | 前端输入框填写 |
| `input` | 前端表单填写 / 文件上传后组装 |
| `idempotency_key` | 前端生成唯一字符串放入 Header |

### 5.3 Go 后端运行时生成的

| 字段 | Go 怎么产生 |
|------|------------|
| `trace_id` | `uuid.New().String()` 代码生成 |
| `status` | 常量 `StatusDraft = "draft"` 硬编码 |

### 5.4 从其他 DB 表查询得来的

| 字段 | 从哪查来 |
|------|---------|
| `workflow_template_id` | `SELECT id FROM workflow_templates WHERE ...` |
| `workflow_template_version` | `SELECT version FROM workflow_templates WHERE ...` |
| `graph_key` | `SELECT graph_key FROM workflow_templates WHERE ...` |

### 5.5 从 JWT 解析得来的

| 字段 | JWT claim | 原始来源 |
|------|-----------|---------|
| `created_by` | `sub` | `users.id`（用户注册时 PG 生成） |
| `tenant_id` | `tenant_id` | `tenants.id`（租户创建时 PG 生成） |

### 5.6 字段流转全景图

```
┌──────────────┬──────────┬──────────┬──────────┬──────────┬──────────────┐
│  前端 HTTP   │  JWT     │  Go 代码  │  查 DB   │  PG 自动  │  PG 默认值   │
│  生成        │  解析    │  生成     │  得来    │  生成     │  (可覆盖)    │
├──────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ business_    │ user_id  │ trace_id  │ workflow_│ id       │ status       │
│  app_code    │(→created │ (uuid.New│  _       │ (gen_    │ (='draft')   │
│              │  _by)    │  ())     │  template│  random_ │              │
│ workflow_    │ tenant_id│ status   │  _id     │  uuid()) │ input_json   │
│  template_   │          │ (='draft'│ workflow_│ created_ │ (='{}')      │
│  key         │          │  硬编码) │  _       │  at(now) │              │
│              │          │          │  version │ updated_ │ output_json  │
│ title        │          │          │ graph_key│  at(now) │ (=NULL)      │
│              │          │          │          │          │              │
│ input        │          │          │          │          │ started_at   │
│ (→input_json)│          │          │          │          │ (=NULL)      │
│              │          │          │          │          │              │
│ idempotency_ │          │          │          │          │ finished_at  │
│  key (Header)│          │          │          │          │ (=NULL)      │
└──────────────┴──────────┴──────────┴──────────┴──────────┴──────────────┘
       ↓            ↓          ↓          ↓          ↓          ↓
       └────────────┴──────────┴──────────┴──────────┴──────────┘
                                    ↓
                          INSERT INTO workflow_instances
                                    ↓
                          PostgreSQL 存储完整记录
```

---

## 六、创建前后数据库记录对照

### 6.1 创建前（只有模板）

```
workflow_templates:
┌──────────────────────────────────────────────────────────────────────────┐
│ id: a1b2c3d4-...                                                         │
│ business_app_code: risk_review                                           │
│ workflow_template_key: document_review_v1                                │
│ version: 1.0.0                                                           │
│ graph_key: doc_graph                                                     │
│ status: active                                                           │
│ definition_json: {                                                       │
│   "nodes": [                                                             │
│     {"id":"upload","type":"file_upload","name":"上传"},                   │
│     {"id":"analyze","type":"agent_graph","name":"分析","graph_key":"doc_graph"}, │
│     {"id":"review","type":"human_review","name":"复核","role":"reviewer"},     │
│     {"id":"archive","type":"system","name":"归档"}                        │
│   ],                                                                     │
│   "edges": [                                                             │
│     {"from":"upload","to":"analyze","when":"succeeded"},                 │
│     {"from":"analyze","to":"review","when":"succeeded"},                 │
│     {"from":"review","to":"archive","when":"approved"}                   │
│   ]                                                                      │
│ }                                                                        │
└──────────────────────────────────────────────────────────────────────────┘

workflow_instances:      (空)
workflow_node_instances: (空)
audit_logs:              (空)
```

### 6.2 创建后（POST 返回 201）

```
workflow_instances (1 条):
┌────────────────────┬──────────────┬────────┬──────────┬──────────┬──────────────┐
│ id                 │ tenant_id    │ status │ trace_id │ created_ │ business_    │
│                    │              │        │          │ by       │ app_code     │
├────────────────────┼──────────────┼────────┼──────────┼──────────┼──────────────┤
│ 7f8a9b0c-1d2e...   │ tenant-123   │ draft  │ 550e8400 │ user-456 │ risk_review  │
│ title: "季度报告审核"                                                      │
│ input_json: {"doc_url":"...","quarter":"Q2"}                             │
│ workflow_template_key: document_review_v1                                 │
│ workflow_template_version: 1.0.0                                          │
│ graph_key: doc_graph                                                     │
│ output_json: NULL  started_at: NULL  finished_at: NULL                   │
└──────────────────────────────────────────────────────────────────────────┘

workflow_node_instances (4 条，全部 pending):
┌────────────────────┬────────────────────┬──────────┬─────────────┬────────┬────────────┐
│ id                 │ workflow_          │ node_key │ node_type   │ status │ max_retries│
│                    │ instance_id        │          │             │        │            │
├────────────────────┼────────────────────┼──────────┼─────────────┼────────┼────────────┤
│ e1f2a3b4-...       │ 7f8a9b0c-1d2e...   │ upload   │ file_upload │ pending│ 0          │
│ c3d4e5f6-...       │ 7f8a9b0c-1d2e...   │ analyze  │ agent_graph │ pending│ 3          │
│ g7h8i9j0-...       │ 7f8a9b0c-1d2e...   │ review   │ human_review│ pending│ 0          │
│ k1l2m3n4-...       │ 7f8a9b0c-1d2e...   │ archive  │ system      │ pending│ 0          │
└────────────────────┴────────────────────┴──────────┴─────────────┴────────┴────────────┘
（retry_count 全部为 0，input_json/output_json/error_json 全部为 NULL）

audit_logs (1 条):
┌──────────┬────────────────────────┬───────────────────────┬────────┐
│ trace_id │ action                 │ resource_id           │ status │
├──────────┼────────────────────────┼───────────────────────┼────────┤
│ 550e8400 │ workflow_instance_     │ 7f8a9b0c-1d2e...      │ draft  │
│          │ created                │                       │        │
└──────────┴────────────────────────┴───────────────────────┴────────┘
```

### 6.3 API 请求/响应对照

```json
// 请求
POST /api/v1/workflow-instances
Headers:
  Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
  Idempotency-Key: client-req-001
Body:
{
  "business_app_code": "risk_review",
  "workflow_template_key": "document_review_v1",
  "title": "季度报告审核",
  "input": {"doc_url": "s3://reports/q2.pdf", "quarter": "Q2"}
}

// 响应
HTTP 201 Created
{
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "data": {
    "id": "7f8a9b0c-1d2e-3f4a-5b6c-7d8e9f0a1b2c",
    "business_app_code": "risk_review",
    "workflow_template_key": "document_review_v1",
    "workflow_template_version": "1.0.0",
    "graph_key": "doc_graph",
    "title": "季度报告审核",
    "status": "draft",
    "trace_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

### 6.4 实际 INSERT 语句

```sql
-- Go 组装 + PG 执行的完整 INSERT
INSERT INTO workflow_instances (
  business_app_code,          -- 'risk_review'          ← 前端传
  workflow_template_id,       -- 'a1b2c3d4-...'         ← 查模板得来
  workflow_template_key,      -- 'document_review_v1'   ← 前端传
  workflow_template_version,  -- '1.0.0'                ← 查模板得来
  graph_key,                  -- 'doc_graph'            ← 查模板得来
  title,                      -- '季度报告审核'          ← 前端传
  status,                     -- 'draft'                ← Go 硬编码
  input_json,                 -- '{"doc_url":"s3://...","quarter":"Q2"}' ← 前端传+Go序列化
  created_by,                 -- 'user-456'             ← JWT 解析
  trace_id,                   -- '550e8400-...'         ← Go 生成
  idempotency_key,            -- 'client-req-001'       ← 前端 Header
  tenant_id                   -- 'tenant-123'           ← JWT 解析
) RETURNING
  id,         -- '7f8a9b0c-...'  ← PG 生成
  created_at, -- 2026-07-26 ...  ← PG 生成
  updated_at; -- 2026-07-26 ...  ← PG 生成

-- 4 条节点 INSERT（遍历模板 nodes）
INSERT INTO workflow_node_instances (workflow_instance_id, node_key, node_type, name, status, max_retries)
VALUES ('7f8a9b0c-...', 'upload',  'file_upload',  '上传', 'pending', 0);

INSERT INTO workflow_node_instances (workflow_instance_id, node_key, node_type, name, status, max_retries)
VALUES ('7f8a9b0c-...', 'analyze', 'agent_graph',  '分析', 'pending', 3);

INSERT INTO workflow_node_instances (workflow_instance_id, node_key, node_type, name, status, max_retries)
VALUES ('7f8a9b0c-...', 'review',  'human_review', '复核', 'pending', 0);

INSERT INTO workflow_node_instances (workflow_instance_id, node_key, node_type, name, status, max_retries)
VALUES ('7f8a9b0c-...', 'archive', 'system',       '归档', 'pending', 0);

-- 1 条审计日志
INSERT INTO audit_logs (trace_id, actor_user_id, business_app_code, action, resource_type, resource_id, status, detail_json)
VALUES ('550e8400-...', 'user-456', 'risk_review', 'workflow_instance_created', 'workflow_instance', '7f8a9b0c-...', 'draft', '{}');
```

---

## 七、3 分钟脱稿口述

> 这个请求是 `POST /api/v1/workflow-instances`，作用是根据模板创建一个工作流实例。
>
> 请求进来后，先经过三个中间件：**TraceMiddleware** 生成或透传 trace_id；**AuthMiddleware** 从 JWT 解析出 user_id 和 tenant_id 注入到 gin.Context；**RequirePermission** 检查当前用户是否有 `workflow:create` 权限。
>
> 然后进入 **Handler**，它做三件事：反序列化 JSON Body 拿到 business_app_code、workflow_template_key、title、input 四个字段；从 Context 拿 user_id 和 tenant_id；从 Header 拿可选的 Idempotency-Key。这些全部传给 Service。
>
> **Service** 的核心逻辑是"把模板实例化"。首先用 business_app_code + workflow_template_key 去 PostgreSQL 查一条 active 状态的模板记录。拿到模板后，把 definition_json 这个 JSONB 字段反序列化成 TemplateDefinition 结构体，里面包含 nodes 数组和 edges 数组。
>
> 接着生成一个 **Workflow Trace ID**（Go 代码用 uuid.New 生成），组装一条 Instance 记录，状态写死为 draft，用户输入序列化为 input_json，然后 INSERT 到 workflow_instances 表，id 由 PostgreSQL 的 gen_random_uuid 自动生成。
>
> Instance 创建完后，遍历模板定义中的每个 node，逐条 INSERT 到 workflow_node_instances 表，每条记录状态都是 pending，关联到刚才创建的 instance id。agent_graph 类型的节点默认 max_retries=3，其他默认 0。注意 edges 不会单独建表，它们只存在模板的 definition_json 里，引擎在运行时解析来决定节点流转。
>
> 最后写一条审计日志，返回 201 Created，响应体包含实例 id、状态 draft、trace_id 等字段。
>
> 幂等机制是：如果请求带了 Idempotency-Key，先按 tenant_id + created_by + key 三元组查已有实例，命中就直接返回，不重复创建。
>
> 总结一句话：**Template 是蓝图，definition_json 定义了节点和边；CreateInstance 把蓝图实例化为一条 Instance 记录 + N 条 NodeInstance 记录，全部初始状态为 draft/pending，等待后续 Start 接口触发执行。**

---

## 八、字段溯源速查表

| workflow_instances 字段 | 出生地点 | 流转路径 |
|------------------------|---------|---------|
| `id` | PG 自动生成 | `gen_random_uuid()` → `RETURNING id` → `Scan(&inst.ID)` |
| `tenant_id` | JWT `tenant_id` claim | 用户注册时关联 → 登录签入 JWT → AuthMiddleware 解析 → `c.Set("tenant_id")` → Handler 取出 → Service → `INSERT (tenant_id)` |
| `business_app_code` | 前端 HTTP Body | `c.ShouldBindJSON(&req)` → `CreateInstanceRequest.BusinessAppCode` → `Instance.BusinessAppCode` → `INSERT (business_app_code)` |
| `workflow_template_id` | DB 查模板得来 | `FindTemplateByBusinessAndKey()` → `SELECT id FROM workflow_templates` → `tmpl.ID` → `Instance.WorkflowTemplateID` → `INSERT (workflow_template_id)` |
| `workflow_template_key` | 前端 HTTP Body | `c.ShouldBindJSON(&req)` → `CreateInstanceRequest.WorkflowTemplateKey` → `Instance.WorkflowTemplateKey` → `INSERT (workflow_template_key)` |
| `workflow_template_version` | DB 查模板得来 | `FindTemplateByBusinessAndKey()` → `SELECT version FROM workflow_templates` → `tmpl.Version` → `Instance.WorkflowTemplateVersion` → `INSERT (workflow_template_version)` |
| `graph_key` | DB 查模板得来 | `FindTemplateByBusinessAndKey()` → `SELECT graph_key FROM workflow_templates` → `tmpl.GraphKey` → `Instance.GraphKey` → `INSERT (graph_key)` |
| `title` | 前端 HTTP Body | `c.ShouldBindJSON(&req)` → `CreateInstanceRequest.Title` → `Instance.Title` → `INSERT (title)` |
| `status` | Go 硬编码 | 常量 `StatusDraft = "draft"` → `Instance.Status` → `INSERT (status)` |
| `input_json` | 前端 HTTP Body + Go 序列化 | `c.ShouldBindJSON(&req)` → `CreateInstanceRequest.Input` (map) → `json.Marshal(req.Input)` → `Instance.InputJSON` (string) → `INSERT (input_json)` |
| `output_json` | PG 默认 NULL | 创建时不传，归档时由 `UpdateInstanceOutput` 写入 |
| `created_by` | JWT `sub` claim | 用户注册时 PG 生成 → 登录签入 JWT → AuthMiddleware 解析 → `c.Set("user_id")` → Handler 取出 → Service → `Instance.CreatedBy` → `INSERT (created_by)` |
| `trace_id` | Go 运行时生成 | `uuid.New().String()` → `Instance.TraceID` → `INSERT (trace_id)` |
| `idempotency_key` | 前端 HTTP Header | `c.GetHeader("Idempotency-Key")` → Service 参数 → `Instance.IdempotencyKey` → `INSERT (idempotency_key)` |
| `started_at` | PG 默认 NULL | 创建时不传，Start 时由 `UpdateInstanceStatus` 写入 |
| `finished_at` | PG 默认 NULL | 创建时不传，完成/取消时写入 |
| `created_at` | PG 自动生成 | `DEFAULT now()` → `RETURNING created_at` → `Scan(&inst.CreatedAt)` |
| `updated_at` | PG 自动生成 | `DEFAULT now()` → `RETURNING updated_at` → `Scan(&inst.UpdatedAt)` |
