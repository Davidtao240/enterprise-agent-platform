# Enterprise Workbench 前端设计 (M6)

> 文档状态：Design Specification
> 更新日期：2026-08-18
> 目标：构建统一的企业工作台，串联所有后端能力，提供优秀的用户体验和运维效率。

## 1. 设计原则

1.  **一切围绕 Run**: 以 Agent Run 为核心，所有页面的设计都应能快速跳转到 Run 详情页查看完整的生命周期。
2.  **状态可视化**: Agent 的“暂停”、“等待”、“补偿”等状态必须清晰地展示给用户，并提供明确的操作指引。
3.  **权限驱动**: 所有 UI 元素（菜单、按钮、数据）严格根据用户角色和权限动态渲染，避免越权操作。
4.  **响应式与易用**: 适配桌面端主要使用场景，强调信息密度和操作效率。

## 2. 页面规划

M6 将重构现有页面并新增多个核心页面。

### 2.1. 核心页面列表

> 注: 本表为 M6 初期规划;最终实现以 §6.4(前置修订版路由契约)为准。
> 差异说明: `/operations/approvals` 的待审批列表能力并入 Tool Call 探索器
> (`status=pending_approval` 过滤 + 工作台待办卡片);`/explore/traces` 的
> Trace 查看能力并入 Run 时间线详情页(经 `trace_id` 自动关联拉取 L1-L6);
> `/operations/dlq` 并入 `/operations/outbox` 的 DLQ Tab。

| 路由 | 页面名称 | 描述 | 依赖后端 |
|---|---|---|---|
| `/` | **工作台首页** | 系统总览：等待审批、进行中 Run、今日 Eval 概览、可靠性指标 (M5) | `approvals`, `agent_runs`, `eval` |
| `/runs/:id` | **Run 时间线详情页** | 核心页面！展示单个 Agent Run 的完整生命周期时间线 (基于 M5 Trace) | `trace_events`, `agent_runs` |
| `/operations/outbox` | **Outbox 监控 (含 DLQ Tab)** | 可靠消息投递的状态监控，支持人工补偿;DLQ Tab 展示死信 Tool Call | `connector_outbox`, `tool_calls` |
| `/explore/tool-calls` | **Tool Call 探索器** | 所有工具调用的全局搜索和过滤器(含待审批过滤) | `tool_calls` |
| `/experiments` | **实验中心** | Canary/Shadow/Replay 受控路由管理 (M5-C) | `replays`, `shadow_rules`, `canary_releases` |
| `/settings/rbac` | **权限管理** (原 RBAC 页面) | 用户、角色、权限的配置 | `users`, `roles`, `permissions` |
| `/settings/connectors` | **连接器管理** | Connector Registry 的可视化配置和授权范围管理 (M3-A) | `connector_registry`, `connector_bindings` |
| `/business/:app_code` | **业务应用中心** | 动态生成的业务应用主页（如 `/business/finance`） | `business_apps` |

### 2.2. 组件规划

为了提高复用性，设计以下核心组件：

*   **`Timeline`**: 用于 Run 详情页的垂直时间线组件，支持多种节点类型。
*   **`EventNode`**: 时间线上的事件节点，根据事件类型（Start/End/Error/Pause）有不同的样式。
*   **`PayloadViewer`**: 用于查看 JSON 格式的事件详情、工具参数、LLM 输入输出。
*   **`StatusBadge`**: 状态徽章组件，统一展示各种状态（Pending, Running, Success, Failed, Waiting）。
*   **`ActionPanel`**: 当 Run 处于 Waiting 状态时展示的操作面板，包含审批按钮、补偿按钮等。

## 3. 核心页面设计

### 3.1. Run 时间线详情页 (`/run/:id`)

这是 M6 中最重要的页面，用于深度诊断和理解 Agent 的行为。

**布局**:
*   **左侧面板 (Main)**:
    *   **顶部**: Run 的基本信息 (ID、状态、开始时间、耗时、调用的 Agent/Skill)。
    *   **中部**: **垂直时间线**。
        *   每个 Trace Event (`trace_events`) 是一个节点。
        *   **L1-Workflow**: 显示顶层业务流程节点。
        *   **L2-Agent Run**: 当前 Run 的起止节点。
        *   **L3-Model Turn**: 折叠的对话轮次，点击可展开查看 Prompt 和 Output。
        *   **L4-Tool Call**: 显示工具调用，成功为绿色，失败为红色，并高亮其影响范围。
        *   **L5-Checkpoint**: 显示断点保存。
        *   **L6-Interrupt**: 显示打断点，**非常醒目**。
    *   **底部**: 快速操作按钮（如"重放"、"取消"）。

