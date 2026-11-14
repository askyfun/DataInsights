package entity

// Chart represents a visualization chart configuration
type Chart struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	DatasetID int    `json:"dataset_id"`
	ChartType string `json:"chart_type"` // "line", "bar", "pie"
	Config    string `json:"config"`     // JSON configuration
	// OwnerID 是 R-95（#220）的归属注入位：handler 从已认证 principal 取值塞进
	// 来，service 落库；匿名请求保持 0（由 service 换算成 SystemOwnerID）。
	// json:"-" 不上 wire、不参与请求绑定（chartCreateIn 因此转为有意裁剪档）。
	OwnerID   int    `json:"-"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// OwnerName 是读出口的创建人用户名（列表回填，可空）；单体 GET/POST/PUT 响应
	// 目前不回填，序列化为 null。
	OwnerName *string `json:"owner_name"`
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

// ChartFieldSelection 是观看侧对一块动态槽位的本次选择（issue #134）：把某个
// binding 的生效列换成 bindings[].candidates 里的另一列。它只作用于本次取数，
// 绝不写回 bi_chart.config —— 图表的默认选中项仍是 binding.fieldId。
type ChartFieldSelection struct {
	BindingID string `json:"bindingId"`
	FieldID   string `json:"fieldId"`
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
	Data        interface{}            `json:"data"`
	SelectSQL   string                 `json:"select_sql,omitempty"`
	CountSQL    string                 `json:"count_sql,omitempty"`
	Diagnostics *ChartQueryDiagnostics `json:"diagnostics,omitempty"`
}

// ChartQueryDiagnostics 是一次图表查询的执行元信息（R-20 / issue #182）。
// 只读诊断口径，不携带任何可执行入口（路线图 N2：不做自由 SQL 工作台）。
type ChartQueryDiagnostics struct {
	// DatasourceType 是数据源记录的原始类型（postgresql/mysql/clickhouse/starrocks）；
	// Dialect 是 SQL 生成实际使用的方言（StarRocks 走 MySQL 方言）。
	DatasourceType string `json:"datasource_type"`
	Dialect        string `json:"dialect"`
	DurationMs     int64  `json:"duration_ms"`
	// Capabilities 只在本次执行真的读过方言能力时给出（pivot 选择分组集 builder、
	// boxplot / percentile 前置门）；nil 表示本次图型未触发能力门控。
	Capabilities *ChartQueryCapabilities `json:"capabilities,omitempty"`
}

// ChartQueryCapabilities 是诊断面板显示的能力门控快照。
type ChartQueryCapabilities struct {
	SupportsGroupingSets    bool   `json:"supports_grouping_sets"`
	SupportsPercentileCont  bool   `json:"supports_percentile_cont"`
	SupportsWindowFunctions bool   `json:"supports_window_functions"`
	PercentileStrategy      string `json:"percentile_strategy"`
}
