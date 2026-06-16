# API Design

## API 风格

- 外部 API 使用 REST。
- 内部 Agent 调用使用 HTTP JSON。
- 所有请求携带 trace_id。

## Business App API

```text
GET /api/v1/business-apps
GET /api/v1/business-apps/{code}
```

前端必须从该接口动态获取业务入口，V1 只返回 finance，也不要写死业务菜单。

## Workflow API

```text
GET   /api/v1/workflow-templates
GET   /api/v1/business-apps/{code}/workflow-templates
GET   /api/v1/workflow-templates/{id}

POST  /api/v1/workflow-instances
GET   /api/v1/workflow-instances
GET   /api/v1/workflow-instances/{id}
POST  /api/v1/workflow-instances/{id}/start
POST  /api/v1/workflow-instances/{id}/cancel
POST  /api/v1/workflow-instances/{id}/retry
GET   /api/v1/workflow-instances/{id}/nodes
GET   /api/v1/workflow-instances/{id}/events
```

创建流程实例时应传入 `business_app_code` 和 `workflow_template_key`。

示例：

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "title": "2026 年 5 月经营数据填报",
  "input": {}
}
```

后端根据 workflow template 中的 `graph_key` 调用 Python Agent Graph。前端不直接提交 agent_id 或 graph 内部节点。

## Internal Agent API

```text
POST /internal/v1/agent-runs
```

Agent 调用 API 是跨业务通用协议。新增 HR、采购、合同、IT、客服场景时，不新增一套独立 Agent API，而是复用该接口，通过 `agent_id` 和 `context.business_app` 区分。

请求必须包含：

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {}
}
```

Python Agent Service 只能根据 `graph_key` 路由 Graph，不允许让 LLM 自行决定跨业务路由。
