package entity

// Chart represents a visualization chart configuration
type Chart struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	DatasetID int    `json:"dataset_id"`
	ChartType string `json:"chart_type"` // "line", "bar", "pie"
	Config    string `json:"config"`     // JSON configuration
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ChartQueryRequest represents a chart query request
type ChartQueryRequest struct {
	DatasetID  int            `json:"dataset_id"`
	ChartType  string         `json:"chart_type"`
	Dims       []string       `json:"dims"`
	Metrics    []MetricConfig `json:"metrics"`
	Filters    []Filter       `json:"filters"`
	Pagination *Pagination    `json:"pagination"`
	Sort       *SortConfig    `json:"sort"`
}

// MetricConfig represents a metric aggregation configuration
type MetricConfig struct {
	Field string `json:"field"`
	Agg   string `json:"agg"` // "sum", "avg", "count", "max", "min"
	Alias string `json:"alias,omitempty"`
}

// Pagination represents pagination configuration
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// ChartDataResult represents chart data result
type ChartDataResult struct {
	Data      interface{} `json:"data"`
	SelectSQL string      `json:"select_sql,omitempty"`
	CountSQL  string      `json:"count_sql,omitempty"`
}
