# API Design

> 文档状态：Active Specification
> 更新日期：2026-08-18

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

## Durable Run API（M1 Target）

```text
POST /internal/v2/agent-runs
POST /internal/v2/agent-runs/{run_id}/resume
POST /internal/v2/agent-runs/{run_id}/cancel
POST /internal/v2/runtime-events

GET  /api/v1/agent-runs/{run_id}
GET  /api/v1/agent-runs/{run_id}/steps
GET  /api/v1/agent-runs/{run_id}/events
```

V2 Start 返回 accepted/queued，不保持 HTTP 连接直到 Graph 全部完成。现有 `/internal/v1/agent-runs` 在 Finance 迁移期继续兼容。

## Tool Execution API（M2 Target）

```text
POST /internal/v1/tool-calls
GET  /internal/v1/tool-calls/{tool_call_id}
POST /internal/v1/tool-calls/{tool_call_id}/reconcile
```

Internal API 只允许经过认证的服务身份访问；用户/Tenant/Agent/Skill 身份由 Go 从 Run 快照恢复，不接受模型自报覆盖。

## Agent Gallery API（M7 Target）

员工第一屏的 Agent 发现与选择层。完整契约见 [AGENT_GALLERY.md](AGENT_GALLERY.md)。

```text
GET   /api/v1/agent-gallery?category=&business_app=&q=   # business_app:read，仅 published
GET   /api/v1/agent-gallery/{code}                        # business_app:read，详情+示例 Prompt
POST  /api/v1/agent-packages                              # agent:manage，注册包
PATCH /api/v1/agent-packages/{code}                       # agent:manage，更新/上下架
```

`agent_package.graph_key` 注册时校验，运行时不可改写；画廊不放宽权限与 Domain Policy。

## Conversation API（M7 Target）

对话式入口。完整契约（SSE 事件、澄清语义、预算）见 [CONVERSATION_ENGINE.md](CONVERSATION_ENGINE.md)，选型见 [ADR-007](../02_ARCHITECTURE/ADR-007_CONVERSATION_ENGINE_SSE.md)。

```text
POST  /api/v1/conversations                     # conversation:write，创建会话 {agent_package_code, title?}
GET   /api/v1/conversations?limit=&status=      # conversation:read，我的会话列表
GET   /api/v1/conversations/{id}                # conversation:read，详情+消息（分页倒序）
PATCH /api/v1/conversations/{id}                # conversation:write，重命名/关闭
POST  /api/v1/conversations/{id}/messages       # conversation:write，发消息 → 202 {message_id, run_id}
POST  /api/v1/conversations/{id}/answers        # conversation:write，回答澄清 {interrupt_id, answer} → Resume
POST  /api/v1/conversations/{id}/cancel         # conversation:write，取消当前 Run
GET   /api/v1/conversations/{id}/stream         # conversation:read，SSE（text/event-stream）
```

规则：

- 发消息返回 `202`（受理）；输出经 SSE 流接收，POST 语义不是"完成"。
- SSE 断线以 `Last-Event-Id` 续传；SSE 通道只读，写操作一律走 REST 端点。
- 同一会话同时只允许一个活跃 Run；并发发送返回 `409`。
- 权限点 `conversation:read` / `conversation:write` 授予全部默认角色。
