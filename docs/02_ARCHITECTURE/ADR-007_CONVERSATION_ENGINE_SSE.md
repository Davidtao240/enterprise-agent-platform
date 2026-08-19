# ADR-007: 对话引擎与 SSE 流式通道

> 状态：Accepted
> 日期：2026-08-18
> 里程碑：M7-A
> 相关：[AGENTIC_WORKBENCH_M7_M9_DESIGN.md §4](../05_FUTURE/AGENTIC_WORKBENCH_M7_M9_DESIGN.md)、[CONVERSATION_ENGINE.md](../03_PLATFORM_SPEC/CONVERSATION_ENGINE.md)、[ADR-003_DURABLE_AGENT_RUN.md](ADR-003_DURABLE_AGENT_RUN.md)

## 背景

M1-M6 交付的是"表单 → 工作流 → 审批"范式，系统内没有任何对话入口。产品评审（2026-08-18）确认"体验不是对话式"为首要缺陷，M7-A 需要建立对话引擎：多轮会话、澄清追问、流式输出、会话持久化。

## 决策

1. **通道选型：SSE，不用 WebSocket**
   - 对话流式本质是服务端单向推送（token 增量、状态事件），客户端无高频写通道需求。
   - EventSource 断线自动重连 + `Last-Event-Id` 续传天然匹配"会话不丢帧"要求。
   - 直接复用现有 Gin 中间件（JWT 认证、租户上下文），无需引入 ws 网关与心跳协议。
2. **全链路流式**：LangGraph `astream_events` → Python Runtime V2 事件 → Go SSE 转发（`text/event-stream`）→ 前端 EventSource 打字机渲染。
3. **会话模型**：`conversations` / `conversation_messages` 落 PostgreSQL，Go 为权威（System of Record）；对话底层复用 `agent_threads` + Durable Run，不另造执行状态机。
4. **澄清机制**：Graph 以 `Interrupt(kind=input_required)` 发起澄清（复用 M1-B `agent_interrupts`），前端渲染澄清卡片，用户回答走 Resume；不引入独立问答协议。
5. **路由不变**：会话由画廊选择的 `agent_package` 解析出 `graph_key`（可信注册数据）；LLM 不得改写 `graph_key`（见 [GRAPH_ROUTING_AND_ISOLATION.md](GRAPH_ROUTING_AND_ISOLATION.md)「对话入口的路由」）。

## 备选方案

| 方案 | 结论 | 原因 |
|---|---|---|
| WebSocket | 否 | 双向能力冗余；断线重连、代理穿透、认证复用成本高于 SSE |
| 前端轮询 | 否 | 无法实现打字机效果；Run 详情页已有轮询，会话层不重复建设 |
| gRPC streaming | 否 | 浏览器端需 grpc-web 代理，复杂度与收益不匹配 |

## 影响

- 新增 API 与 SSE 事件契约见 [CONVERSATION_ENGINE.md](../03_PLATFORM_SPEC/CONVERSATION_ENGINE.md) §3-4；表结构见 [DATABASE_SCHEMA.md](../03_PLATFORM_SPEC/DATABASE_SCHEMA.md) M7 Target。
- Go 侧新增 conversation handler 与 SSE writer；反向代理需对该路径关闭响应缓冲（`X-Accel-Buffering: no`）。
- 断线恢复：SSE 重连后从 `Last-Event-Id` 对应的 sequence 续发；服务端保留最近事件窗口（默认 1000 条/会话）。
- 与审批复用：`approval.requested` 事件触发前端审批卡片，决策仍走既有 Approval API，不在 SSE 通道内做写操作。
