# Conversation Engine

> 文档状态：Active Specification (M7-A Target)
> 更新日期：2026-08-18
> 相关：[ADR-007](../02_ARCHITECTURE/ADR-007_CONVERSATION_ENGINE_SSE.md)、[DATABASE_SCHEMA.md](DATABASE_SCHEMA.md)（M7 Target 表）、[AGENT_GALLERY.md](AGENT_GALLERY.md)、[MEMORY_AND_CONTEXT.md](MEMORY_AND_CONTEXT.md)

## 1. 职责与边界

**负责**：Conversation/Message 持久化、SSE 流式转发、澄清追问、会话历史、会话级 Token 预算。

**不负责**（全部复用既有组件）：

| 能力 | 复用组件 |
|---|---|
| Graph 执行与状态机 | Durable Run（M1） |
| Tool 授权/审批/执行 | Tool Execution Gateway（M2/M3） |
| 审批决策 | Approval API |
| 上下文组装 | Context Builder（M4，含 Memory 聚合与 Token 裁剪） |
| 链路追踪 | 六层 Trace（M5） |

Go 是会话与消息的 System of Record；Python 是流式事件生产者。

## 2. 数据模型

权威表定义见 [DATABASE_SCHEMA.md](DATABASE_SCHEMA.md) M7 Target。要点：

- `conversations.thread_id` 外键关联 `agent_threads`：对话是 thread 的用户态包装，Run 仍挂在 thread 上，索引与租户隔离沿用。
- `conversation_messages.run_id` 关联 `agent_runs`：每条 assistant 消息可溯源到 Durable Run。
- SSE 流事件不落新表：由 Runtime Events 派生转发；`conversation_messages` 只保存消息终态。

## 3. API

