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

-- Partial indexes over only the live rows (deleted_at IS NULL), so the ubiquitous
-- "list / detail" filters (WHERE deleted_at IS NULL) hit an index instead of a
-- full scan, and deleted rows never bloat the index.
--
-- NOTE: the indexed column is deliberately NOT deleted_at. On a partial index
-- whose predicate is "deleted_at IS NULL", every indexed row has deleted_at =
-- NULL, so an index keyed on deleted_at would be a constant — it could narrow the
-- scan set but could not support ORDER BY id / WHERE dataset_id = ? pushdown.
-- Key the indexes on the columns the queries actually order/filter by instead.
CREATE INDEX IF NOT EXISTS idx_datasource_not_deleted ON bi_datasource (id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_not_deleted    ON bi_dataset (datasource_id, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_lineage_not_deleted ON bi_dataset_lineage (upstream_dataset_id, downstream_dataset_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_chart_not_deleted      ON bi_chart (dataset_id, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_share_not_deleted      ON bi_share (chart_id) WHERE deleted_at IS NULL;

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
