package database

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const currentVersionQuery = `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = true`

func TestLatestMigrationVersionMatchesEmbeddedFiles(t *testing.T) {
	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}
	// 00012 是本文件写入时的最新迁移；新迁移只会让它变大，不会让它变小。
	if latest < 12 {
		t.Fatalf("expected at least migration 12, got %d", latest)
	}
}

func TestCurrentMigrationVersionReadsBookkeepingTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(currentVersionQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(12))

	got, err := CurrentMigrationVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("CurrentMigrationVersion: %v", err)
	}
	if got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
}

func TestMigrationsUpToDate(t *testing.T) {
	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(currentVersionQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(latest))

	current, upToDate, err := MigrationsUpToDate(context.Background(), db)
	if err != nil {
		t.Fatalf("MigrationsUpToDate: %v", err)
	}
	if current != latest || !upToDate {
		t.Fatalf("expected current=%d up-to-date, got current=%d up=%v", latest, current, upToDate)
	}
}

func TestMigrationsUpToDateDetectsPending(t *testing.T) {
	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(currentVersionQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(latest - 1))

	_, upToDate, err := MigrationsUpToDate(context.Background(), db)
	if err != nil {
		t.Fatalf("MigrationsUpToDate: %v", err)
	}
	if upToDate {
		t.Fatal("expected pending migrations to report not up to date")
	}
}
