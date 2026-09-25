package entity

// Dataset represents a logical data set derived from a datasource
type Dataset struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	DatasourceID     int     `json:"datasource_id"`
	TableName        *string `json:"table_name"`
	QuerySQL         *string `json:"query_sql"`
	QueryType        string  `json:"query_type"` // "table" or "sql"
	Mode             string  `json:"mode"`       // "direct"
	AccelerateConfig *string `json:"accelerate_config"`
	Description      *string `json:"description"`
	Tags             string  `json:"tags"`
	RefreshStrategy  *string `json:"refresh_strategy"`
	PreviewData      *string `json:"preview_data"`
	QualityRules     string  `json:"quality_rules"`
	Columns          string  `json:"columns"`
	ShardEnabled     bool    `json:"shard_enabled"`
	ShardKeys        string  `json:"shard_keys"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// DatasetColumn represents a column definition in a dataset
type DatasetColumn struct {
	// ID 是列的稳定标识（idgen 短 ID），落库后不再变化；Name 是可变展示名。
	// 图表配置 / shard_keys / 查询请求一律引用 ID，展示时才由 ID 解析回 Name。
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Expr       string     `json:"expr"`
	Type       string     `json:"type"`
	TypeConfig TypeConfig `json:"type_config"`
	Comment    string     `json:"comment"`
	Role       string     `json:"role"` // "dimension" or "metric"
}

// TypeConfig holds type-specific configuration
type TypeConfig struct {
	Precision int `json:"precision"`
	Scale     int `json:"scale"`
}

// QueryConfig represents a query configuration for dataset
type QueryConfig struct {
	DimensionGroups []FieldGroup `json:"dimension_groups"`
	MetricGroups    []FieldGroup `json:"metric_groups"`
	Filters         []Filter     `json:"filters"`
	Sort            *SortConfig  `json:"sort"`
	Limit           int          `json:"limit"`
}

// FieldGroup represents a group of fields
type FieldGroup struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields"`
	Alias  string   `json:"alias,omitempty"`
}

// Filter represents a filter condition
type Filter struct {
	ID       string      `json:"id"`
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	ValueEnd interface{} `json:"value_end,omitempty"`
	Logic    string      `json:"logic"`
}

// SortConfig represents sorting configuration
type SortConfig struct {
	Field string `json:"field"`
	Order string `json:"order"` // "asc" or "desc"
}
