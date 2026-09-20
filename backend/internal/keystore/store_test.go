package keystore

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return db, m
}

func TestLoadOrCreateExistingKey(t *testing.T) {
	db, m := newMockDB(t)
	defer db.Close()
	m.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(SettingKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"))

	key, created, err := LoadOrCreate(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if created {
		t.Fatalf("expected created=false for existing key, got true")
	}
	if key != "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" {
		t.Fatalf("expected stored key, got %q", key)
	}
}

func TestLoadOrCreateGeneratesAndInserts(t *testing.T) {
	db, m := newMockDB(t)
	defer db.Close()
	m.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(SettingKey).
		WillReturnError(sql.ErrNoRows)
	m.ExpectExec(regexp.QuoteMeta(`INSERT INTO bi_setting (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`)).
		WithArgs(SettingKey, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	key, created, err := LoadOrCreate(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true on fresh insert, got false")
	}
	if len(key) != 64 {
		t.Fatalf("expected 64-char hex key, got %q (len %d)", key, len(key))
	}
}

func TestLoadOrCreateConflictAdoptsWinner(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(SettingKey).
		WillReturnError(sql.ErrNoRows)
	// Our INSERT lost the race (0 rows affected) — another boot persisted.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO bi_setting (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`)).
		WithArgs(SettingKey, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Re-read returns the winner's key.
	const winner = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(SettingKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(winner))

	key, created, err := LoadOrCreate(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if created {
		t.Fatalf("expected created=false when a concurrent boot won, got true")
	}
	if key != winner {
		t.Fatalf("expected winner key, got %q", key)
	}
}

func TestLoadOrCreateReadError(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM bi_setting WHERE key = $1`)).
		WithArgs(SettingKey).
		WillReturnError(sql.ErrConnDone)

	if _, _, err := LoadOrCreate(context.Background(), db); err == nil {
		t.Fatalf("expected error on read failure, got nil")
	}
}

func TestGenerateKeyLength(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(key))
	}
	k1, _ := GenerateKey()
	if key == k1 {
		t.Fatalf("two generated keys should differ, both %q", key)
	}
}
