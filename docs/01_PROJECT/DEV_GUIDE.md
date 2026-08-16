# Dev Guide

> 文档状态：Active Development Rules
> 更新日期：2026-08-16

## 开发原则

- 不把 Agent 逻辑写进 Go 后端。
- 不让前端直接调用 Python Agent 服务。
- 不让 Agent 绕过 Go 后端直接写业务数据库。
- 所有状态变化都由 Go 后端持久化。
- 所有关键动作都写审计日志。
- 所有 Agent 输出都做结构化校验。
- 不把 V1 写成财务专用系统。
- 不在 Workflow Engine 里写业务 if else。
- 新增业务场景必须通过 Business App、Workflow Template、Agent Registry、Tool Registry 接入。
- 通用页面和通用后端服务优先，业务定制只放在表单、结果页和领域 Agent 中。
- Workflow Template 必须通过 graph_key 显式路由 Python Agent Graph。
- Graph 按流程隔离，Agent 按能力复用，Tool 按权限隔离。
- shared Agent 必须受 Domain Policy 约束，不能因为复用而获得跨域工具权限。
- Tool Registry 只是定义目录，不是执行或授权边界；企业系统调用必须经过 Go Tool Execution Gateway。
- 模型、Python Agent 和 Skill 不得直接持有企业管理员凭证，也不得直接写数据库、ERP 或工单。
- Workflow 管企业业务流程，Agent Run 管智能体内部执行；不得用 Agent Run Log 代替 Durable Run 状态。
- Run 必须绑定不可变的 Graph、Agent、Profile/Skill、Tool、Policy 和模型配置版本。
- Checkpoint 恢复和 Retry 前必须核对已完成 Tool Call，防止重复副作用。
- 高风险审批必须绑定确定 Payload、版本、风险和有效期，执行前重新校验权限与业务状态。
- 外部文档、数据库字段、工单评论和网页内容一律视为不可信数据，不能改变系统 Policy。
- Memory、Skill 和共享代码不能扩大用户、Tenant、Agent 和 Tool 原有权限。

## 模糊点补充

实现前必须明确：

- workflow_templates.definition_json 的结构。
- graph_key 的注册和版本管理方式。
- Agent Registry 中 reusable_scope 的含义。
- Tool Registry 中 risk_level 和 is_shared 的含义。
- Domain Policy 的校验顺序。
- Go Agent Gateway 和 Python Graph Router 的职责边界。
- Go Control Plane 与 Python Checkpointer 的状态权威边界。
- Agent Runtime Gateway 与 Tool Execution Gateway 的职责边界。
- Tool、Connector、CredentialRef 和外部系统的信任边界。
- Thread、Run、Step、Checkpoint、Interrupt 与 Workflow/Node 的关联关系。

## 命名约束

平台核心模块避免使用 finance 前缀。

推荐：

```text
workflow
agent
tool
approval
audit
business_app
```

财务只出现在：

```text
finance workflow template
finance domain agent
finance report view
sample finance data
```