权限点：`conversation:read` / `conversation:write`（M7 migration 新增，授予全部四个默认角色——对话是员工基本能力）。

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/v1/conversations` | `conversation:write` | 创建会话 `{agent_package_code, title?}`；解析 package → graph_key，校验租户与 Domain Policy |
| GET | `/api/v1/conversations?limit=&status=&group_by=agent_package` | `conversation:read` | 我的会话列表（仅本人）。`group_by=agent_package` 时按 Agent 包分组返回，组内 `last_message_at` 倒序；默认 limit=50、上限 200 |
| GET | `/api/v1/conversations/:id` | `conversation:read` | 会话详情 + 消息（分页倒序，`before_message_id` 游标；默认 limit=50、上限 200） |
| PATCH | `/api/v1/conversations/:id` | `conversation:write` | 重命名 / 关闭会话 |
| POST | `/api/v1/conversations/:id/messages` | `conversation:write` | 发送用户消息 `{content, attachments?}`；同一会话同时只允许一个活跃 Run |
| POST | `/api/v1/conversations/:id/answers` | `conversation:write` | 回答澄清 `{interrupt_id, answer}`；经 `resume_schema_json` 校验后 Resume |
| POST | `/api/v1/conversations/:id/cancel` | `conversation:write` | 取消当前 Run |
| GET | `/api/v1/conversations/:id/stream` | `conversation:read` | SSE 事件流（`text/event-stream`） |

发送消息返回 `202 {message_id, run_id}`；实际输出通过 SSE 流接收。POST 语义是"受理"，不是"完成"。

### 3.1 会话侧栏分组契约（产品决策 2026-08-18）

- **M7 采用"按 Agent 分组 + 组内时间倒序"**：仅 3 个 Agent，按时间分组意义小；员工心智是"我在和哪个 Agent 对话"。
- `GET /conversations?group_by=agent_package` 返回结构：

```json
{
  "groups": [
    {
      "agent_package_code": "finance_operating_report_assistant",
      "agent_package_name": "财务经营报告助手",
      "icon": "finance",
      "conversations": [
        {"id": "...", "title": "Q3 华东经营报告", "status": "active", "last_message_at": "2026-08-18T10:00:00Z"}
      ]
    }
  ]
}
```

- 组间排序：组内最近一条消息时间倒序（最活跃的 Agent 组在最上）。
- 分组在前端渲染，数据来自该端点一次拉取；M7 不做分页加载更多（50 条上限内），Agent 数量或会话量增长后（M9）再评估时间分组切换或虚拟滚动。

## 4. SSE 事件契约

```text
id: <conversation 内单调递增 seq>
event: <事件类型>
data: <JSON payload>
```

| event | payload 要点 | 说明 |
|---|---|---|
| `conversation.started` | conversation_id, graph_key | 流建立 |
| `run.started` | run_id | Durable Run 启动 |
| `message.delta` | seq, delta | assistant 增量文本（打字机） |
| `message.completed` | message_id, content, tokens | 消息终态 |
| `clarification.requested` | interrupt_id, schema, question | 澄清卡片（Interrupt input_required） |
| `approval.requested` | approval_task_id, summary, evidence | 审批卡片；决策走既有 Approval API |
| `tool.started` / `tool.completed` | tool_call_id, tool_id | 工具进度条 |
| `run.completed` / `run.failed` | run_id, status, error | Run 终态 |
| `done` | — | 本轮结束 |
| `error` | code, message | 通道级错误（不中止会话） |

规则：

- 心跳：每 15s 发送 `:ping` comment，防代理空闲断连。
- 断线续传：EventSource 自动重连带 `Last-Event-Id`；服务端按 seq 续发缺失事件（保留窗口默认 1000 条/会话）。
- 重连失败兜底：前端回退轮询 `GET /conversations/:id` 消息终态。
- SSE 通道只读：任何写操作（发消息、回答澄清、审批）走对应 REST 端点。

## 5. 澄清与多轮语义

- Graph 判定信息不足 → `Interrupt(kind=input_required)` → 会话置 `waiting_input` → 前端渲染 `ClarificationCard`（按 `resume_schema_json` 动态表单）。
- 用户提交 `/answers` → Go 校验 payload → Resume Run → 会话回 `active`。
- 多轮上下文：Context Builder 聚合 `conversation_messages` 近 N 条（默认 20）作为 thread history；Token 预算沿用既有裁剪优先级（System > Domain > Team > User > Thread > History）。
- 中途改需求：用户直接发新消息；若存在活跃 Run 则提示或先取消（前端交互，后端仍拒绝并发 Run）。

### 5.1 澄清卡片 Schema 契约（resume_schema_json）

Interrupt payload 中 `resume_schema_json` 遵循以下契约（`schema_version: 1`，M7 固定）。它是 Graph 声明、Go 校验、前端 `ClarificationCard` 渲染三方的**公共映射标准**，新增组件类型必须先修订本节再实现。

**顶层结构**：

```json
{
  "schema_version": 1,
  "title": "请补充报告范围",
  "description": "需要确认期间与部门后才能生成经营报告",
  "properties": {
    "period":      { "...字段定义，见下表..." : "" }
  },
  "required": ["period", "department"],
  "submit_label": "生成报告"
}
```

**完整示例**（财务报告助手的澄清）：

```json
{
  "schema_version": 1,
  "title": "请补充报告范围",
  "properties": {
    "period": {
      "component": "select",
      "label": "报告期间",
      "required": true,
      "default": "2026-Q3",
      "options": [
        {"value": "2026-Q3", "label": "2026 第三季度"},
        {"value": "2026-Q2", "label": "2026 第二季度"},
        {"value": "2026-Q1", "label": "2026 第一季度"}
      ]
    },
    "department": {
      "component": "text",
      "label": "部门",
      "placeholder": "如：华东销售部",
      "required": true,
      "max_length": 64
    },
    "compare_last_year": {
      "component": "checkbox",
      "label": "对比去年同期",
      "default": false
    },
    "threshold": {
      "component": "number",
      "label": "波动预警阈值（%）",
      "min": 0, "max": 100, "step": 5, "default": 10
    },
    "report_date": {
      "component": "date",
      "label": "报表基准日"
    },
    "invoice": {
      "component": "attachment",
      "label": "补充凭证（可选）",
      "accept": ["pdf", "jpg", "png"],
      "max_files": 3
    }
  },
  "required": ["period", "department"],
  "submit_label": "生成报告"
}
```

**组件类型映射表（M7 全集，共 7 种）**：

| component | 前端控件（AntD） | answer 值类型 | 支持的校验字段 |
|---|---|---|---|
| `text` | `Input` | string | `min_length`, `max_length`, `pattern`（Go regexp） |
| `textarea` | `Input.TextArea` | string | `min_length`, `max_length` |
| `select` | `Select` | string | `options[].value` 枚举（必须非空） |
| `number` | `InputNumber` | number | `min`, `max`, `step` |
| `date` | `DatePicker` | string `YYYY-MM-DD` | `min_date`, `max_date`（同格式） |
| `checkbox` | `Checkbox` | boolean | — |
| `attachment` | `Upload`（走既有文件上传 API） | string[]（file_id） | `accept`（扩展名白名单）, `max_files`, `max_size_mb`（默认 10） |

**Go 侧校验规则（`POST /answers` 处理器，按序执行）**：

1. `interrupt_id` 存在、属于该会话当前活跃 Run、状态为 `input_required`（防重放/防串会话）。
2. `answer` 仅允许包含 `properties` 中声明的键；未知键 → `400 unknown_field`。
3. `required` 字段缺失 → `400 missing_required_field`；类型不匹配 → `400 invalid_type`。
4. 枚举：值必须 ∈ `options[].value`；数值/长度/正则/日期范围逐项校验。
5. `attachment` 值必须是已上传文件的 `file_id`（前端先调文件 API 拿 id，answers 不接收裸文件）；Go 校验文件归属租户与扩展名。
6. 校验通过后 answer 原样作为 Resume payload 传给 Graph，**Go 不做语义级修改**。

**前端兼容约定**：

- 遇到未知 `component`：渲染兜底 `textarea` + 顶部提示"该字段类型暂不支持高级控件"，不阻断提交（向前兼容 M8 新组件）。
- 遇到未知 `schema_version`：按 v1 解析并告警上报（Sentry/console），Graph 侧承诺新增字段只增不改语义。
- `waiting_input` 期间输入区禁用自由文本，仅渲染澄清表单；`cancel` 按钮常驻。

**Graph 侧生成约束（Python）**：

- Interrupt 前必须先输出一句自然语言问题（`message.delta`），再发 `clarification.requested`——避免用户只见表单不见解释。
- 单次澄清字段数 ≤ 6（超过应拆成多轮）；每个 Agent 的澄清模板在包 `capabilities_json` 中声明，便于 Eval 检查澄清率。

## 6. Token 预算与会话生命周期

- 会话级预算落 `conversations.budget_json`（token 上限，默认继承租户配置）；超限 → Run `failed(budget_exceeded)` + error 事件，会话保留。
- 会话状态机：`active → waiting_input → active`；`active → waiting_approval → active`；任意态 → `closed`（用户关闭或 TTL 30 天自动归档）。
- 关闭会话不删除数据：审计与 Trace 保留。

## 7. 安全

- 租户隔离：所有查询强制 `tenant_id`，会话仅创建者可见（M7 无共享会话）。
- Prompt Injection：用户输入与检索内容一律视为数据；消息渲染端 Markdown 转义（前端 DOMPurify）。
- 审计：创建会话、发消息、回答澄清、取消均写 `audit_logs`（action: `conversation.*`）。
- 速率限制：发消息接口按用户维度限流（Redis 计数，默认 30 次/分钟）。
