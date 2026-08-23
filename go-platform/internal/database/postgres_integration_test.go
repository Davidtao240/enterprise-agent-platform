package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFreshMigrationChainPostgresAcceptance creates and drops only its own
// randomly named database, proving all migrations work from an empty DB.
func TestFreshMigrationChainPostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	defer adminPool.Close()

	databaseName := "eap_m1_acceptance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create isolated acceptance database: %v", err)
	}
	defer func() {
		if _, err := adminPool.Exec(context.Background(), "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated acceptance database %s: %v", databaseName, err)
		}
	}()

	targetConfig := adminConfig.Copy()
	targetConfig.ConnConfig.Database = databaseName
	targetPool, err := pgxpool.NewWithConfig(ctx, targetConfig)
	if err != nil {
		t.Fatalf("connect isolated acceptance database: %v", err)
	}
	if err := RunMigrations(ctx, targetPool); err != nil {
		targetPool.Close()
		t.Fatalf("run fresh migration chain: %v", err)
	}

	// 期望的迁移数 = migrations 目录下 .up.sql 文件数（动态计算，避免硬编码漂移）
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		targetPool.Close()
		t.Fatal("locate test file")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		targetPool.Close()
		t.Fatalf("read migrations dir: %v", err)
	}
	wantCount := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			wantCount++
		}
	}

	var migrationCount int
	if err := targetPool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		targetPool.Close()
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != wantCount {
		targetPool.Close()
		t.Fatalf("migration count = %d, want %d", migrationCount, wantCount)
	}
	for _, table := range []string{
		"agent_threads", "agent_runs", "agent_run_steps", "runtime_events",
		"agent_checkpoints", "agent_interrupts", "tool_calls",
	} {
		var exists bool
		if err := targetPool.QueryRow(ctx,
			`SELECT to_regclass('public.' || $1) IS NOT NULL`, table,
		).Scan(&exists); err != nil || !exists {
			targetPool.Close()
			t.Fatalf("table %s exists = %v, error = %v", table, exists, err)
		}
	}
	targetPool.Close()
}
