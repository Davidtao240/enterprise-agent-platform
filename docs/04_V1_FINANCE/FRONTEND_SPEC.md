# Frontend Spec

## 页面列表

### Dashboard

- 展示业务入口
- V1 只展示财务中心
- 后续可展示 HR、采购、法务、IT 服务

Dashboard 必须从 Business App API 获取可用业务入口，V1 只是返回 finance，不要把业务卡片写死成财务专用组件。

### Finance Home

- 经营数据填报入口
- 最近任务
- 待审核任务
- 已归档报告

Finance Home 是 V1 的业务定制页，但内部应复用通用 Workflow Instance、Workflow Detail、Audit Log、Agent Run Log 组件。

### Workflow Detail

- 展示流程节点
- 展示当前状态
- 展示每个 Agent 的输入输出摘要
- 展示失败原因
- 支持重试失败节点

### Human Review

- 展示最终报告
- 展示 Agent 分析摘要
- 展示风险提示
- 审核通过
- 审核驳回
- 填写审核意见

## 前端扩展原则

通用页面：

- Dashboard
- Workflow Template List
- Workflow Instance List
- Workflow Detail
- Workflow Node Timeline
- Agent Run Log
- Audit Log
- Approval Task

业务定制页：

- Finance Report Preview
- HR Candidate Review
- Procurement Quote Compare
- Contract Risk Report
- IT Incident Detail
- Customer Ticket Detail

新增业务时优先复用通用页面，只在表单和结果展示处做业务定制。

