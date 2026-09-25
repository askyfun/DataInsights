package model

import (
	"database/sql"
	"encoding/json"

	"github.com/uptrace/bun"
)

type Datasource struct {
	bun.BaseModel `bun:"bi_datasource"`

	ID           int          `bun:"id,pk,autoincrement" json:"id"`
	Name         string       `bun:"name" json:"name"`
	Type         string       `bun:"type" json:"type"`
	Host         string       `bun:"host" json:"host"`
	Port         int          `bun:"port" json:"port"`
	DatabaseName string       `bun:"database_name" json:"database_name"`
	Username     string       `bun:"username" json:"username"`
	Password     string       `bun:"password" json:"password"`
	CreatedAt    sql.NullTime `bun:"created_at" json:"created_at"`
	UpdatedAt    sql.NullTime `bun:"updated_at" json:"updated_at"`
	DeletedAt    sql.NullTime `bun:"deleted_at,nullzero" json:"-"`
}

// Dataset represents a data set
type Dataset struct {
	bun.BaseModel `bun:"bi_dataset"`

	ID               int            `bun:"id,pk,autoincrement" json:"id"`
	Name             string         `bun:"name" json:"name"`
	DatasourceID     int            `bun:"datasource_id" json:"datasource_id"`
	TableName        sql.NullString `bun:"table_name" json:"table_name"`
	QuerySQL         sql.NullString `bun:"query_sql" json:"query_sql"`
	QueryType        string         `bun:"query_type" json:"query_type"`
	Mode             string         `bun:"mode" json:"mode"`
	AccelerateConfig sql.NullString `bun:"accelerate_config" json:"accelerate_config"`
	Description      sql.NullString `bun:"description" json:"description"`
	Tags             string         `bun:"tags" json:"tags"`
	RefreshStrategy  sql.NullString `bun:"refresh_strategy" json:"refresh_strategy"`
	PreviewData      sql.NullString `bun:"preview_data" json:"preview_data"`
	QualityRules     string         `bun:"quality_rules" json:"quality_rules"`
	Columns          string         `bun:"columns" json:"columns"`
	ShardEnabled     bool           `bun:"shard_enabled" json:"shard_enabled"`
	ShardKeys        string         `bun:"shard_keys" json:"shard_keys"`
	CreatedAt        sql.NullTime   `bun:"created_at" json:"created_at"`
	UpdatedAt        sql.NullTime   `bun:"updated_at" json:"updated_at"`
	DeletedAt        sql.NullTime   `bun:"deleted_at,nullzero" json:"-"`
}

// MarshalJSON implements custom JSON marshaling for Dataset
func (d Dataset) MarshalJSON() ([]byte, error) {
	type Alias Dataset
	aux := &struct {
		TableName        any `json:"table_name"`
		QuerySQL         any `json:"query_sql"`
		AccelerateConfig any `json:"accelerate_config"`
		Description      any `json:"description"`
		RefreshStrategy  any `json:"refresh_strategy"`
		PreviewData      any `json:"preview_data"`
		CreatedAt        any `json:"created_at"`
		UpdatedAt        any `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(&d),
	}

	if d.TableName.Valid {
		aux.TableName = d.TableName.String
	} else {
		aux.TableName = nil
	}
	if d.QuerySQL.Valid {
		aux.QuerySQL = d.QuerySQL.String
	} else {
		aux.QuerySQL = nil
	}
	if d.AccelerateConfig.Valid {
		aux.AccelerateConfig = d.AccelerateConfig.String
	} else {
		aux.AccelerateConfig = nil
	}
	if d.Description.Valid {
		aux.Description = d.Description.String
	} else {
		aux.Description = nil
	}
	if d.RefreshStrategy.Valid {
		aux.RefreshStrategy = d.RefreshStrategy.String
	} else {
		aux.RefreshStrategy = nil
	}
	if d.PreviewData.Valid {
		aux.PreviewData = d.PreviewData.String
	} else {
		aux.PreviewData = nil
	}
	if d.CreatedAt.Valid {
		aux.CreatedAt = d.CreatedAt.Time
	} else {
		aux.CreatedAt = nil
	}
	if d.UpdatedAt.Valid {
		aux.UpdatedAt = d.UpdatedAt.Time
	} else {
		aux.UpdatedAt = nil
	}

	return json.Marshal(aux)
}

type DatasetColumn struct {
	// ID 是列的稳定标识（idgen 短 ID），落库后不再变化；Name 是可变展示名。
	// 图表配置、shard_keys、查询请求一律引用 ID，展示时才由 ID 解析回 Name。
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Expr       string           `json:"expr"`
	Type       StandardDataType `json:"type"`
	TypeConfig TypeConfig       `json:"type_config"`
	Comment    string           `json:"comment"`
	Role       string           `json:"role"`
}

// DatasetLineage represents lineage relationship between datasets
type DatasetLineage struct {
	bun.BaseModel `bun:"bi_dataset_lineage"`

	ID                  int          `bun:"id,pk,autoincrement" json:"id"`
	UpstreamDatasetID   int          `bun:"upstream_dataset_id" json:"upstream_dataset_id"`
	DownstreamDatasetID int          `bun:"downstream_dataset_id" json:"downstream_dataset_id"`
	CreatedAt           sql.NullTime `bun:"created_at" json:"created_at"`
	DeletedAt           sql.NullTime `bun:"deleted_at,nullzero" json:"-"`
}

// Chart represents a chart configuration
type Chart struct {
	bun.BaseModel `bun:"bi_chart"`

	ID        int          `bun:"id,pk,autoincrement" json:"id"`
	Name      string       `bun:"name" json:"name"`
	DatasetID int          `bun:"dataset_id" json:"dataset_id"`
	ChartType string       `bun:"chart_type" json:"chart_type"`
	Config    string       `bun:"config" json:"config"`
	CreatedAt sql.NullTime `bun:"created_at" json:"created_at"`
	UpdatedAt sql.NullTime `bun:"updated_at" json:"updated_at"`
	DeletedAt sql.NullTime `bun:"deleted_at,nullzero" json:"-"`
}

// Share represents a shared chart
type Share struct {
	bun.BaseModel `bun:"bi_share"`

	ID        int            `bun:"id,pk,autoincrement" json:"id"`
	Token     string         `bun:"token" json:"token"`
	ChartID   int            `bun:"chart_id" json:"chart_id"`
	Password  sql.NullString `bun:"password" json:"password"`
	ExpiresAt sql.NullTime   `bun:"expires_at" json:"expires_at"`
	CreatedAt sql.NullTime   `bun:"created_at" json:"created_at"`
	DeletedAt sql.NullTime   `bun:"deleted_at,nullzero" json:"-"`
}
