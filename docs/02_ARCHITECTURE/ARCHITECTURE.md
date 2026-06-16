# Architecture

## 总体架构

```text
React / TypeScript Frontend
        |
Go Platform Backend
        |
Python FastAPI Agent Service
        |
PostgreSQL + Redis + MinIO + Qdrant
```

## 架构原则

V1 只启用 finance，但架构必须面向多业务扩展。

平台核心不能写死为财务专用逻辑。后续 HR、采购、合同、IT、客服都应通过新增 Business App、Workflow Template、Agent、Tool 和少量业务页面接入。

本项目采用：

```text
Workflow Template 显式路由 Graph
Graph 按流程隔离
Agent 按能力复用
Tool 按权限隔离
Domain Policy 做业务域约束
```

具体设计见 `GRAPH_ROUTING_AND_ISOLATION.md`。

## 服务职责

### React Frontend

负责企业工作台：

- 登录
- 财务中心
- 任务列表
- 新建填报任务
- 文件上传
- 数据预览
- 流程状态展示
- Agent 执行轨迹
- 人工确认 / 驳回
- 报告预览
- 审计日志展示
- Agent/Tool 管理

### Go Platform Backend

负责平台控制面和企业级后端能力：

- Auth
- User / Department / Role / Permission
- Business App
- Workflow Template
- Workflow Instance
- Workflow Node Instance
- Agent Registry
- Tool Registry
- Agent Gateway
- File Metadata
- Audit Log
- Agent Run Log
- Approval Task
- Cost Usage
- Async Job

Go 后端必须保持业务无关，财务逻辑只能落在：

- workflow template 配置
- finance domain agent
- 少量财务结果页面适配

### Python Agent Service

负责智能体执行层：

- DataExtractAgent
- SchemaMappingAgent
- ValidationAgent
- FinanceAnalysisAgent
- ReportAgent
- ReviewSummaryAgent
- RAG 检索
- LLM 调用
- 结构化输出

## 核心调用链

```text
User
-> Frontend
-> Go Backend 创建 workflow_instance
-> Go Backend 上传文件并保存 file metadata
-> Go Backend 投递异步任务
-> Asynq Worker 执行 workflow node
-> Agent Gateway 调用 Python Agent Service
-> Python Agent 返回结构化结果
-> Go Backend 校验结果并更新节点状态
-> Frontend 通过轮询或 SSE 展示进度
-> Human Review
-> Archive
```

## Agent Gateway

Agent Gateway 是 Go 后端中的统一 Agent 调用入口。

职责：

- 根据 agent_id 查询 Agent Registry。
- 检查当前流程是否允许调用该 Agent。
- 检查 Agent 是否有对应 Tool 权限。
- 构造标准请求发送给 Python Agent Service。
- 设置超时、重试、错误处理。
- 记录 Agent Run Log。
- 记录 token/cost usage。
- 写入审计日志。
- 校验 graph_key 是否属于当前 workflow template。
- 校验 Agent/Tool 是否满足 Domain Policy。

## 工作流执行原则

- 流程定义来自 Workflow Template。
- Workflow Template 通过 `graph_key` 显式选择 Python Agent Graph。
- 流程运行时生成 Workflow Instance。
- 每个节点运行时生成 Workflow Node Instance。
- Agent 节点通过异步任务执行。
- Human Review 节点进入 waiting_review 状态。
- 每次状态变化必须持久化。
- 每次关键操作必须写审计日志。

Go Workflow Engine 管业务流程状态，Python Agent Graph 管智能体子流程。Go 不需要知道 Graph 内部每个 Agent 的具体执行细节，但必须知道 `graph_key`、状态、结果和审计。

## 后续扩展架构

后续扩展其他业务场景时，不重写主链路，而是新增：

- Business App
- Workflow Template
- Domain Agent
- Tool Permission
- Business Form Schema
- Result View

扩展流程：

```text
新增 Business App
-> 新增 Workflow Template
-> 注册领域 Agent
-> 注册领域 Tool
-> 配置 Agent Tool Permission
-> 新增业务表单 schema
-> 复用通用 Workflow Detail / Audit / Agent Run Log 页面
-> 只为特殊结果新增定制页
```

例如 HR 入职材料审核：

```text
ResumeParseAgent
-> MaterialCheckAgent
-> HumanReview
-> OnboardingNoticeAgent
```

例如采购申请：

```text
RequirementParseAgent
-> SupplierCompareAgent
-> BudgetCheckAgent
-> HumanReview
-> PurchaseOrderAgent
```

如果未来新增合同法务、IT 服务、客服工单，也必须遵守同样的扩展模式，不能在 Workflow Engine 里写业务 if else。
