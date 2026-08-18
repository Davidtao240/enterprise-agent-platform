// Package contextbuilder 实现 M4-C:上下文组装管道。
//
// 管道(Spec MEMORY_AND_CONTEXT.md §3):
//
//	来源聚合(System/Domain/Team/User/Thread/History)
//	→ ACL 过滤(基于 viewer 身份与角色)
//	→ Token 预算裁剪(History 最先裁,Domain 尽量保留,System 绝对保留)
//	→ 组装输出(System -> Domain -> Team -> User -> Thread -> History)
//
// 供 Python Agent Service 经 internal HTTP 端点调用。
package contextbuilder
