# Agent Gallery

> 文档状态：Active Specification (M7-B Target；M8 扩展安装/卸载语义)
> 更新日期：2026-08-18
> 相关：[ADR-007](../02_ARCHITECTURE/ADR-007_CONVERSATION_ENGINE_SSE.md)、[GRAPH_ROUTING_AND_ISOLATION.md](../02_ARCHITECTURE/GRAPH_ROUTING_AND_ISOLATION.md)、[CONVERSATION_ENGINE.md](CONVERSATION_ENGINE.md)、[SEED_DATA.md](SEED_DATA.md)

## 1. 定位

Agent 画廊是 Agent 的**发现与选择层**（员工第一屏），不是执行层。选择结果解析为受治理的 `graph_key`，走既有 Agent Runtime Gateway 校验链；画廊不放宽任何权限与 Domain Policy。

## 2. 分类

| category | 含义 | 绑定 | M7 示例 |
|---|---|---|---|
| general | 通用 Agent，不绑定部门 | business_app_code = productivity（通用办公） | 文档总结、会议纪要 |
| departmental | 部门 Agent | business_app_code = finance/hr/... | 财务经营报告助手 |

## 3. agent_packages 模型

表定义见 [DATABASE_SCHEMA.md](DATABASE_SCHEMA.md) M7 Target。关键字段：

| 字段 | 说明 |
|---|---|
| package_code | 唯一标识（如 `document_summary_assistant`） |
| category | general / departmental |
| business_app_code | 归属业务域 |
| graph_key | **注册时校验必须存在于 graph_registry 且 active；运行时不可改写** |
| entry_type | conversation（对话式）/ form（表单流程，兜底） |
| capabilities_json | 能力描述（供画廊卡片与 LLM 提示词） |
| sample_prompts_json | 示例 Prompt（画廊一键发起） |
| status | draft / published / disabled |

生命周期（M7）：管理员预置 + 上下架（`agent:manage`）。（M8）：安装/卸载/版本/审核流 UI 化，接入 `configuration_versions` 治理。

### 3.1 包状态机（M7 定稿）

```text
                agent:manage（上架）
     ┌───────── draft ─────────────┐
     │            │                ▼
     │            │           published ⇄ disabled
     │            │           （下架）  （重新上架）
     │            └── disabled（draft 直接废弃，不进画廊）
     ▼
  （M8 新增 publishing 审核态，M7 不做）
```

M7 状态迁移表：

| 迁移 | 权限 | 前置校验 | 副作用 |
|---|---|---|---|
| draft → published | `agent:manage` | graph_key 存在且 active；business_app_code 存在 | 画廊可见、可发起会话；写 audit_logs |
| published → disabled | `agent:manage` | — | 画廊隐藏、不可新建会话；**存量会话转只读**；写 audit_logs |
| disabled → published | `agent:manage` | 同上架校验 | 重新可见 |
| draft → disabled | `agent:manage` | — | 废弃，不进画廊 |

全部迁移走 `PATCH /agent-packages/:code {status}`，服务端按本表校验非法迁移并返回 `409 invalid_transition`。M7 不做审核流（管理员操作即生效，靠审计追溯）；M8 引入 `publishing`（提交审核）态后，本表扩展但不破坏既有迁移。

### 3.2 安装语义（M7 / M8 分阶段定稿）

**分阶段策略（评审 2026-08-18）：数据模型与 API 按"包"设计（中量级骨架），M7 用轻量方式跑通闭环，M8 升级为原子安装。**

