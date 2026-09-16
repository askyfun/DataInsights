-- +goose Up
-- Query records: every chart query execution is persisted as one row so that
-- the address-bar id (shareable link) and the history list can consume it.
--
-- Design constraints (PRD v2 + v3):
--   * query_id is a non-guessable UUIDv4 (NEVER an auto-increment integer).
--   * Expiry is encoded by a SINGLE column `expires_at` (NULL = permanent).
--     We deliberately do NOT add a `status` enum: status would drift from the
--     REAL `expires_at` value, which is the source of truth.
--   * No partial index: PostgreSQL partial-index predicates must be IMMUTABLE,
--     but `now()` is STABLE, so `WHERE expires_at > now()` is rejected with
--     "functions in index predicate must be marked IMMUTABLE". We use a plain
--     composite index instead; expiry filtering is a post-scan filter predicate.
--   * owner_id / tenant_id are kept (nullable) as irreversible audit columns for
--     the future account system (R-22). They carry no value in L0 (no accounts).

CREATE TABLE IF NOT EXISTS bi_query (
    query_id         UUID PRIMARY KEY,
    spec_json        JSONB NOT NULL,
    spec_hash        VARCHAR(64) NOT NULL UNIQUE,
    dataset_id       INTEGER NOT NULL,
    chart_id         INTEGER,
    source_type      VARCHAR(50),
    row_count        INTEGER,
    duration_ms      INTEGER,
    ip               VARCHAR(64),
    created_at       TIMESTAMP NOT NULL DEFAULT now(),
    last_accessed_at TIMESTAMP,
    hit_count        INTEGER NOT NULL DEFAULT 0,
    expires_at       TIMESTAMP,
    owner_id         INTEGER,
    tenant_id        INTEGER
);

-- Composite index powering keyset pagination on (created_at, query_id).
CREATE INDEX IF NOT EXISTS idx_query_created_at_id
    ON bi_query (created_at DESC, query_id);

-- Plain index on the expiry column (used only as a post-scan filter predicate,
-- never as a partial index).
CREATE INDEX IF NOT EXISTS idx_query_expires_at
    ON bi_query (expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_query_expires_at;
DROP INDEX IF EXISTS idx_query_created_at_id;
DROP TABLE IF EXISTS bi_query;
