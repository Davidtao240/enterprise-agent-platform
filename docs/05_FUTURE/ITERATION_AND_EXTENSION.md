# Iteration and Extension

## 核心原则

后续扩展不是在代码里堆 if else，而是围绕四个扩展点：

- Business App
- Workflow Template
- Agent Registry
- Tool Registry

V1 必须从一开始实现这四个扩展点，即使 V1 只启用 finance。否则后续扩展 HR、采购、合同、IT、客服时会被财务场景锁死。

扩展设计采用：

```text
Workflow Template 显式路由 Graph
Graph 按流程隔离
Agent 按能力复用
Tool 按权限隔离
Domain Policy 做业务域约束
```

## 各层扩展清单

### 数据库层

新增业务时通常新增：

- business_apps 记录
- workflow_templates 记录
- agent_registry 记录
- tool_registry 记录
- agent_tool_permissions 记录
- 可选 business_form_data schema
- graph_registry 记录
- domain_policies 记录

成熟后再新增领域表：

- HR: candidates, resumes, onboarding_records
- 采购: procurement_requests, suppliers, quotes
- 合同: contracts, contract_review_items
- IT: incidents, service_requests
- 客服: customer_tickets, sla_records

### 后端层

优先复用：

- BusinessAppService
- WorkflowTemplateService
- WorkflowInstanceService
- WorkflowEngine
- AgentRegistryService
- ToolRegistryService
- AgentGateway
- ApprovalService
- AuditLogService
- FileService

新增业务时只新增少量领域 adapter 或 result assembler，不新增一套独立流程引擎。

### Agent 层

新增领域 Agent，并注册能力、输入输出 schema、可调用工具。

Agent 不能直接写平台数据库，必须通过 Go 后端的工具或回调完成状态变更。

Graph 按流程隔离，Agent 可复用。新增业务时优先复用 shared Agent，例如 ValidationAgent、ReportAgent，但必须通过 Domain Policy 限制它们只能使用当前业务域授权的 Tool。

### 前端层

复用：

- Dashboard
- Workflow Template List
- Workflow Instance List
- Workflow Detail
- Approval Page
- Audit Log
- Agent Run Log

新增：

- 业务入口卡片
- 业务创建表单
- 业务结果展示页

## 扩展验收标准

新增一个业务场景时，应满足：

- 不修改核心 Workflow Engine
- 不修改 Agent Gateway 协议
- 只新增模板、Agent、Tool 和少量业务页面
- 审计日志和 Agent Run Log 自动复用
- 权限系统可控制新业务入口
- 前端 Dashboard 能通过 Business App API 展示新入口
- 新业务流程能通过 Workflow Template 解释执行
- 新 Agent 只能调用被授权的 Tool
- 新业务的数据能先用 JSONB 承载，后续再拆领域表
- 新 workflow template 必须声明 graph_key
- 新 graph_key 必须注册到 graph_registry
- 新业务必须配置 Domain Policy
