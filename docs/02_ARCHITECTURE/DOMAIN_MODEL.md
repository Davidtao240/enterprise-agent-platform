# Domain Model

## 核心领域对象

### User

企业内部用户。

### Department

组织部门。

### Role

角色。

### Permission

权限点。

### BusinessApp

业务应用入口。

V1 只有 finance，后续可新增 hr、procurement、legal、it_service、customer_service。

字段建议：

- id
- code
- name
- description
- icon
- sort_order
- status
- created_at
- updated_at

BusinessApp 是业务入口和权限边界的基础，不能写死在前端菜单或 Go 路由中。

### WorkflowTemplate

流程模板。

V1：

- finance_operating_report

后续可新增：

- hr_onboarding_review
- procurement_request
- contract_review
- it_incident_ticket
- customer_ticket

WorkflowTemplate 必须通过 definition_json 描述节点，不能依赖 if else 分业务执行。

WorkflowTemplate 必须显式声明 `graph_key`。Go 后端根据该字段调用 Python Agent Graph。

### WorkflowInstance

一次流程运行实例。

状态：

- draft
- running
- waiting_review
- approved
- rejected
- archived
- failed
- cancelled

### WorkflowNodeInstance

流程节点运行实例。

### Agent

智能体定义。

Agent 应按 domain 和 capabilities 注册，而不是按财务/HR 代码包硬编码。

Agent 字段建议：

- agent_id
- name
- domain
- reusable_scope: domain_only 或 shared
- capabilities
- input_schema
- output_schema
- status

领域 Agent 通常 `domain_only`，通用 Agent 可以是 `shared`。

### Tool

工具定义。

不同业务域应拥有不同工具，Agent 调用必须经过授权。

示例：

- finance: query_finance_data, generate_finance_report
- hr: parse_resume, query_position, query_hr_policy
- procurement: query_supplier, compare_quote, query_budget
- legal: parse_contract, query_legal_policy
- it_service: query_logs, create_ticket
- customer_service: query_knowledge_base, create_customer_ticket

Tool 字段建议：

- tool_id
- name
- domain
- risk_level: low / medium / high
- input_schema
- output_schema
- status

Tool 默认按 domain 隔离。shared Tool 必须显式标记。

### DomainPolicy

业务域策略，用于约束 Graph、Agent、Tool 的组合关系。

字段建议：

- id
- business_app_code
- allowed_agent_domains
- allowed_tool_domains
- allow_shared_agents
- allow_shared_tools
- high_risk_requires_review
- created_at
- updated_at

DomainPolicy 解决 Agent 可复用带来的跨域风险。

### ApprovalTask

人工确认任务。

### AuditLog

业务审计日志。

### AgentRunLog

Agent 执行日志。

### File

上传文件和生成文件的元数据。

## 扩展原则

新增业务场景时，优先新增配置和领域 Agent，而不是改平台核心模型。

通用模型必须稳定：

- BusinessApp
- WorkflowTemplate
- WorkflowInstance
- WorkflowNodeInstance
- Agent
- Tool
- ApprovalTask
- AuditLog
- AgentRunLog
- File
