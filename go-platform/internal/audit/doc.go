// Package audit 记录所有关键业务操作，用于合规审计和可追溯性。
//
// 已实现：
//   - InsertLog：写入审计日志（供 Gateway/Worker/Handler 调用）
//   - ListLogs：分页查询 + 多条件过滤
//   - ListAuditLogs（HTTP handler）：GET /api/v1/audit-logs
package audit
