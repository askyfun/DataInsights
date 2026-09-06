package entity

// Datasource represents a data source connection configuration
type Datasource struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DatabaseName string `json:"database_name"`
	Username     string `json:"username"`
	// Password never leaves the API surface: responses must not expose the
	// stored (encrypted) value; requests use dedicated handler bindings.
	Password  string `json:"-"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// DatasourceConnectionConfig holds connection parameters for establishing database connections
type DatasourceConnectionConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DatabaseName string `json:"database_name"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

// TableInfo represents metadata about a database table
type TableInfo struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// ColumnInfo represents metadata about a database column
type ColumnInfo struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Comment  string `json:"comment"`
}

// PreviewResult represents data preview from a datasource
type PreviewResult struct {
	Columns []string         `json:"columns"`
	Data    []map[string]any `json:"data"`
}

// TableDataResult represents paginated table data with metadata
type TableDataResult struct {
	Columns     []string         `json:"columns"`
	Data        []map[string]any `json:"data"`
	Total       int64            `json:"total"`
	PrimaryKeys []string         `json:"primary_keys"`
	Page        int              `json:"page"`
	PageSize    int              `json:"page_size"`
}

// FieldDistribution represents the distribution of values in a field
type FieldDistribution struct {
	FieldName    string            `json:"field_name"`
	TotalCount   int64             `json:"total_count"`
	UniqueCount  int               `json:"unique_count"`
	Distribution []FieldValueCount `json:"distribution"`
}

// FieldValueCount represents a single value's count in distribution
type FieldValueCount struct {
	Value      any     `json:"value"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}
