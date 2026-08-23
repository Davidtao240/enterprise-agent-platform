# Roadmap

> 文档状态：Active Roadmap
> 更新日期：2026-08-18
> 目标依据：北极星三层架构（[`ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md` §4.2](../05_FUTURE/ENTERPRISE_AGENTIC_PLATFORM_EVOLUTION.md)）、[M7-M9 产品形态定稿](../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md)

## 使用方式

本文件决定当前实施顺序与每个里程碑**要达到的能力**。详细对象、API 和数据契约以 `02_ARCHITECTURE/`、`03_PLATFORM_SPEC/` 为准；Finance V1 外部行为以 `04_V1_FINANCE/` 和回归 Fixture 为准。

三层架构视角下的能力进度：

| 层 | M1-M6（已完成） | M7-M8（已完成） | M9（当前） |
|---|---|---|---|
| 平台集成层（骨架） | 全部建成 | pgvector 统一向量底座、注册中心安装/卸载语义、混合 Agent 形态 | — |
| 共享能力层（肌肉） | Memory/Skill/Trace/Eval 技术底座 | 对话引擎 v1、知识库产品化 | 多 Agent 编排、LLM Gateway、管理者看板 |
| 可插拔扩展层（血肉） | — | Agent 画廊 + 3 个对话式 Agent、Agent 包、Skill/连接器市场、知识库包 | 市场 UI（安装/评分/用量闭环） |

## 历史阶段：V1 平台骨架（已完成）

### Phase 0–1：项目骨架与身份权限

- Monorepo、Docker Compose。
- Auth、User、Department、Role、Permission、RBAC。

### Phase 2：Workflow Core

- Workflow Template、Instance、Node Instance。
- 状态机、Asynq 异步任务、模板解释执行。
- `graph_key` 显式路由。

### Phase 3：Registry、Gateway 与治理

- Agent、Graph、Tool Registry。
- Agent Gateway、Agent Run Log、Domain Policy。
- Approval、Audit、Configuration Governance。

### Phase 4–6：Finance V1、Workbench 与可观测性

- Finance Agent Graph 和端到端流程。
- 文件、报告、人工审批、归档和审计。
- Token/Cost、平台汇总、演示和质量闸门。

以上阶段是现有能力和兼容基线，不代表最终 Agent Runtime 已完成。

## 过渡阶段：契约与 Shared Core（第一批已完成，待验收）

- Finance Contract / Regression Fixture。
- Shared Core 与 Versioned Finance Profile。
- Python/Go Agent Domain Metadata 对齐。
- Finance Graph 显式绑定 Profile，保持 V1 契约兼容。

本阶段完成不代表 Procurement Phase B 或真实企业集成取得准入。

## M1：Durable Agent Run（已完成）

目标：把一次性 Graph 调用升级为可持久化、可恢复、可取消的 Agent 执行。

交付：

- Thread、Run、Step、Checkpoint、Interrupt 数据模型。
- Run Start、Resume、Cancel、Event 协议。
- Python 持久化 Checkpointer。
- Worker lease、heartbeat、attempt 和失联接管语义。
- Run 绑定不可变的 Graph、Agent、Profile/Skill 和模型配置版本。
- Workflow 状态机与 Agent Run 状态机明确分离和关联。

完成 Gate：

- 执行中终止 Python 后可以从正确 Checkpoint 恢复。
- 执行中终止 Go Worker 后不会重复完成。
- 重复 Resume、Retry 或队列消息不会造成重复副作用。
- 等待审批期间重启服务，审批后仍能继续。
- Finance V1 回归保持通过。

## M2：Tool Execution Gateway（已完成）

目标：所有企业系统访问和副作用都经过可信的 Go 执行边界。

交付：

- Tool Definition Version 和 Tool Execution Contract。
- 调用级身份、Tenant、Domain、资源范围和风险校验。
- 不可变审批 Payload、幂等、超时、重试、熔断和 DLQ。
- CredentialRef 与 Secret 使用边界。
- Tool Call、Policy Decision、Approval、Result、Verify 的 Trace/Audit。

完成 Gate：

