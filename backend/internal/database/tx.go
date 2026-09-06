package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// WithTx runs fn inside a database transaction, committing on success and
// rolling back on error.
func WithTx(ctx context.Context, db *bun.DB, fn func(ctx context.Context, tx bun.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Ensure rollback on panic; after a successful Commit this is a no-op
	// (Rollback returns ErrTxDone, which we ignore).
	defer tx.Rollback() //nolint:errcheck

	if err := fn(ctx, tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