*   **右侧面板 (Sidebar)**:
    *   **Payload Inspector**: 点击时间线上的任意节点，此处显示该事件的详细 JSON 数据。
    *   **关联信息**: 显示与该 Run 关联的其他对象 (Workflow ID, User ID, Workflow Instance ID)。

**状态处理**:
*   **Running**: 页面自动滚动并轮询刷新，显示实时进度。
*   **Waiting for Approval**: 时间线暂停在 L6-Interrupt 节点，右侧显示 `ActionPanel`，提供 "Approve" 和 "Reject" 按钮。
*   **Compensation Pending**: 显示 Outbox 补偿的进度。

### 3.2. 工作台首页 (`/workbench`)

作为用户的默认入口，提供关键信息的聚合视图。

**布局**:
*   **顶栏 (Header)**:
    *   全局搜索框 (支持按 Trace ID, Run ID, User ID 搜索)。
    *   用户信息和通知中心。

*   **内容区 (Dashboard)**:
    *   **第一行**:
        *   **[卡片] 待办审批**: 显示当前用户的待处理 ToolCall 审批任务，提供快速处理入口。
        *   **[卡片] 系统健康**: 显示核心服务的状态 (Go Backend, Agent Service, M3 Connectors)。
    *   **第二行**:
        *   **[卡片] 进行中 Run**: 实时状态的 Agent Run 列表，高亮显示耗时较长的 Run。
        *   **[卡片] 今日概览**: M5 生成的今日评估摘要 (Total Runs, Avg Duration, Success Rate, Cost)。
    *   **第三行**:
        *   **[卡片] 可靠性**: Outbox 投递状态、DLQ 数量、熔断状态等核心稳定性指标。

## 4. 技术选型与实现

*   **框架**: React 18 + TypeScript
*   **UI 组件库**: Ant Design 5.x
*   **状态管理**: Zustand (全局状态) + 组件内 useState/useReducer
*   **API 通信**: Axios，封装在 `src/services/api.ts` 中
*   **样式**: CSS Modules 或 Tailwind CSS (待定)
*   **路由**: React Router v6

## 5. 开发阶段规划

*   **M6-A**: 核心组件开发 (`Timeline`, `EventNode`, `PayloadViewer`) + Run 详情页 MVP + 工作台首页改造。
*   **M6-B**: Tool Call 探索器 + Outbox 监控 + DLQ 页面 (可靠性运维)。
*   **M6-C**: 受控路由可视化 (Canary/Shadow/Replay) + 连接器授权范围可视化 + Trace 探索器。

## 6. M6 API 契约与权限映射 (Spec 前置修订)

现有 `tool_calls`/`connector_outbox`/`connector_registry` 端点位于 `/internal/*`
(InternalServiceToken 保护,供服务间调用),前端不可达。M6 新增以下
**protected 端点**(JWT 认证 + `tenant_id` 来自认证上下文,严格租户隔离),
复用既有 Repository/Service,新增 tenant-scoped 查询方法。

### 6.1. Run 查询 (M6-A)

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/runs?status=&limit=` | `workflow:read` | Run 列表(按 `updated_at` 倒序,可按状态过滤) |
| GET | `/api/v1/runs/:id` | `workflow:read` | Run 详情(基本信息 + steps + runtime events) |

Run 详情页时间线数据 = `GET /runs/:id`(Run/Step/Event) + `GET /traces/:trace_id`(L1-L6 Trace 事件),经 `trace_id` 关联。

### 6.2. 可靠性运维 (M6-B, migration 029 新增权限)

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/ops/tool-calls?status=&tool_id=&limit=` | `tool:read` | Tool Call 探索器(分页/过滤) |
| GET | `/api/v1/ops/tool-calls/:id` | `tool:read` | Tool Call 详情 |
| GET | `/api/v1/ops/tool-calls/dead-letters` | `tool:read` | DLQ(`is_dead_letter=true`) |
| GET | `/api/v1/ops/outbox?state=&limit=` | `outbox:read` | Outbox 监控列表 |
| GET | `/api/v1/ops/outbox/:id` | `outbox:read` | Outbox 单条详情 |
| POST | `/api/v1/ops/outbox/:id/compensate` | `outbox:read` | 人工触发补偿(写操作,运维 Owner 自理;与 internal 端点同语义) |

新增权限点(migration 029,授予 `platform_admin`): `tool:read`(Tool Calls read)、
`outbox:read`(Outbox governance)。Compensate 复用 `outbox:read` 以简化运维角色
(平台管理员限定,审计日志兜底)。

