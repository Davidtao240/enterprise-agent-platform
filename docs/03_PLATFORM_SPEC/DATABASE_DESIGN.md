# Database Design

## 数据库

V1 使用 PostgreSQL。

## 多业务扩展原则

V1 不能只建财务专用表，平台必须先有通用表，再通过配置和 JSONB 支撑财务场景。

不要依赖这类强绑定表：

```text
finance_tasks
finance_reports
finance_approvals
```

推荐通用表：

```text
business_apps
workflow_templates
workflow_instances
workflow_node_instances
agent_registry
tool_registry
agent_tool_permissions
approval_tasks
audit_logs
agent_run_logs
files
business_form_data
domain_policies
graph_registry
```

## 核心表

### business_apps

V1 初始化可仅有 `finance`，但表结构必须支持未来新增 `hr`、`procurement`、`legal`、`it_service`、`customer_service`。

### workflow_templates

后续新增业务时，只新增模板记录，不修改 Workflow Engine。

关键字段：

- business_app_code
- workflow_template_key
- definition_json
- graph_key
- status

`graph_key` 用于显式路由 Python Agent Graph。

### graph_registry

记录可调用的 Python Agent Graph。

字段建议：

- graph_key
- business_app_code
- name
- version
- status
- description

### domain_policies

记录业务域隔离策略。

字段建议：

- business_app_code
- allowed_agent_domains
- allowed_tool_domains
- allow_shared_agents
- allow_shared_tools
- high_risk_requires_review

### agent_registry

需要包含：

- agent_id
- domain
- reusable_scope
- capabilities_json
- input_schema_json
- output_schema_json
- status

### tool_registry

需要包含：

- tool_id
- domain
- risk_level
- is_shared
- input_schema_json
- output_schema_json
- status

### business_form_data

业务差异字段建议先放 JSONB 中：

- finance_operating_report: revenue, cost, net_profit, department, month
- hr_onboarding_review: candidate_name, position, resume_file_id
- procurement_request: item_name, supplier_ids, budget_amount
- contract_review: contract_file_id, counterparty, contract_amount
- it_incident_ticket: severity, system_name, incident_summary

## 扩展策略

财务流程成熟后可新增：

- finance_reports
- finance_metrics

HR 流程成熟后可新增：

- candidates
- resumes
- onboarding_records

采购流程成熟后可新增：

- suppliers
- purchase_requests
- quotes

合同流程成熟后可新增：

- contracts
- contract_review_items
- legal_risk_findings

IT 服务成熟后可新增：

- incidents
- service_requests
- change_requests

客服工单成熟后可新增：

- customer_tickets
- ticket_messages
- sla_records
