// Package skill 实现 M4-B:Skill 版本化注册与生命周期治理。
//
// Skill 是可复用的 Agent 能力单元(Prompt 模板、工具绑定、模型参数)。
// 生命周期状态机(Spec MEMORY_AND_CONTEXT.md §4.1):
//
//	draft -> review -> published -> deprecated
//
// 不变量:
//   - draft 状态配置可修改;review/published/deprecated 配置锁定
//   - published 起可供 Agent 引用,deprecated 不可逆但保持兼容引用
//   - (skill_code, version) 唯一,版本不可变
package skill
