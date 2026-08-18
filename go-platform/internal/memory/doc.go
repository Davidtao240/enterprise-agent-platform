// Package memory 实现 M4-A:Agent 分层记忆存储。
//
// Memory 按作用域分为 run/thread/user/team/domain 五层,决定生命周期与可见性:
//   - run:    单次 Agent Run 期间 (运行时临时变量)
//   - thread: 单次对话期间 (对话历史摘要)
//   - user:   长期,仅用户本人 (用户偏好)
//   - team:   长期,团队共享 (业务术语、SOP)
//   - domain: 长期,全租户 (领域知识、全局配置)
//
// 安全边界:
//   - 所有查询强制 tenant_id 过滤(跨租户隔离)
//   - ACL 为空/[] 时仅创建者可见;否则匹配 "user:<id>" 或 "role:<code>"
//   - 过期(expires_at)与软删除(deleted_at)条目不参与检索
package memory
