package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFreshMigrationChainPostgresAcceptance creates and drops only its own
// randomly named database, proving migrations 001..013 work from an empty DB.
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

	var migrationCount int
	if err := targetPool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		targetPool.Close()
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 15 {
		targetPool.Close()
		t.Fatalf("migration count = %d, want 15", migrationCount)
	}
	for _, table := range []string{
		"agent_threads", "agent_runs", "agent_run_steps", "runtime_events",
		"agent_checkpoints", "agent_interrupts",
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
