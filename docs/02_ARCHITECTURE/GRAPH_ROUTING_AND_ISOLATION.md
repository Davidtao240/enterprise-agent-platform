# Graph Routing and Isolation

## 核心原则

本平台采用以下设计：

```text
Workflow Template 显式路由 Graph
Graph 按流程隔离
Agent 按能力复用
Tool 按权限隔离
Domain Policy 做业务域约束
```

这套设计的目标是：既保证企业流程可控，又允许通用 Agent 能力复用，避免每个业务场景都复制一套系统。

## 总体关系

```text
Business App
  -> Workflow Template
    -> graph_key
      -> Python Agent Graph
        -> Agent Registry
          -> Tool Registry
            -> Agent Tool Permission
              -> Domain Policy
```

示例：

```text
Business App: finance
Workflow Template: finance_operating_report
graph_key: finance_operating_report_graph
Agents: DataExtractAgent, ValidationAgent, FinanceAnalysisAgent, ReportAgent
Tools: parse_csv, query_finance_data, generate_finance_report
```

## Workflow Template 显式路由 Graph

前端用户选择业务入口和流程模板，而不是选择 Agent。

前端请求：

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report"
}
```

Go 后端读取 workflow template：

```json
{
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "nodes": [
    {
      "id": "upload",
      "type": "file_upload"
    },
    {
      "id": "agent_graph",
      "type": "agent_graph",
      "graph_key": "finance_operating_report_graph"
    },
    {
      "id": "human_review",
      "type": "human_review",
      "role": "finance_manager"
    },
    {
      "id": "archive",
      "type": "system",
      "action": "archive"
    }
  ]
}
```

Go 后端执行到 `agent_graph` 节点时，调用 Python Agent Service：

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {}
}
```

## Graph 按流程隔离

每个流程模板可以对应一个独立 Agent Graph。

示例：

```text
finance_operating_report_graph
hr_onboarding_review_graph
procurement_request_graph
contract_review_graph
it_incident_ticket_graph
customer_ticket_graph
```

Graph 隔离意味着：

- 财务 Graph 的节点变化不影响 HR Graph。
- HR Graph 的异常不影响采购流程。
- 合同审核可以有自己的 RAG 和风险识别链路。
- 每个 Graph 可以独立测试、版本化和回滚。

不推荐做一个 `enterprise_super_graph` 包含所有业务判断。超级 Graph 会导致权限边界模糊、调试困难、变更风险变大。

## Agent 按能力复用

Agent 不一定和 Graph 一一绑定。Agent 是能力单元，可以被多个 Graph 复用。

通用 Agent：

```text
DataExtractAgent
ValidationAgent
ReportAgent
NotificationAgent
PolicyRAGAgent
```

领域 Agent：

```text
FinanceAnalysisAgent
ResumeParseAgent
SupplierCompareAgent
ContractRiskAgent
IncidentClassifyAgent
CustomerTicketAgent
```

例如 `ReportAgent` 可以用于：

- finance_operating_report_graph
- contract_review_graph
- procurement_request_graph

但它在不同业务域中只能使用当前业务域允许的模板、工具和数据。

## Tool 按权限隔离

Tool 是企业系统和外部能力的调用边界，必须强隔离。

示例：

```text
FinanceAnalysisAgent -> query_finance_data, generate_finance_report
ResumeParseAgent -> parse_resume, query_hr_policy
SupplierCompareAgent -> query_supplier, compare_quote
ContractRiskAgent -> parse_contract, query_legal_policy
```

不允许：

```text
ResumeParseAgent -> query_finance_data
SupplierCompareAgent -> query_hr_policy
ReportAgent -> 任意业务报表工具
```

Tool 调用必须经过 Go Agent Gateway，并检查：

- 当前 workflow 的 business_app_code
- 当前 graph_key
- 当前 agent_id
- 当前 tool_id
- agent_tool_permissions
- domain policy
- tool risk_level

## Domain Policy 做业务域约束

Domain Policy 用来解决“Agent 可复用”带来的跨域风险。

基本规则：

- Graph 归属于某个 business_app/domain。
- Agent 可以是 domain-specific，也可以是 shared。
- Tool 可以是 domain-specific，也可以是 shared。
- domain-specific Agent 默认只能进入同 domain 的 Graph。
- shared Agent 可以进入多个 Graph，但只能使用当前 Graph domain 授权的 Tool。
- high risk Tool 必须进入 human review 或显式审批。

示例：

```text
finance graph:
  shared ReportAgent -> only finance report tools

hr graph:
  shared ReportAgent -> only hr report tools

legal graph:
  shared PolicyRAGAgent -> only legal policy knowledge base
```

## Python Graph Router

Python Agent Service 中维护 Graph Registry：

```python
GRAPH_REGISTRY = {
    "finance_operating_report_graph": finance_operating_report_graph,
    "hr_onboarding_review_graph": hr_onboarding_review_graph,
    "procurement_request_graph": procurement_request_graph,
    "contract_review_graph": contract_review_graph,
}
```

Graph Router 只根据 Go 后端传入的 `graph_key` 路由，不让 LLM 自己决定跨业务 Graph。

禁止：

```text
让 Planner Agent 判断这是财务还是 HR，然后自由选择工具。
```

允许：

```text
在一个已确定的 Graph 内部，让 Planner Agent 做局部任务拆解。
```

## Go Agent Gateway 校验顺序

执行 Agent Graph 前：

1. 校验 workflow_template_key 是否属于 business_app_code。
2. 校验 graph_key 是否属于 workflow_template。
3. 校验当前用户是否有该 workflow 的执行权限。
4. 创建 agent_graph 类型的 workflow_node_instance。
5. 调用 Python Agent Service。

Graph 内部请求 Tool 时：

1. 校验 agent_id 是否存在且 active。
2. 校验 tool_id 是否存在且 active。
3. 校验 agent_tool_permissions。
4. 校验 Agent domain / Tool domain / business_app_code。
5. 校验 risk_level，必要时进入 human review。
6. 记录 audit_logs 和 agent_run_logs。

## 实现边界

Go 负责：

- Workflow Template
- Workflow Instance
- Graph 路由参数
- Agent Gateway
- Tool 权限
- Domain Policy
- 审计日志
- 人工确认

Python 负责：

- 根据 graph_key 路由具体 Graph
- 执行 Graph 内部 Agent 协作
- LLM/RAG/文档解析
- 返回结构化结果

前端负责：

- 展示 Business App
- 展示 Workflow Template
- 展示流程实例状态
- 展示 Agent Graph 执行摘要
- 展示人工确认和审计日志

