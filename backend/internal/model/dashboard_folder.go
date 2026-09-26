package model

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
)

// DashboardFolder is the bun model for the bi_dashboard_folder table
// (migration 00007).
//
// The tree is expressed by the self-reference ParentID only — there is no
// materialized path column, so moving a folder touches exactly one row and the
// cycle guard walks the chain in the service layer.
//
// ParentID is nullable and points at another row of the same table, but there
// is deliberately no FK: under soft-delete semantics a real FK would reject the
// legitimate state "child kept while its folder was deleted elsewhere". Readers
// filter deleted_at, so an orphan parent only degrades to root-level display.
//
// DeletedAt is json:"-" like every other soft-delete column: nothing is ever
// physically deleted.
type DashboardFolder struct {
	bun.BaseModel `bun:"bi_dashboard_folder"`

	ID        string         `bun:"id,pk" json:"id"`
	Name      string         `bun:"name,notnull" json:"name"`
	ParentID  sql.NullString `bun:"parent_id" json:"parent_id"`
	OwnerID   sql.NullInt32  `bun:"owner_id" json:"owner_id"`
	TenantID  sql.NullInt32  `bun:"tenant_id" json:"tenant_id"`
	CreatedAt time.Time      `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time      `bun:"updated_at,notnull" json:"updated_at"`
	DeletedAt sql.NullTime   `bun:"deleted_at" json:"-"`
}

// TableName satisfies bun's table naming (also covered by the bun tag).
func (*DashboardFolder) TableName() string { return "bi_dashboard_folder" }
