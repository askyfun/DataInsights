-- +goose Up
-- Soft delete: every entity gains a nullable deleted_at marker (NULL = live).
-- The previous hard deletes (ON DELETE CASCADE) are replaced by explicit
-- soft deletes in the service layer, so no row is ever physically removed.
-- Existing rows keep deleted_at NULL, so this migration is non-destructive.

ALTER TABLE bi_datasource       ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;
ALTER TABLE bi_dataset          ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;
ALTER TABLE bi_dataset_lineage ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;
ALTER TABLE bi_chart            ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;
ALTER TABLE bi_share           ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL;

-- Partial index over only the live rows (deleted_at IS NULL), so the ubiquitous
-- "list / detail" filters (WHERE deleted_at IS NULL) hit an index instead of a
-- full scan, and deleted rows never bloat the index.
CREATE INDEX IF NOT EXISTS idx_datasource_not_deleted ON bi_datasource (deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_not_deleted    ON bi_dataset (deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_lineage_not_deleted ON bi_dataset_lineage (deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_chart_not_deleted      ON bi_chart (deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_share_not_deleted      ON bi_share (deleted_at) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_share_not_deleted;
DROP INDEX IF EXISTS idx_chart_not_deleted;
DROP INDEX IF EXISTS idx_dataset_lineage_not_deleted;
DROP INDEX IF EXISTS idx_dataset_not_deleted;
DROP INDEX IF EXISTS idx_datasource_not_deleted;

ALTER TABLE bi_share           DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE bi_chart           DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE bi_dataset_lineage DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE bi_dataset         DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE bi_datasource       DROP COLUMN IF EXISTS deleted_at;
