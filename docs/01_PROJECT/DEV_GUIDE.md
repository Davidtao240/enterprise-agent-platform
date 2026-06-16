# Dev Guide

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

## 模糊点补充

实现前必须明确：

- workflow_templates.definition_json 的结构。
- graph_key 的注册和版本管理方式。
- Agent Registry 中 reusable_scope 的含义。
- Tool Registry 中 risk_level 和 is_shared 的含义。
- Domain Policy 的校验顺序。
- Go Agent Gateway 和 Python Graph Router 的职责边界。

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
