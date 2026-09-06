//go:build integration

package database

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestMigrationsUpToLatest runs all embedded migrations against a real
// PostgreSQL instance and verifies idempotency plus key table existence.
// It requires TEST_DATABASE_URL to point at a scratch database.
func TestMigrationsUpToLatest(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bunDB := bun.NewDB(db, pgdialect.New())
	defer bunDB.Close()
	if err := RunMigrations(bunDB); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	// 幂等：再跑一遍必须成功
	if err := RunMigrations(bunDB); err != nil {
		t.Fatalf("migrate idempotent: %v", err)
	}

	for _, table := range []string{"bi_datasource", "bi_dataset", "bi_dataset_lineage", "bi_chart", "bi_share"} {
		var exists bool
		if err := db.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist after migrations", table)
		}
	}
}
