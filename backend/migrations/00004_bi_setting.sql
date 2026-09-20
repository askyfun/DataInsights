-- +goose Up
-- Key-value settings table: a compact, single-row-per-key store for
-- runtime-persisted operational values. The only consumer today is the
-- datasource-password encryption key (SECURITY_KEY): when no key is provided
-- via the environment at first boot, the process generates a 32-byte random
-- AES key and persists it here, so restarts / container rebuilds keep the same
-- key and already-encrypted passwords stay decryptable. Persisting in the
-- database (instead of a mounted file) means it rides the same PostgreSQL
-- volume as the data it protects, with no extra volume to configure.

CREATE TABLE IF NOT EXISTS bi_setting (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS bi_setting;