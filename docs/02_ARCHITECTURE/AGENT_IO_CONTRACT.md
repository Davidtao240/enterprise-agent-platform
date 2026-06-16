# Agent IO Contract

## 目的

定义 Go 平台后端与 Python Agent / Graph / Tool 之间的输入输出合同。

## 统一请求体

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "agent_id": "finance_analysis_agent",
  "tool_id": "query_finance_data",
  "input": {},
  "context": {
    "user_id": "user_001",
    "department_id": "finance",
    "tenant_id": "default"
  }
}
```

## 统一响应体

```json
{
  "run_id": "run_001",
  "status": "succeeded",
  "output": {},
  "usage": {
    "model": "qwen-plus",
    "prompt_tokens": 1200,
    "completion_tokens": 600,
    "cost": 0.03
  },
  "error": null
}
```

## 必填字段

- trace_id
- business_app_code
- workflow_template_key
- graph_key
- workflow_instance_id
- node_instance_id
- input

## 可选字段

- agent_id
- tool_id
- context.user_id
- context.department_id
- context.tenant_id

## 响应约束

- `status` 只能是 `succeeded`、`failed`、`retrying`、`cancelled`
- `output` 必须是结构化 JSON
- `error` 失败时必须存在
- `usage` 应记录 token 和 cost

## 幂等与重试

- `trace_id` 用于跨服务追踪。
- `run_id` 用于单次执行标识。
- 同一 `node_instance_id` 重试时可生成新的 `run_id`，但必须保留同一 `trace_id`。
- Go 后端基于 `node_instance_id + retry_count` 管理重试。

## Agent 输出规范

### 成功输出

```json
{
  "summary": "分析完成",
  "warnings": [],
  "data": {},
  "next_actions": []
}
```

### 失败输出

```json
{
  "code": "SCHEMA_VALIDATION_FAILED",
  "message": "Missing required field revenue",
  "retryable": false
}
```

## Tool 调用规范

Tool 调用必须先通过以下校验：

1. agent_id 存在且 active
2. tool_id 存在且 active
3. agent_tool_permissions 允许
4. Domain Policy 允许
5. risk_level 合规

## 共享 Agent 约束

shared Agent 可以跨 Graph 复用，但必须满足：

- 只能访问当前 Graph domain 允许的 Tool
- 只能返回当前业务域可解释的输出
- 不能绕过 Go 的审批和审计