- 模型无法伪造身份或绕过网关。
- 未授权和跨 Tenant Tool Call 确定性拒绝。
- 重复调用不产生重复外部副作用。
- 审批内容和实际执行内容一致。

子阶段（M2-A Runtime 桥接、M2-B Tool Call 生命周期、M2-C 超时/熔断/DLQ、M2-D CredentialRef 边界、M2-E 全链路 Trace）均已完成并通过审查。

## M3：Connector Runtime（已完成）

目标：以供应商无关契约接入真实企业系统。

首批能力：

- `enterprise_db_read`：治理后的只读视图。
- `ticket_create_or_update`：幂等、并发控制、审批和状态验证。
- `erp_purchase_request`：Sandbox/Dry-run 优先。

子阶段：

- M3-A：Connector Contract Go 接口、connector_registry 注册与版本治理、Binding 补列（environment/allowed_capabilities/connector_version）、Mock Connector、`enterprise_db_read` 只读、ConnectorRuntime.Execute 接入 ToolCall 执行路径。Gate：版本、能力、认证、健康检查、执行和验证契约可用。（已完成）
- M3-B：`ticket_create_or_update`（幂等键去重、expected_version 乐观锁、closed 终态拒绝）、webhook_events Inbox（HMAC-SHA256 签名、(connector_code, external_event_id) 唯一去重、occurred_at 乱序恢复、失败重试）、WebhookConsumer 周期消费。Gate：Webhook 重复与乱序可恢复。（已完成）
- M3-C：`erp_purchase_request` Sandbox（Dry-run preview、幂等创建、补偿撤销）、connector_outbox（OutboxConnector 契约自声明、指数退避重试、stale Verify 收敛）、Compensation（自动补偿 + 人工治理端点）。Gate：部分成功可恢复，外部请求全链路关联。（已完成）

完成 Gate：

- Connector 具备版本、能力、认证、健康检查、执行和验证契约。
- 限流、超时、Schema Drift、Webhook 重复和部分成功可恢复。
- 外部请求可关联到本地 Run、Step、ToolCall 和 Audit。

## M4：Context、Skill 与 Memory（已完成）

- Context Builder、来源、ACL、裁剪和预算。
- Skill Version 及 Draft/Review/Published/Deprecated 生命周期。
- Run State、Thread Memory、User Memory 和 Team/Domain Memory 分层。
- 写入、读取、过期、删除、敏感数据和跨 Tenant 隔离策略。

## M5：Trace、Eval 与 Replay（已完成）

- Workflow → Run → Model Turn → Tool Call → Checkpoint → Interrupt 层次 Trace。
- Tool 选择、参数、副作用、权限、成本和业务结果 Eval。
- 失败重放、Regression、Shadow、Canary 和人工修改反馈。

## M6：Enterprise Workbench 与受控路由（已完成）

- 统一目标入口、授权能力发现和 Run 时间线。
- 展示等待原因、审批 Payload、Tool Call、外部请求和恢复记录。
- Router 只能在用户授权的 Business App、Agent、Skill 和 Tool 范围内选择。
- 不建设拥有全部权限的“万能主 Agent”。

## M7：对话式骨架（已完成）

> 设计依据：[`../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md`](../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md)、北极星 §4.2 三层架构
> 定位：让产品"像个 Agent 产品"——补齐共享能力层的对话引擎，改造员工第一屏。
> 达成能力：员工可从 Agent 画廊选择 Agent 并以对话方式使用（流式输出、澄清追问、多轮上下文）；向量底座统一为 pgvector；产品范式从"表单驱动"切换为"对话优先"。

目标：从"表单驱动工作流"转为"对话式 Agent 工作台"，同时完成向量库统一。

子任务：

- M7-A 对话引擎 v1（共享层）：SSE 流式、多轮会话持久化、澄清追问、会话历史。Gate：打字机效果可用；会话刷新不丢失；Agent 可追问。
- M7-B Agent 画廊 + 聊天 UI（体验层）：首页改造为 Agent 市场入口 + 聊天组件。Gate：员工第一屏看到可选择的 Agent 卡片，按通用/部门分类。
- M7-C 3 个对话式 Agent（业务层）：财务报告对话式改造 + 文档总结 + 会议纪要。Gate：均可对话使用，输出结构化可追溯。
- M7-D pgvector 迁移（平台层）：向量表迁移 + Qdrant 退役 + 租户隔离。Gate：Qdrant 容器移除，检索走 PG，租户隔离测试通过。