### 6.3. 受控路由与授权范围 (M6-C)

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/connector-registry` | `tool:manage` | Connector 注册表(授权范围可视化:code/版本/能力/outbox 声明) |
| GET | `/api/v1/connector-bindings` | `tool:manage` | Connector Binding 列表(工具 ↔ 连接器授权关系) |
| GET | `/api/v1/domain-policies` | `business_app:read` | (既有) 域策略,受控路由的域约束可视化 |

实验管理(Canary/Shadow/Replay)直接消费 M5-C 契约(TRACE_AND_EVAL.md §4.5,
权限 `experiment:manage`),不新增端点。

### 6.4. 前端路由与菜单

| 路由 | 页面组件 | 权限守卫 |
|---|---|---|
| `/runs/:id` | `RunDetailPage` | `workflow:read` |
| `/explore/tool-calls` | `ToolCallExplorerPage` | `tool:read` |
| `/operations/outbox` | `OpsOutboxPage`(含 DLQ Tab) | `outbox:read` |
| `/experiments` | `ExperimentsPage`(Canary/Shadow/Replay) | `experiment:manage` |
| `/settings/connectors` | `ConnectorScopePage` | `tool:manage` |
| `/` (改造) | `DashboardPage`(审批/Run/Eval/可靠性四卡片) | 登录即可,卡片按权限渲染 |

## 7. M7 对话式骨架改造

> 依据 [AGENTIC_WORKBENCH_M7_M9_DESIGN.md](../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md)：M7 起新功能优先对话式入口，表单流程仅作兜底。API 契约见 [CONVERSATION_ENGINE.md](../03_PLATFORM_SPEC/CONVERSATION_ENGINE.md)、[AGENT_GALLERY.md](../03_PLATFORM_SPEC/AGENT_GALLERY.md)。

### 7.1 页面与路由变化

| 路由 | 页面 | 说明 |
|---|---|---|
| `/`（改造） | `AgentGalleryPage` | **员工第一屏**：Agent 卡片网格 + 分类 Tab（全部/通用/各部门）+ 搜索 |
| `/dashboard`（迁移） | `DashboardPage` | 原 M6 四卡片总览迁移至此，画廊页顶栏保留入口（管理/运维视角） |
| `/conversations/:id` | `ConversationPage` | 对话窗口（核心新增页面） |
| 其余 M6 路由 | 不变 | Run 详情/运维/实验/设置保持 |

### 7.2 新增组件（全平台复用）

| 组件 | 职责 |
|---|---|
| `AgentCard` / `AgentGrid` | 画廊卡片与网格；点击创建会话并跳转 |
| `ChatWindow` | 对话容器：消息流 + 输入区 + 会话侧栏 |
| `MessageList` / `MessageBubble` | 消息流；Markdown 渲染（DOMPurify 转义） |
| `StreamingIndicator` | 打字机光标与流式状态 |
| `ClarificationCard` | 澄清追问卡片，按 `resume_schema_json` 渲染动态表单 |
| `ApprovalCard` | 审批卡片（复用既有 Approval 操作端点，展示 Agent 决策依据） |
| `CitationCard` | 引用溯源卡片（M8 知识库接入） |
| `ToolProgress` | 工具调用进度（tool.started/completed 事件驱动） |

技术要点：EventSource 封装（`src/services/sse.ts`，处理重连与 Last-Event-Id）；打字机 = `message.delta` 增量追加；不引入 `@ant-design/x`（自建，见设计文档 §4.3）。

### 7.3 交互要点

- 发送消息 → `202 {message_id, run_id}` → 订阅 `/stream` SSE → delta 增量渲染。
- `clarification.requested` → 渲染澄清卡片，输入区禁用自由文本、仅接受表单提交（`POST /answers`）。
- `approval.requested` → 渲染审批卡片；决策走既有审批端点，完成后收 `run.completed` 继续。
- 断线：EventSource 自动重连续传；重连失败回退轮询 `GET /conversations/:id` 消息终态。
- 会话侧栏：**按 Agent 分组 + 组内时间倒序**（产品决策 2026-08-18：M7 仅 3 个 Agent，按时间分组意义小；数据来自 `GET /conversations?group_by=agent_package`，契约见 `CONVERSATION_ENGINE.md` §3.1）+ 新建会话入口。
- 并发保护：会话存在活跃 Run 时输入区置灰并显示"生成中"（后端同时返回 `409` 兜底）。

### 7.4 权限

- `conversation:read` / `conversation:write`：全部默认角色（员工基本能力）。
- 画廊浏览：`business_app:read`；包管理：`agent:manage`。
- `/dashboard` 及 M6 运维页权限守卫不变。

### 7.5 M7 三个对话式 Agent：卡片文案、示例 Prompt 与演示剧本（定稿）

#### 7.5.1 画廊卡片文案

| | 财务经营报告助手 | 文档总结助手 | 会议纪要助手 |
|---|---|---|---|
| package_code | `finance_operating_report_assistant` | `document_summary_assistant` | `meeting_minutes_assistant` |
| category | departmental（finance） | general（productivity） | general（productivity） |
| icon | `finance-chart` | `document-text` | `calendar-note` |
| 一句话描述 | 一句话生成部门经营报告：自动完成数据校验、波动分析与归因初判，超阈值结论自动走审批。 | 上传长文档秒级提炼：摘要、核心结论与行动项，可继续追问细节。 | 粘贴转录或上传会议稿，自动生成结构化纪要：决议、行动项、负责人与截止时间。 |
| 卡片角标 | 部门·财务 | 通用 | 通用 |

#### 7.5.2 示例 Prompt（sample_prompts_json，每包 3 条）

**财务经营报告助手**：

1. 「帮我生成 2026 Q3 华东销售部的经营报告，重点看费用异常」
2. 「对比 2026 Q2 与 Q3 的毛利率变化，找出波动最大的三个科目」
3. 「把上次报告的口径换成事业部维度，重新出一版」

**文档总结助手**：

1. 「总结这份季度财务分析报告的五个核心结论」
2. 「这份合同里对我方不利的条款有哪些？」
3. 「用三句话向管理层汇报这份调研报告的要点」

**会议纪要助手**：

1. 「根据这段转录生成纪要，列出所有行动项和负责人」
2. 「把上次会议的待办和这次对齐，哪些已经完成了？」
3. 「给这次评审会生成一封进展同步邮件草稿」

#### 7.5.3 演示剧本（3 组，M7 验收演示与测试数据依据）

**剧本 A：财务报告助手（澄清 → 工具 → 审批 → 完成，全链路）**

```text
1. 用户: "帮我出 Q3 华东销售部的经营报告"
2. Agent (message.delta): "好的，生成经营报告前请确认以下信息——"
3. Agent (clarification.requested): schema {period: select(2026-Q1~Q3), department: text(必填),
   compare_last_year: checkbox(默认 true), threshold: number(默认 10%)}
