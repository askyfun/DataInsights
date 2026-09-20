// Package keystore resolves, and if necessary creates, the persistent
// datasource-password encryption key (SECURITY_KEY).
//
// Design: the key is persisted in the bi_setting table, right next to the
// encrypted data it protects, so it rides the same PostgreSQL volume and stays
// stable across process restarts and container rebuilds — no extra mounted
// file or env var to forget. The environment variable remains the primary
// source (admin overrides DB); this package only kicks in when no env key is
// configured on first boot.
package keystore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// SettingKey is the bi_setting row key that holds the AES-256-GCM encryption key.
const SettingKey = "security_key"

// KeyBytes is the AES-256 key width in bytes.
const KeyBytes = 32

// GenerateKey returns a new random 32-byte AES key as lowercase hex.
func GenerateKey() (string, error) {
	buf := make([]byte, KeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate security key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// LoadOrCreate returns the current encryption key from storage, generating and
// persisting a new random one when none has been stored. The returned bool is
// true when a new key was created. Multi-instance bootstrap is safe: the
// INSERT uses ON CONFLICT DO NOTHING, so concurrent first boots converge on the
// same persisted key instead of diverging.
func LoadOrCreate(ctx context.Context, db *sql.DB) (keyHex string, created bool, err error) {
	// Fast path: an existing key. Keep it even if its value looks malformed —
	// overwriting it would silently re-key every encrypted password.
	var existing string
	err = db.QueryRowContext(ctx,
		`SELECT value FROM bi_setting WHERE key = $1`, SettingKey,
	).Scan(&existing)
	switch {
	case err == nil:
		return existing, false, nil
	case err != sql.ErrNoRows:
		return "", false, fmt.Errorf("read %s: %w", SettingKey, err)
	}

	keyHex, err = GenerateKey()
	if err != nil {
		return "", false, err
	}

	// ON CONFLICT DO NOTHING: if another boot inserted in the gap between our
	// SELECT and INSERT, keep theirs and re-read it below.
	res, err := db.ExecContext(ctx,
		`INSERT INTO bi_setting (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO NOTHING`, SettingKey, keyHex)
	if err != nil {
		return "", false, fmt.Errorf("persist %s: %w", SettingKey, err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		// A concurrent boot won the insert; adopt the persisted winner.
		err = db.QueryRowContext(ctx,
			`SELECT value FROM bi_setting WHERE key = $1`, SettingKey,
		).Scan(&keyHex)
		if err != nil {
			return "", false, fmt.Errorf("re-read %s after conflict: %w", SettingKey, err)
		}
		return keyHex, false, nil
	}

	return keyHex, true, nil
}
