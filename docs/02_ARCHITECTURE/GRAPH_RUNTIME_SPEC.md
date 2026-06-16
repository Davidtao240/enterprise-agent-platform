# Graph Runtime Spec

## 目的

定义 Python Agent Service 中的 Graph 如何被路由、执行、记录和返回结果。

## 核心原则

- Graph 按流程隔离。
- Go 只传 `graph_key`，不传内部 Agent 编排细节。
- Python 通过 Graph Registry 选择对应 Graph。
- Graph 执行只发生在已授权的业务域内。

## Graph Registry

```python
GRAPH_REGISTRY = {
    "finance_operating_report_graph": finance_operating_report_graph,
    "hr_onboarding_review_graph": hr_onboarding_review_graph,
    "procurement_request_graph": procurement_request_graph,
    "contract_review_graph": contract_review_graph
}
```

## Graph 输入

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {
    "file_id": "file_001"
  },
  "context": {
    "user_id": "user_001",
    "department_id": "finance",
    "tenant_id": "default"
  }
}
```

## Graph 输出

```json
{
  "run_id": "run_001",
  "graph_key": "finance_operating_report_graph",
  "status": "succeeded",
  "output": {
    "summary": "本月收入环比增长 8.2%",
    "warnings": [],
    "result_file_id": "file_099"
  },
  "usage": {
    "model": "qwen-plus",
    "prompt_tokens": 1200,
    "completion_tokens": 600,
    "cost": 0.03
  },
  "error": null
}
```

## Graph 执行流程

1. Go 调用 Python Agent Service。
2. Python 根据 `graph_key` 从 Graph Registry 获取 Graph。
3. Graph 依次调用内部 Agent。
4. Agent 调用 Tool 时要经过 Tool 权限校验。
5. Graph 返回结构化结果。
6. Python 保存运行记录。
7. Go 更新 workflow node 状态。

## Graph 隔离规则

- 不同 Graph 之间默认不共享内部状态。
- 一个 Graph 的节点变更不影响其他 Graph。
- Graph 可以共享 Agent，但不能共享未授权 Tool。
- Graph 可以复用通用 Agent，例如 ValidationAgent、ReportAgent。

## 运行时约束

- Graph 执行必须可重入和可重试。
- Graph 输入输出必须结构化。
- Graph 失败时要返回明确错误码。
- Graph 不应直接写业务最终结果，必须由 Go 后端确认状态变更。

## 错误码建议

- GRAPH_NOT_FOUND
- GRAPH_VERSION_NOT_FOUND
- AGENT_NOT_ALLOWED
- TOOL_NOT_ALLOWED
- DOMAIN_POLICY_VIOLATION
- GRAPH_EXECUTION_FAILED
- TIMEOUT

