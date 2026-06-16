# Agent Protocol

## 目标

Agent Protocol 定义 Go 平台后端与 Python Agent 服务之间的统一调用协议。

该协议必须跨业务通用。V1 虽然只接财务 Agent，但后续 HR、采购、合同、IT、客服 Agent 都复用同一套协议。

路由规则：

```text
Workflow Template -> graph_key -> Python Graph Router -> Agent Graph
```

Agent 不负责跨业务选路，LLM 也不能自行决定进入财务 Graph 或 HR Graph。

## Agent 注册格式

```json
{
  "agent_id": "finance_analysis_agent",
  "name": "Finance Analysis Agent",
  "domain": "finance",
  "capabilities": ["metric_analysis", "trend_summary", "risk_explanation"],
  "endpoint": "http://agent-service:8000/internal/v1/agent-runs",
  "input_schema": {},
  "output_schema": {},
  "status": "active"
}
```

## Tool 注册格式

```json
{
  "tool_id": "generate_report",
  "name": "Generate Report",
  "domain": "finance",
  "risk_level": "medium",
  "input_schema": {},
  "output_schema": {},
  "status": "active"
}
```

## V1 Agent 列表

- DataExtractAgent
- SchemaMappingAgent
- ValidationAgent
- FinanceAnalysisAgent
- ReportAgent
- ReviewSummaryAgent

## Agent 复用规则

Agent 分为两类：

- domain-specific Agent：只服务某个业务域，例如 FinanceAnalysisAgent、ResumeParseAgent。
- shared Agent：可跨业务复用，例如 DataExtractAgent、ValidationAgent、ReportAgent、NotificationAgent。

shared Agent 在不同 Graph 中只能调用当前业务域授权的 Tool。

## 后续业务 Agent 扩展

### HR

- ResumeParseAgent
- MaterialCheckAgent
- PolicyCheckAgent
- OnboardingNoticeAgent

### Procurement

- RequirementParseAgent
- SupplierCompareAgent
- BudgetCheckAgent
- PurchaseOrderAgent

### Legal

- ContractExtractAgent
- ClauseRiskAgent
- PolicyRAGAgent
- LegalReportAgent

### IT Service

- IncidentClassifyAgent
- LogAnalysisAgent
- SolutionRecommendAgent
- TicketUpdateAgent

### Customer Service

- IntentClassifyAgent
- KnowledgeAnswerAgent
- TicketCreateAgent
- QualityReviewAgent

## 扩展约束

新增 Agent 时必须：

- 注册到 Agent Registry
- 声明 domain 和 capabilities
- 声明 input_schema 和 output_schema
- 绑定允许调用的 Tool
- 通过 Agent Gateway 被调用
- 输出结构化 JSON

新增 Agent 还必须声明：

- domain
- reusable_scope: domain_only 或 shared
- allowed_graphs，可选
- input_schema
- output_schema
