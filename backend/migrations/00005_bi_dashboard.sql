-- +goose Up
-- Dashboards: a 12-column free grid that composes EXISTING charts into one page.
--
-- Design constraints (see deliverables/product-strategy/prd-dashboard-v1-2026-09-25.md):
--   * id is a non-guessable UUIDv7 (NEVER an auto-increment integer), same
--     rationale as bi_query.query_id: an enumerable id combined with the absence
--     of authentication would let anyone walk the entire dashboard table. The
--     application generates the value (uuid.NewV7), so no PG extension is needed.
--   * layout_json holds the whole composition (widget placement + widget
--     payloads) as a versioned document. It references charts BY ID and never
--     embeds a chart config snapshot: a chart edited once must show its new shape
--     in every dashboard that embeds it.
--   * status is the draft/published marker. v1 never exposes a publish action
--     (sharing is explicitly out of scope), but the column is created now because
--     it belongs to the irreversible surface: adding it later would leave every
--     pre-existing row without state.
--   * owner_id / tenant_id are kept (nullable) as irreversible audit columns for
--     the future account system (R-22). Four older tables already lack them and
--     that omission is tracked as debt; this new table must not repeat it.
--   * deleted_at follows the "never physically delete" contract.
--   * No FK to bi_chart, on purpose. A widget lives inside layout_json rather
--     than as its own row, so there is nothing to cascade: when a chart is
--     soft-deleted the dashboard row is left untouched and the widget renders a
--     placeholder at read time. A real FK would also wrongly reject the
--     legitimate case of one chart appearing in several dashboards (and twice in
--     the same one).

CREATE TABLE IF NOT EXISTS bi_dashboard (
    id          UUID PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    layout_json JSONB NOT NULL DEFAULT '{"version":1,"widgets":[]}'::jsonb,
    status      VARCHAR(20) NOT NULL DEFAULT 'draft',
    owner_id    INTEGER,
    tenant_id   INTEGER,
    created_at  TIMESTAMP NOT NULL DEFAULT now(),
    updated_at  TIMESTAMP NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMP NULL
);

-- List/summary index. The partial predicate uses IS NULL, which is IMMUTABLE and
-- therefore legal (unlike the now() case documented in 00003). The index key
-- cannot be deleted_at itself: every row satisfying the predicate has it
-- constantly NULL, which makes it a useless key.
CREATE INDEX IF NOT EXISTS idx_dashboard_not_deleted
    ON bi_dashboard (created_at DESC, id) WHERE deleted_at IS NULL;

-- Reverse lookup for "which dashboards embed chart X", surfaced as a delete-time
-- hint. jsonb_path_ops supports the @? existence operator and its index is
-- substantially smaller than the default jsonb_ops.
CREATE INDEX IF NOT EXISTS idx_dashboard_layout_gin
    ON bi_dashboard USING GIN (layout_json jsonb_path_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_dashboard_layout_gin;
DROP INDEX IF EXISTS idx_dashboard_not_deleted;
DROP TABLE IF EXISTS bi_dashboard;
