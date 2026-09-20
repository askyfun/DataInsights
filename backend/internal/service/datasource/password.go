package datasource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"data-insights/internal/crypto"
	"data-insights/internal/model"

	"github.com/uptrace/bun"
)

// ResolvePassword resolves the password used to establish a connection for the
// given datasource model. With a security key configured, stored ciphertext is
// decrypted; a legacy plaintext value (no v1: prefix) is used as-is and
// transparently upgraded by writing the encrypted value back. Without a key,
// plaintext passthrough. Shared by the datasource, chart and dataset connect
// paths so every consumer of a stored datasource password decrypts it exactly
// once.
func ResolvePassword(ctx context.Context, db bun.IDB, ds *model.Datasource, key []byte) (string, error) {
	if key == nil {
		return ds.Password, nil
	}
	if ds.Password == "" {
		return "", nil
	}
	pt, err := crypto.Decrypt(key, ds.Password)
	if err == nil {
		return pt, nil
	}
	if !errors.Is(err, crypto.ErrNotEncrypted) {
		return "", fmt.Errorf("decrypt datasource password: %w", err)
	}
	ct, err := crypto.Encrypt(key, ds.Password)
	if err != nil {
		return "", fmt.Errorf("upgrade legacy datasource password: %w", err)
	}
	if _, err := db.NewUpdate().Model(&model.Datasource{ID: ds.ID, Password: ct}).Column("password").WherePK().Where("deleted_at IS NULL").Exec(ctx); err != nil {
		return "", fmt.Errorf("upgrade legacy datasource password: %w", err)
	}
	slog.Info("upgraded legacy plaintext datasource password", "datasource_id", ds.ID)
	return ds.Password, nil
}
