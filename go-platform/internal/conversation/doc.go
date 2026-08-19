// Package conversation M7-A: 会话引擎 —— 面向用户的对话接口层。
//
// 本包围绕 agent_threads / agent_runs 提供用户友好的会话抽象:
//
//	Conversation  会话元数据(标题、状态、关联 agent_package)
//	ConversationMessage  最终落库的消息(user / assistant / system / tool)
//	SSEWriter     基于内存环形缓冲区的 Server-Sent Events 推送
//
// 核心链路: Conversation → AgentThread → DurableRun
//
// 组件分工:
//   - Repository  封装 conversations / conversation_messages 的 SQL 查询
//   - Service     编排 Thread 创建、Run 启动、消息持久化、SSE 事件分发
//   - Handler     处理 HTTP 请求,调用 Service,返回统一 JSON 响应
//   - SSEWriter   管理每个 conversation 的事件缓冲区,支持断线重连回放
package conversation