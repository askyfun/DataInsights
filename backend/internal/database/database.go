package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"

	"data-insights/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func InitDB(databaseUrl string) (*bun.DB, error) {
	sqldb, err := sql.Open("pgx", databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqldb.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := bun.NewDB(sqldb, pgdialect.New())

	slog.Info("Database connected successfully")
	return db, nil
}

// migrationsDir is the path of the SQL files inside migrations.FS. The embed
// directive `//go:embed *.sql` places them at the FS root, hence ".".
const migrationsDir = "."

// RunMigrations applies all pending goose migrations embedded in the binary.
func RunMigrations(db *bun.DB) error {
	sqldb := db.DB

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.Up(sqldb, migrationsDir); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Migrations completed successfully")
	return nil
}

// CurrentMigrationVersion returns the highest applied goose migration version.
// Unlike goose.GetDBVersion it only reads the bookkeeping table, so a readiness
// probe never creates or mutates it.
func CurrentMigrationVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var version sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = true`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to read migration version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}

// LatestMigrationVersion returns the highest migration version embedded in the
// binary — the version a fully migrated database should be at.
func LatestMigrationVersion() (int64, error) {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("failed to set goose dialect: %w", err)
	}
	embedded, err := goose.CollectMigrations(migrationsDir, 0, math.MaxInt64)
	if err != nil {
		return 0, fmt.Errorf("failed to collect migrations: %w", err)
	}
	return embedded[len(embedded)-1].Version, nil
}

// MigrationsUpToDate reports whether the database has applied every migration
// embedded in this binary. A database ahead of the binary counts as up to date
// (rolling downgrades are a separate decision, not a readiness failure).
func MigrationsUpToDate(ctx context.Context, db *sql.DB) (current int64, upToDate bool, err error) {
	current, err = CurrentMigrationVersion(ctx, db)
	if err != nil {
		return 0, false, err
	}
	latest, err := LatestMigrationVersion()
	if err != nil {
		return current, false, err
	}
	return current, current >= latest, nil
}