4. 用户提交 answers: {period: "2026-Q3", department: "华东销售部", compare_last_year: true, threshold: 30}
5. Agent (tool.started): 读取财务数据 → tool.completed: 12 科目 36 条记录
6. Agent (message.delta): 流式输出报告（费用环比 +35% 超过 30% 阈值，触发预警结论）
7. Agent (approval.requested): "销售费用环比 +35% 超过您设定的 30% 阈值，
   预警结论需财务经理确认后写入报告" → 会话 waiting_approval
8. finance_manager 在审批卡片点击「批准」（Approval API）
9. Agent: 报告收尾 + message.completed + run.completed → 会话回到 active
验收点: 澄清表单渲染 select/checkbox/number 三种控件；侧栏新会话出现在
"财务经营报告助手"分组；Run 详情时间线含 tool call 与审批节点。
```

**剧本 B：文档总结助手（附件 + 多轮追问）**

```text
1. 用户: 上传 report.pdf（附件）+ "总结这份报告的五个核心结论"
2. Agent (message.delta): 流式输出 5 条结论（每条标注章节来源占位，M8 接 pgvector 溯源）
3. 用户: "第 3 条展开讲讲，和去年相比呢？"
4. Agent: 基于 thread history 上下文直接回答（不重新澄清），展示多轮记忆
验收点: 附件上传走文件 API；追问不触发新澄清；两轮消息在同一会话。
```

**剧本 C：会议纪要助手（转录粘贴 + 行动项澄清）**

```text
1. 用户: 粘贴 20 分钟会议转录文本 + "生成纪要"
2. Agent (message.delta): 输出结构化纪要（议题/决议/行动项 4 条）
3. Agent (clarification.requested): "行动项 3 未识别到负责人" 
   schema {owner: select(张三/李四/王五), due_date: date}
4. 用户提交: {owner: "王五", due_date: "2026-08-25"}
5. Agent: 生成终版纪要 + 邮件草稿 + run.completed
验收点: select + date 控件组合；澄清仅针对缺失字段（≤2 个，不重复已识别信息）。
```

#### 7.5.4 管理员演示（上下架）

```text
1. admin 登录 → 画廊管理入口（agent:manage）→ 看到包列表（3 个 published）
2. 「文档总结助手」→ 下架 → 画廊员工视图隐藏该卡片、API 返回列表不含
3. 重新上架 → 恢复；全程 audit_logs 可查（agent_package.disable / publish）
验收点: 存量会话在下架期间只读不删；usage_count 仍显示真实历史统计。
```
