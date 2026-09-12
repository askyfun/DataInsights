package query

// ChartType 可视化图表类型
type ChartType string

const (
	ChartTypeTable   ChartType = "table"
	ChartTypeBar     ChartType = "bar"
	ChartTypeLine    ChartType = "line"
	ChartTypePie     ChartType = "pie"
	ChartTypeArea    ChartType = "area"
	ChartTypeScatter ChartType = "scatter"
	ChartTypePivot   ChartType = "pivot"
)

// AggregationType 聚合函数类型
type AggregationType string

const (
	AggSum   AggregationType = "sum"
	AggAvg   AggregationType = "avg"
	AggCount AggregationType = "count"
	AggMax   AggregationType = "max"
	AggMin   AggregationType = "min"
)

// FilterOperator 过滤条件操作符
type FilterOperator string

const (
	FilterEq        FilterOperator = "eq"
	FilterNeq       FilterOperator = "neq"
	FilterGt        FilterOperator = "gt"
	FilterGte       FilterOperator = "gte"
	FilterLt        FilterOperator = "lt"
	FilterLte       FilterOperator = "lte"
	FilterLike      FilterOperator = "like"
	FilterIn        FilterOperator = "in"
	FilterBetween   FilterOperator = "between"
	FilterIsNull    FilterOperator = "isNull"
	FilterIsNotNull FilterOperator = "isNotNull"
)

// MetricConfig 指标配置
type MetricConfig struct {
	Field string          `json:"field"`
	Agg   AggregationType `json:"agg"`
	Alias string          `json:"alias,omitempty"`
}

// FilterConfig 过滤条件配置
type FilterConfig struct {
	Field    string         `json:"field"`
	Op       FilterOperator `json:"op"`
	Value    interface{}    `json:"value"`
	ValueEnd interface{}    `json:"value_end,omitempty"`
	Logic    string         `json:"logic"`
}

// Pagination 分页配置
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// SortConfig 排序配置
type SortConfig struct {
	Field string `json:"field"`
	Order string `json:"order"` // asc | desc
}

// ChartQueryRequest 图表查询请求
type ChartQueryRequest struct {
	DatasetID  int            `json:"dataset_id"`
	ChartType  ChartType      `json:"chart_type"`
	Dims       []string       `json:"dims"`
	Metrics    []MetricConfig `json:"metrics"`
	Filters    []FilterConfig `json:"filters"`
	Pagination *Pagination    `json:"pagination,omitempty"`
	Sort       *SortConfig    `json:"sort,omitempty"`
	PlannedAST *QueryAST      `json:"-"`
}

// ChartQueryResponse 图表查询响应
type ChartQueryResponse interface{}

// TableResponse Table 图表响应
type TableResponse struct {
	Columns    []string         `json:"columns"`
	Data       []map[string]any `json:"data"`
	Pagination TablePagination  `json:"pagination"`
}

// TablePagination Table 分页信息
type TablePagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// PieResponse Pie 图表响应
type PieResponse struct {
	Data  []PieDataItem `json:"data"`
	Other *PieDataItem  `json:"other,omitempty"`
}

// PieDataItem Pie 数据项
type PieDataItem struct {
	Name       string  `json:"name"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
}

// AxisResponse 坐标轴图表响应 (Bar, Line, Area)
type AxisResponse struct {
	XAxis  []string     `json:"x_axis"`
	Series []AxisSeries `json:"series"`
}

// AxisSeries 系列数据
type AxisSeries struct {
	Name string `json:"name"`
	Data []any  `json:"data"`
}

// ScatterResponse Scatter 图表响应
type ScatterResponse struct {
	Data [][]float64 `json:"data"`
}

// PivotResponse Pivot 图表响应
type PivotResponse struct {
	Columns []string         `json:"columns"`
	Data    []map[string]any `json:"data"`
}

// ResolveAlias 解析字段别名
func (m *MetricConfig) ResolveAlias() string {
	if m.Alias != "" {
		return m.Alias
	}
	return m.Field
}

// GetAggFunc 获取聚合函数名
func (a AggregationType) GetAggFunc() string {
	switch a {
	case AggSum:
		return "SUM"
	case AggAvg:
		return "AVG"
	case AggCount:
		return "COUNT"
	case AggMax:
		return "MAX"
	case AggMin:
		return "MIN"
	default:
		return "SUM"
	}
}

// ToString 将 FilterOperator 转换为 SQL 操作符
func (op FilterOperator) ToString() string {
	switch op {
	case FilterEq:
		return "="
	case FilterNeq:
		return "<>"
	case FilterGt:
		return ">"
	case FilterGte:
		return ">="
	case FilterLt:
		return "<"
	case FilterLte:
		return "<="
	case FilterLike:
		return "LIKE"
	case FilterIn:
		return "IN"
	case FilterBetween:
		return "BETWEEN"
	case FilterIsNull:
		return "IS NULL"
	case FilterIsNotNull:
		return "IS NOT NULL"
	default:
		return "="
	}
}
