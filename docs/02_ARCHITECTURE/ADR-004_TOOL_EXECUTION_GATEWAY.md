# ADR-004: Tool Execution Gateway

> 状态：Accepted
> 日期：2026-08-16

## 决策

所有企业数据访问和外部副作用由 Go Tool Execution Gateway 执行。Python/模型只提交结构化 Tool Request；可信身份、Policy、Credential 和 Connector Binding 由 Go 从 Run Snapshot 获取。

## 与 Agent Gateway 的区别

- Agent Runtime Gateway：Go → Python，启动和控制 Run。
- Tool Execution Gateway：Python → Go → Connector，授权并执行 Tool Call。

## 后果

Tool Registry 不再被误认为执行边界。写操作必须定义幂等、审批、Verify；状态不确定时先 Reconcile，不能盲目重试。
