# Roadmap

## Phase 0: 文档与项目骨架

- 完成 docs
- 创建 monorepo 结构
- 初始化 Go、Python、React 项目
- 编写 Docker Compose

## Phase 1: 平台基础能力

- Auth
- User
- Department
- Role
- Permission
- RBAC

## Phase 2: Workflow Core

- Workflow Template
- Workflow Instance
- Workflow Node Instance
- 状态机
- 异步任务
- 多业务模板解释执行

Workflow Engine 不依赖 finance 专用代码。

本阶段必须实现 `graph_key` 字段和基础 Graph 路由，不要等到新增 HR/采购时再补。

## Phase 3: Agent Registry and Gateway

- Agent Registry
- Tool Registry
- Agent Tool Permission
- Agent Gateway
- Agent Run Log
- Graph Registry
- Domain Policy 校验

## Phase 4: Python Agent Service

- DataExtractAgent
- SchemaMappingAgent
- ValidationAgent
- FinanceAnalysisAgent
- ReportAgent
- ReviewSummaryAgent

## Phase 5: Frontend Workbench

- 登录页
- 财务中心
- 任务列表
- 新建任务
- 文件上传
- 流程详情
- 人工确认
- 报告预览
- 审计日志

## Phase 6: 审计、可观测性和部署

- 审计日志完善
- Agent 执行日志完善
- token/cost 统计
- Docker Compose 一键启动
- 演示数据

## Phase 7: 后续扩展

新增业务时必须复用：

- Business App
- Workflow Template
- Workflow Engine
- Agent Gateway
- Tool Permission
- Approval Task
- Audit Log
- Agent Run Log

### HR 流程

- BusinessApp: hr
- WorkflowTemplate: hr_onboarding_review
- Agents: ResumeParseAgent, MaterialCheckAgent, PolicyCheckAgent, OnboardingNoticeAgent
- Tools: parse_resume, query_position, query_hr_policy, create_onboarding_task, send_notification

### 采购流程

- BusinessApp: procurement
- WorkflowTemplate: procurement_request
- Agents: RequirementParseAgent, SupplierCompareAgent, BudgetCheckAgent, PurchaseOrderAgent
- Tools: query_supplier, compare_quote, query_budget, create_purchase_order

### 合同法务流程

- BusinessApp: legal
- WorkflowTemplate: contract_review
- Agents: ContractExtractAgent, ClauseRiskAgent, PolicyRAGAgent, LegalReportAgent
- Tools: parse_contract, query_legal_policy, generate_contract_risk_report

### IT 服务流程

- BusinessApp: it_service
- WorkflowTemplate: incident_ticket
- Agents: IncidentClassifyAgent, LogAnalysisAgent, SolutionRecommendAgent
- Tools: query_logs, create_ticket, notify_owner

### 客服工单流程

- BusinessApp: customer_service
- WorkflowTemplate: customer_ticket
- Agents: IntentClassifyAgent, KnowledgeAnswerAgent, TicketCreateAgent, QualityReviewAgent
- Tools: query_knowledge_base, create_customer_ticket, update_sla_record
