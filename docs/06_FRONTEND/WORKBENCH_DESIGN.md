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

| 路由 | 页面名称 | 描述 | 依赖后端 |
|---|---|---|---|
| `/workbench` | **工作台首页** | 系统总览：等待审批、进行中 Run、最近完成、系统指标 (M5) | `approvals`, `agent_runs`, `eval` |
| `/run/:id` | **Run 时间线详情页** | 核心页面！展示单个 Agent Run 的完整生命周期时间线 (基于 M5 Trace) | `trace_events`, `agent_runs` |
| `/operations/approvals` | **审批中心** | 待处理的 ToolCall 审批任务列表，支持确认/拒绝 | `tool_calls` (status: awaiting_approval) |
| `/operations/outbox` | **Outbox 监控** | 可靠消息投递的状态监控，支持人工补偿 | `connector_outbox` |
| `/operations/dlq` | **死信队列** | 失败重试耗尽的消息列表，支持人工干预 | `connector_outbox` (is_dead_letter=true) |
| `/explore/tool-calls` | **Tool Call 探索器** | 所有工具调用的全局搜索和过滤器 | `tool_calls` |
| `/explore/traces` | **Trace 探索器** | 基于 Trace ID 的全链路搜索入口 | `trace_events` |
| `/settings/rbac` | **权限管理** (原 RBAC 页面) | 用户、角色、权限的配置 | `users`, `roles`, `permissions` |
| `/settings/connectors` | **连接器管理** | Connector Registry 的可视化配置和版本管理 (M3-A) | `connector_registry`, `connector_bindings` |
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

*   **M6-A**: 核心组件开发 (`Timeline`, `EventNode`, `PayloadViewer`) + Run 详情页 MVP。
*   **M6-B**: 工作台首页 + 审批中心 + Outbox 监控 + DLQ 页面 (治理功能)。
*   **M6-C**: Trace 探索器 + 连接器管理 + 全局搜索优化。