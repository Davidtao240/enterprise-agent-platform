// Package database 负责 PostgreSQL 连接池管理和数据库迁移。
//
// 启动流程：
//  1. NewPool() → 创建 pgx 连接池
//  2. pool.Ping() → 验证数据库可达
//  3. RunMigrations() → 按文件名顺序执行 migrations/*.up.sql
//
// 迁移文件使用 .up.sql / .down.sql 命名约定，
// RunMigrations 只执行 .up.sql，确保不会被 down 脚本破坏数据。
package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool 创建 PostgreSQL 连接池。
//
// 参数来自 config.Config 的数据库配置字段。
// sslmode=disable 用于本地开发；生产环境应启用 SSL。
func NewPool(ctx context.Context, host, port, user, password, dbname string) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbname)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pgxpool config: %w", err)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// RunMigrations 执行 migrations 目录下的所有 .up.sql 文件。
//
// 工作原理：
//  - 通过 runtime.Caller 定位当前源文件路径
//  - 向上两级找到项目的 migrations/ 目录
//  - 按文件名排序，只执行 .up.sql 结尾的文件
//  - 跳过 .down.sql（回滚脚本）和 .sql 文件（非迁移）
//
// 迁移设计为幂等：建表用 IF NOT EXISTS，插入用 ON CONFLICT DO NOTHING。
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// 利用 runtime.Caller 定位当前文件，推导出 migrations 目录
	_, filename, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", migrationsDir, err)
	}

	for _, e := range entries {
		// 只执行 .up.sql，跳过目录、.down.sql 和其他文件
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		path := filepath.Join(migrationsDir, e.Name())
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// 整个 SQL 文件作为一批执行（含多条语句）
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", e.Name(), err)
		}
		fmt.Printf("[migration] applied %s\n", e.Name())
	}
	return nil
}