| | M7（当前） | M8（目标） |
|---|---|---|
| 包的本质 | **目录/manifest 层**：引用既有注册资源（business_app、graph、agent、tool permissions 由 seed 或既有 Registry API 创建），包行只负责"发现 + 入口 + 示例" | **自包含安装单元**：安装一个包 = 在**单事务**内创建 business_app + workflow_template + agent 注册 + tool_permissions + agent_packages 行 |
| 官方预置包 | 由 seed 脚本安装：seed 事务内原子创建上述全部资源 + 3 个包行，模拟"安装即用"体验 | 打包为独立 Agent 包（目录规范见 M8 `AGENT_PACKAGE_SPEC.md`），支持上传/卸载 |
| 安装入口 | 无 UI（seed 预置 + 管理员 `POST /agent-packages` 注册引用型包） | 市场页"安装"按钮 → 原子安装事务 |
| 卸载 | 仅 disable（数据保留） | disable + 可选清理未使用的注册资源（有引用检查），conversations/审计永久保留 |

**不变式（M7/M8 一致）**：

1. 包注册/上架时必须校验 `graph_key` 存在且 active，运行时不可改写（路由权威在 Go 注册中心）。
2. 包不携带权限——它引用的 agent/tool 权限仍由 RBAC + Domain Policy 治理；画廊只是发现层，不放宽任何权限。
3. 卸载/禁用不删除任何会话、消息、Run、审计数据。
4. M8 安装事务失败必须整体回滚（不允许出现"半装"的包）。

## 4. 使用统计（真实聚合，只读汇总表）

**产品决策（2026-08-18）：M7 做真实聚合（不造假数据），但画廊列表一律读只读汇总表 `agent_package_usage_stats`，禁止列表页对 `conversations` 全表 `COUNT(*)`。**

- 画廊卡片展示 `usage_count`（累计会话数）：`SELECT SUM(conversation_count) FROM agent_package_usage_stats WHERE tenant_id=$1 AND package_code IN (...)`（表极小，随列表页一次查询带回）。
- 汇总表写入方唯一：Asynq 每日任务（02:00）聚合前一天数据 UPSERT；统计 T+1 可见，M7 接受。
- 包详情页（单包）允许实时查询当日统计（单包单查，不构成全表压力）。
- 表结构见 [DATABASE_SCHEMA.md](DATABASE_SCHEMA.md) M7 Target。

## 5. API

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/agent-gallery?category=&business_app=&q=` | `business_app:read` | 画廊列表（仅 published；支持分类/业务域/关键词过滤）；响应含 `usage_count`（读汇总表 SUM）与 `last_used_at` |
| GET | `/api/v1/agent-gallery/:code` | `business_app:read` | 包详情（能力、示例 Prompt、使用统计：累计会话数/近 7 天趋势/当日实时） |
| POST | `/api/v1/agent-packages` | `agent:manage` | 注册包（校验 graph_key 有效性与 Domain Policy；初始 status=draft） |
| PATCH | `/api/v1/agent-packages/:code` | `agent:manage` | 更新元数据 / 上下架（状态迁移按 §3.1 校验，非法迁移 409） |

## 6. 前端契约

- 员工第一屏（`/`）：`AgentCard` 网格 + 分类 Tab（全部/通用/各部门）+ 搜索框。
- 卡片字段：图标、名称、一句话描述、分类徽标、来源（官方/部门）、`usage_count`（真实统计，格式化为 "1.2k 次"）。
- 点击卡片 → `POST /conversations`（entry_type=conversation）→ 跳转 `/conversations/:id`；表单型跳既有业务中心。
- 示例 Prompt：卡片或详情页一键发起会话并预填首条消息（文案定稿见 [WORKBENCH_DESIGN.md](../06_FRONTEND/WORKBENCH_DESIGN.md) §7.5）。

## 7. 治理与约束

- `graph_key` 校验发生在注册与发布时；运行期 graph 失效（disabled/deprecated）→ 包自动隐藏，已有会话只读。
- 包发布进 `configuration_versions`（resource_type 沿用 graph/agent 治理），不绕过审批流。
- 禁用包：画廊隐藏、不可新建会话；存量会话与 Run 数据保留（审计要求）。
- 跨租户：agent_packages 属租户资源，画廊查询天然隔离；官方预置包按租户初始化时安装。
- 上下架操作全部写 audit_logs（action: `agent_package.publish` / `agent_package.disable`），M7 无审核流但审计不缺位。
