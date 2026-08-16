# Project Brief

> 文档状态：Active Project Definition
> 更新日期：2026-08-16
> 北极星路线：[`../05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md`](../05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md)

## 项目名称

Enterprise Agentic Runtime and Workbench（企业级智能体运行与受控行动平台）

## 项目定位

本项目面向企业内部员工、业务负责人和平台治理人员，提供能够长期执行任务、受控调用企业能力、支持人工介入并可追踪评估的 Agentic Runtime 与工作台。

Finance V1 已落地第一条兼容业务主链：

**经营数据填报 / 财务分析 / 报表生成 / 人工确认 / 审计日志**

Finance 是回归基线，不是平台终点。平台底层必须保持业务领域中立，并优先补齐 Durable Run、Tool Execution Gateway、Connector、Context/Skill/Memory、Trace 和 Eval，而不是立即横向复制更多业务 Agent。

## 平台抽象

- Business App：业务入口，例如 finance、hr、procurement、legal、it_service、customer_service。
- Workflow Template：业务流程模板。
- Workflow Instance：一次流程运行实例。
- Workflow Node Instance：流程节点实例。
- Agent Registry：不同领域 Agent 注册中心。
- Tool Registry：企业工具注册与权限控制。
- Agent Runtime Gateway：Go 启动、恢复和取消 Python Agent Run 的入口。
- Approval Task：人工确认任务。
- Audit Log：统一审计。
- Agent Run Log：Agent 执行日志。
- Thread / Run / Step：Agent 长生命周期执行及步骤记录。
- Checkpoint / Interrupt：故障恢复和人工介入边界。
- Skill Version：可版本化的指令、工具、资源和评估包。
- Tool Execution Gateway：所有外部动作的授权、审批、幂等和审计边界。
- Connector：数据库、ERP、工单、知识库等企业系统适配层。
- Eval Run：对任务结果、过程、安全、成本和可靠性的评估。

## 目标用户

- 财务人员
- 财务负责人
- 平台管理员
- 技术/运维人员
- Agent 平台管理员与安全/合规人员

## V1 成功标准

- Docker Compose 可启动完整系统。
- 前端能完成任务创建、文件上传、流程查看、人工确认、报告预览。
- Go 后端能管理用户、权限、工作流、Agent/Tool 注册、审计日志。
- Python Agent 服务能完成解析、校验、分析、报表生成。
- 每个 Agent 调用都有 run_id、trace_id、输入输出摘要、状态、耗时、错误信息。
- 高风险动作必须经过人工确认。
- 新增 HR、采购、合同、IT、客服业务时，不需要重写核心 Workflow Engine、Agent Gateway、Audit Log。

## 平台化边界

V1 不能写成财务专用系统。

财务只是第一个业务模板：

```text
business_app = finance
workflow_template = finance_operating_report
```

通用平台层必须长期复用，后续业务只是在其上新增配置和领域能力。

## 当前成功标准

项目下一阶段的成功不以新增业务 Agent 数量衡量，而以以下能力衡量：

- Agent 执行中进程退出后可从 Checkpoint 恢复。
- 重复消息、Retry 和 Resume 不会重复产生副作用。
- 模型不能绕过 Tool Execution Gateway 访问企业系统。
- 高风险动作绑定确定 Payload 并经过人工审批。
- 每次 Run 可关联 Workflow、模型轮次、Tool Call、审批、外部请求和最终结果。
- 新业务在不修改核心 Runtime 的情况下通过版本化配置、Skill、Tool 和 Connector 接入。