完成 Gate：

- 对话成为新功能的第一入口，表单流程仅作兜底。
- Finance V1 回归保持通过（财务报告对话式改造不破坏既有契约）。

## M8：可插拔机制（已完成）

> 定位：让业务"加得快"——新 Agent/连接器/Skill 安装即用，平台零代码改动。
> 达成能力：新增部门/通用 Agent 通过安装 Agent 包接入，无需修改 Go 平台代码或重启服务；Skill 与连接器具备市场化的安装/版本/审核语义；知识库可用并带引用溯源——可插拔扩展层（血肉）成形。

子任务：

- M8-A Agent 包动态加载（平台层）：官方 Agent package 规范 + 安装即用；第三方注册协议 v1。Gate：新 Agent 包安装无需改平台代码或重启。
- M8-B Skill 市场 v1（可插拔层）：安装/卸载/版本/审核流 UI 化。Gate：Skill 全生命周期在 UI 完成。
- M8-C Connector Protocol（可插拔层）：HTTP/gRPC 契约（execute/verify/compensate/health）+ sidecar 注册接入 + 1 个示例 sidecar。Gate：第三方 sidecar 注册后可被 Tool 调用。
- M8-D 知识库 v1（共享层）：文档上传 → pgvector → 检索 → 回答带引用溯源。Gate：回答附带"来自文档 X 第 Y 节"引用。

完成 Gate：

- 新增部门 Agent 不修改 Go `internal/` 平台代码（业务中立原则延续）。
- 可插拔层组件启用/禁用全部在 UI 完成，禁止改代码或重新部署。

## M9：多 Agent 协作 + 运营化（当前主线）

> 定位：让体验"上一个台阶"——协作编排、成本可见、市场闭环、管理者看板。
> 达成能力：跨 Agent 协作场景（supervisor 分发→协作→汇总）跑通；LLM 成本按部门/Agent 计量、预算可熔断；Agent/连接器市场完成安装→使用→评分闭环；管理者可通过看板回答"这个 Agent 帮部门省了多少时间、质量如何"——共享能力层（肌肉）完备。

子任务：

- M9-A 多 Agent 编排（共享层）：LangGraph supervisor 模式，任务分发→协作→汇总。Gate：1 个跨 Agent 协作场景跑通（如财务报告 = 采集 + 分析 + 合规三 Agent）。
- M9-B LLM Gateway（共享层）：自建轻量模块，多模型路由 + token 计量 + 预算控制 + prefix cache。Gate：按部门/Agent 维度 token 成本可见，预算超限熔断。
- M9-C 市场 UI（可插拔层）：Agent 市场 + 连接器市场（安装/评分/用量）。Gate：市场页面完成安装→使用闭环。
- M9-D 管理者看板（体验层）：部门效率指标、Agent 质量评分、Top 失败原因。Gate：管理者可回答"这个 Agent 帮部门省了多少时间"。

## 完整业务场景扩展（M9 之后）

只有 M7-M8 平台 Gate 通过后，才按顺序进入完整 Procurement、HR、Legal、IT Service 或 Customer Service 场景。

每个新场景必须：

- 复用 Workflow Engine、Durable Runtime、Tool Gateway、Connector、Approval、Audit 和 Eval。
- 不在 Go 平台核心增加业务条件分支。
- 提供版本化 Domain Profile/Skill、契约 Fixture 和独立预期结果。
- 先 Mock/Sandbox/Read-only，再 Shadow、人工审批写入和 Limited Canary。

## 当前明确不做

- 无 Durable Run 的长期任务。
- 无 Tool Gateway 的真实数据库、ERP 或工单写入。
- 任意 SQL、HTTP、Shell、Filesystem 或全权限 Tool。
- 无 Eval 目标的复杂多 Agent 扩张。
- 大规模前端视觉重构。
- 外部 LLM 网关（LiteLLM 等）与 Langfuse 对接（自建 Trace/Eval 与 LLM Gateway 已覆盖）。
