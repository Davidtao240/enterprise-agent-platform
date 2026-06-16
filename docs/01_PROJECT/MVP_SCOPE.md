# MVP Scope

## V1 范围原则

V1 要做全面，但只围绕一个业务场景做深：

**经营数据填报 / 财务分析 / 报表生成 / 人工确认 / 审计日志**

不要在 V1 同时落地 HR、采购、合同审核等场景。

但 V1 的技术实现必须保留多业务扩展能力，不能因为只做财务就把数据库、后端、前端写成财务专用系统。

必须预留的扩展点：

- Business App
- Workflow Template
- Workflow Instance
- Workflow Node Instance
- Agent Registry
- Tool Registry
- Agent Tool Permission
- Approval Task
- Audit Log
- Agent Run Log
- Graph Registry
- Domain Policy

## 必做功能

### 认证与权限

- 用户登录
- JWT 鉴权
- 用户管理
- 部门管理
- 角色管理
- 权限管理
- 财务中心访问权限
- 人工确认权限

### 财务业务入口

- 财务中心首页
- 经营数据填报任务列表
- 新建填报任务
- 上传 Excel/CSV
- 查看任务状态
- 查看历史归档

注意：财务中心只是 V1 第一业务入口，前端和后端都应按 Business App 的方式组织。

### 工作流

- Workflow Template 初始化
- Workflow Instance 创建
- Workflow Node Instance 状态流转
- 节点失败重试
- 人工确认节点
- 归档节点
- Workflow Engine 必须通过模板解释执行，不允许把财务流程写死在代码里。
- Workflow Template 必须通过 graph_key 显式路由 Python Agent Graph。

### Agent

- DataExtractAgent
- SchemaMappingAgent
- ValidationAgent
- FinanceAnalysisAgent
- ReportAgent
- ReviewSummaryAgent

### 审计与日志

- 审计日志
- Agent 执行日志
- 状态变更日志
- 文件上传日志
- 人工确认日志
- token/cost 统计

### 文件与报表

- 文件上传
- 文件元数据管理
- 原始文件归档
- 报告文件生成
- 报告预览

## 可选功能

- SSE 实时推送任务进度
- Qdrant 财务制度 RAG
- Prometheus 指标
- OpenTelemetry trace
- CSV 模板下载

## V1 不做

- HR 流程
- 采购流程
- 合同审核流程
- 完整可视化流程编辑器
- 多租户计费
- Java Legacy Adapter
- Kubernetes
- 复杂 BI 图表

但这些业务后续要能通过新增 Business App、Workflow Template、Agent、Tool 和表单 schema 接入。

## 验收标准

- 管理员可以创建用户和角色
- 财务人员可以创建填报任务并上传数据文件
- 系统可以按节点执行 Agent 流程
- 前端可以展示每个节点的执行状态
- AI 生成的分析报告需要人工确认后归档
- 审计日志能追踪是谁在什么时候执行了什么操作
- Agent 执行失败后可以看到错误信息并支持重试
