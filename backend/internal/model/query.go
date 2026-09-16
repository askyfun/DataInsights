package model

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
)

// QueryRecord is the bun model for the bi_query table.
//
// Each row is one persisted chart-query execution. The row is addressed by a
// non-guessable UUID (query_id) and deduplicated by the content hash of its
// spec_json (spec_hash). Expiry is decided solely by expires_at: NULL means
// permanent, otherwise the record is "soft-expired" once expires_at <= now().
// We never physically delete a record and never store a separate status enum
// (it would drift from expires_at).
type QueryRecord struct {
	bun.BaseModel `bun:"bi_query"`

	QueryID        string        `bun:"query_id,pk" json:"query_id"`
	SpecJSON       string        `bun:"spec_json,notnull" json:"spec_json"`
	SpecHash       string        `bun:"spec_hash,unique,notnull" json:"spec_hash"`
	DatasetID      int           `bun:"dataset_id,notnull" json:"dataset_id"`
	ChartID        sql.NullInt32 `bun:"chart_id" json:"chart_id"`
	SourceType     string        `bun:"source_type" json:"source_type"`
	RowCount       sql.NullInt32 `bun:"row_count" json:"row_count"`
	DurationMs     sql.NullInt32 `bun:"duration_ms" json:"duration_ms"`
	IP             string        `bun:"ip" json:"ip"`
	CreatedAt      time.Time     `bun:"created_at,notnull" json:"created_at"`
	LastAccessedAt sql.NullTime  `bun:"last_accessed_at" json:"last_accessed_at"`
	HitCount       int           `bun:"hit_count,notnull" json:"hit_count"`
	ExpiresAt      sql.NullTime  `bun:"expires_at" json:"expires_at"`
	OwnerID        sql.NullInt32 `bun:"owner_id" json:"owner_id"`
	TenantID       sql.NullInt32 `bun:"tenant_id" json:"tenant_id"`
}

// TableName satisfies bun's table naming (also covered by the bun tag).
func (*QueryRecord) TableName() string { return "bi_query" }
