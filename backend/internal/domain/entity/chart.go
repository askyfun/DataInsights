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

// ChartQueryRequest represents a chart query request.
// v1（平铺协议）与 v2（槽位协议）共用同一超集结构：SpecVersion 缺失或 !=2 时
// 消费 Dims/Metrics；SpecVersion=2 时消费 DimensionGroups/MetricGroups
// （槽位名与 binding_id 保留，见 query.ChartSpecFromRequestV2）。
type ChartQueryRequest struct {
	DatasetID  int            `json:"dataset_id"`
	ChartType  string         `json:"chart_type"`
	Dims       []string       `json:"dims"`
	Metrics    []MetricConfig `json:"metrics"`
	Filters    []Filter       `json:"filters"`
	Pagination *Pagination    `json:"pagination"`
	Sort       *SortConfig    `json:"sort"`

	SpecVersion     *int               `json:"spec_version,omitempty"`
	DimensionGroups []DimensionGroupIn `json:"dimension_groups,omitempty"`
	MetricGroups    []MetricGroupIn    `json:"metric_groups,omitempty"`

	// QueryOptions 查询选项扩展袋（R-57）：histogram 的 bin_count/bin_width 经此
	// 传递，由 executor 直接从请求结构体读取（不进入 QuerySpec/QueryAST/planner）。
	QueryOptions map[string]any `json:"query_options,omitempty"`
}

// DimensionGroupIn v2 协议的维度槽位组（json tag 与 query.DimensionGroup 一致）。
// 注意与 FieldGroup（dataset.go，持久化 config 解析用）是完全不同的结构。
type DimensionGroupIn struct {
	Name   string             `json:"name"`
	Label  string             `json:"label"`
	Fields []DimensionFieldIn `json:"fields"`
}

// DimensionFieldIn v2 协议的维度字段绑定（json tag 与 query.DimensionField 一致）。
type DimensionFieldIn struct {
	Field       string `json:"field"`
	Label       string `json:"label,omitempty"`
	Granularity string `json:"granularity,omitempty"`
	BindingID   string `json:"binding_id,omitempty"`
}

// MetricGroupIn v2 协议的指标槽位组（json tag 与 query.MetricGroup 一致）。
type MetricGroupIn struct {
	Name   string          `json:"name"`
	Label  string          `json:"label"`
	Fields []MetricFieldIn `json:"fields"`
}

// MetricFieldIn v2 协议的指标字段绑定（json tag 与 query.MetricField 一致）。
type MetricFieldIn struct {
	Field     string `json:"field"`
	Label     string `json:"label,omitempty"`
	Agg       string `json:"agg"`
	Alias     string `json:"alias,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Format    string `json:"format,omitempty"`
	BindingID string `json:"binding_id,omitempty"`
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
