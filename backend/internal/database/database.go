package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"dataray/migrations"
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
