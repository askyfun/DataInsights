package database

import (
	"io/fs"
	"math"
	"testing"

	"dataray/migrations"
	"github.com/pressly/goose/v3"
)

// TestEmbeddedMigrationsPresent is a no-PG probe guarding the embed FS path
// used by RunMigrations: the SQL files must live at the embed FS root and
// goose must be able to collect them from the same dir string production
// passes to goose.Up.
func TestEmbeddedMigrationsPresent(t *testing.T) {
	info, err := fs.Stat(migrations.FS, "00001_init_schema.sql")
	if err != nil {
		t.Fatalf("00001_init_schema.sql missing from embed FS: %v", err)
	}
	if info.IsDir() {
		t.Fatal("00001_init_schema.sql resolved to a directory, not a file")
	}

	goose.SetBaseFS(migrations.FS)
	collected, err := goose.CollectMigrations(migrationsDir, 0, math.MaxInt64)
	if err != nil {
		t.Fatalf("goose failed to collect migrations from embed FS: %v", err)
	}
	if len(collected) == 0 {
		t.Fatal("goose collected no migrations from embed FS")
	}
}
