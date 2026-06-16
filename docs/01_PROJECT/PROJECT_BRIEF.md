# Project Brief

## 项目名称

企业级多智能体流程自动化平台

## 项目定位

本项目面向企业内部员工，提供可扩展的多智能体流程自动化工作台。V1 只落地一个业务场景：

**经营数据填报 / 财务分析 / 报表生成 / 人工确认 / 审计日志**

但平台底层必须按多业务场景设计，不能做成财务专用系统。

## 平台抽象

- Business App：业务入口，例如 finance、hr、procurement、legal、it_service、customer_service。
- Workflow Template：业务流程模板。
- Workflow Instance：一次流程运行实例。
- Workflow Node Instance：流程节点实例。
- Agent Registry：不同领域 Agent 注册中心。
- Tool Registry：企业工具注册与权限控制。
- Agent Gateway：统一调用 Agent 的网关。
- Approval Task：人工确认任务。
- Audit Log：统一审计。
- Agent Run Log：Agent 执行日志。

## 目标用户

- 财务人员
- 财务负责人
- 平台管理员
- 技术/运维人员

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

