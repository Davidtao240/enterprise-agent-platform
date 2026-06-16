# Workflow Template Schema

## 目的

定义业务流程模板的结构，让 Go Workflow Engine 可以稳定解释执行，并通过 `graph_key` 显式路由到对应 Python Agent Graph。

## 核心原则

- 一个 Workflow Template 对应一个业务流程定义。
- Workflow Template 必须声明 `business_app_code` 和 `workflow_template_key`。
- Workflow Template 必须声明 `graph_key`。
- Workflow Template 只描述流程，不直接实现 Agent 逻辑。
- 业务差异通过模板表达，不通过 Go 代码 if else 表达。

## 模板结构

推荐定义为 JSON 或数据库中的 JSONB。

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "name": "经营数据填报",
  "version": "1.0.0",
  "graph_key": "finance_operating_report_graph",
  "description": "财务经营数据填报与分析流程",
  "nodes": [
    {
      "id": "upload",
      "type": "file_upload",
      "name": "上传经营数据",
      "required": true
    },
    {
      "id": "agent_graph",
      "type": "agent_graph",
      "name": "智能体分析",
      "graph_key": "finance_operating_report_graph",
      "input_mapping": {
        "file_id": "$files[0].id"
      }
    },
    {
      "id": "human_review",
      "type": "human_review",
      "name": "人工确认",
      "role": "finance_manager",
      "required": true
    },
    {
      "id": "archive",
      "type": "system",
      "name": "归档",
      "action": "archive_result"
    }
  ],
  "edges": [
    { "from": "upload", "to": "agent_graph" },
    { "from": "agent_graph", "to": "human_review" },
    { "from": "human_review", "to": "archive", "when": "approved" }
  ]
}
```

## 节点类型

### file_upload

用于上传文件、附件或表单材料。

### agent_graph

用于调用 Python Agent Graph。

必须包含：

- graph_key
- input_mapping
- output_mapping，可选

### human_review

用于人工确认、驳回或补充意见。

必须包含：

- role
- required

### system

用于归档、通知、写入结果、生成审计数据等系统动作。

## 分支与条件

支持简单条件分支：

```json
{
  "from": "human_review",
  "to": "archive",
  "when": "approved"
}
```

推荐先支持：

- approved
- rejected
- failed

后续可扩展更复杂条件，但 V1 不建议引入过度复杂的 DSL。

## 版本策略

- `workflow_template_key` 表示模板逻辑身份。
- `version` 表示模板版本。
- 新任务默认使用最新稳定版。
- 历史任务必须保留当时绑定的模板版本。

## 与 Graph 的关系

`agent_graph` 节点中的 `graph_key` 必须显式填写。

Go Workflow Engine 执行到 `agent_graph` 时：

1. 读取模板中的 `graph_key`
2. 创建 workflow_node_instance
3. 调用 Go Agent Gateway
4. 由 Python Graph Router 路由到具体 Graph
5. 返回结构化结果
6. Go 更新节点状态

## 设计限制

- 模板不应直接写死 Agent 内部调用顺序。
- 模板不应直接调用 Tool。
- 模板不应包含模型提示词。
- 模板不应混入业务代码。

