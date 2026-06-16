# Demo Scenario

## 演示目标

展示一个企业财务人员如何使用多智能体平台完成经营数据填报、财务分析、报表生成、人工确认和归档。

## 准备数据

准备 Excel/CSV，字段示例：

- month
- department
- revenue
- cost
- gross_profit
- operating_expense
- net_profit
- customer_count
- order_count

包含少量异常数据：

- revenue 缺失
- cost 异常偏高
- department 名称不规范

## 演示步骤

1. 管理员登录。
2. 查看财务中心。
3. 创建经营数据填报任务。
4. 上传 Excel/CSV。
5. DataExtractAgent 解析文件。
6. SchemaMappingAgent 映射字段。
7. ValidationAgent 发现缺失值和异常值。
8. 用户确认或修正异常。
9. FinanceAnalysisAgent 生成财务分析摘要。
10. ReportAgent 生成经营分析报告。
11. 财务负责人进入人工确认页。
12. 审核通过。
13. 系统归档报告。
14. 查看审计日志和 Agent 执行日志。

## 演示重点

- 不是聊天机器人，而是流程自动化。
- Agent 输出被状态机承接。
- 高风险结果经过人工确认。
- 每一步都有审计日志。
- 平台可扩展更多业务流程。

## 面试讲述模板

这个项目的核心不是调模型 API，而是把 Agent 接入企业流程。我把系统拆成 Go 平台后端、Python Agent 服务和 React 工作台。Go 负责权限、工作流状态机、Agent/Tool 注册、审计日志和异步任务；Python 负责多 Agent 编排、数据解析、校验、财务分析和报表生成；前端负责展示任务进度、人工确认和报告预览。V1 落地财务经营数据填报，后续可以通过新增 Business App、Workflow Template、Agent 和 Tool 权限扩展到 HR、采购、合同审核等场景。

