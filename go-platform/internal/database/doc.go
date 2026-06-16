// Package database 提供 PostgreSQL 连接池和迁移工具。
//
// 核心能力：
//   - NewPool: 创建并返回 pgxpool 连接池
//   - RunMigrations: 自动按序执行 SQL 迁移脚本
//
// 两个函数在 main.go 启动阶段依次调用：先建池 → 后迁移。
package database
