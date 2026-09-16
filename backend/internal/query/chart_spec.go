package query

import "dataray/internal/domain/entity"

// ChartSpec 图表语义规格，表达一个图表实例绑定了什么字段和配置。
// 属于 Chart 语义层，不直接生成 SQL。
type ChartSpec struct {
	ChartType       ChartType        `json:"chart_type"`
	DimensionGroups []DimensionGroup `json:"dimension_groups"`
	MetricGroups    []MetricGroup    `json:"metric_groups"`
	Style           map[string]any   `json:"style,omitempty"`
	QueryOptions    map[string]any   `json:"query_options,omitempty"`
}

// DimensionGroup 维度组，例如 "x_axis"、"rows"、"columns"、"category"
type DimensionGroup struct {
	Name   string           `json:"name"`
	Label  string           `json:"label"`
	Fields []DimensionField `json:"fields"`
}

// DimensionField 维度字段绑定
type DimensionField struct {
	Field       string `json:"field"`
	Label       string `json:"label,omitempty"`
	Granularity string `json:"granularity,omitempty"` // 时间粒度: day, week, month, etc.
	BindingID   string `json:"binding_id,omitempty"`  // 绑定实例唯一标识（v2 协议，对应前端 BindingInstance.bindingId）
}

// MetricGroup 指标组，例如 "values"、"primary_values"、"secondary_values"
type MetricGroup struct {
	Name   string        `json:"name"`
	Label  string        `json:"label"`
	Fields []MetricField `json:"fields"`
}

// MetricField 指标字段绑定
type MetricField struct {
	Field     string          `json:"field"`
	Label     string          `json:"label,omitempty"`
	Agg       AggregationType `json:"agg"`
	Alias     string          `json:"alias,omitempty"`
	Unit      string          `json:"unit,omitempty"`
	Format    string          `json:"format,omitempty"`
	BindingID string          `json:"binding_id,omitempty"` // 绑定实例唯一标识（v2 协议）
}

// QuerySpec 查询语义规格，表达"查什么"，不包含视觉语义。
// 属于 Query 语义层，由 ChartSpec 转换而来。
type QuerySpec struct {
	Dimensions []DimensionExpr `json:"dimensions"`
	Metrics    []MetricExpr2   `json:"metrics"`
	Filters    []FilterConfig  `json:"filters"`
	Sort       *SortConfig     `json:"sort,omitempty"`
	Pagination *Pagination     `json:"pagination,omitempty"`
	Limit      int             `json:"limit,omitempty"`
}

// DimensionExpr 结构化维度表达式
type DimensionExpr struct {
	Field       string `json:"field"`
	Label       string `json:"label,omitempty"`
	Granularity string `json:"granularity,omitempty"`
	GroupName   string `json:"group_name,omitempty"` // 来源槽位/组名（v2 协议；v1 路径为空）
	BindingID   string `json:"binding_id,omitempty"` // 来源绑定实例标识（v2 协议；v1 路径为空）
}

// MetricExpr2 结构化指标表达式（命名避免与 ast.go 中的 MetricExpr 冲突）
type MetricExpr2 struct {
	Field     string          `json:"field"`
	Agg       AggregationType `json:"agg"`
	Alias     string          `json:"alias,omitempty"`
	Unit      string          `json:"unit,omitempty"`
	Format    string          `json:"format,omitempty"`
	GroupName string          `json:"group_name,omitempty"` // 来源槽位/组名（v2 协议；v1 路径为空）
	BindingID string          `json:"binding_id,omitempty"` // 来源绑定实例标识（v2 协议；v1 路径为空）
}

// ChartSpecFromRequest 将旧的 ChartQueryRequest 转换为 ChartSpec（兼容 adapter）。
// 旧请求的 dims/metrics 映射为默认组。
func ChartSpecFromRequest(req *ChartQueryRequest) *ChartSpec {
	spec := &ChartSpec{
		ChartType:    req.ChartType,
		Style:        make(map[string]any),
		QueryOptions: make(map[string]any),
	}

	// 维度 → 默认维度组
	if len(req.Dims) > 0 {
		fields := make([]DimensionField, len(req.Dims))
		for i, d := range req.Dims {
			fields[i] = DimensionField{Field: d}
		}
		spec.DimensionGroups = []DimensionGroup{
			{Name: defaultDimGroupName(req.ChartType), Label: "维度", Fields: fields},
		}
	}

	// 指标 → 默认指标组
	if len(req.Metrics) > 0 {
		fields := make([]MetricField, len(req.Metrics))
		for i, m := range req.Metrics {
			fields[i] = MetricField{
				Field: m.Field,
				Agg:   m.Agg,
				Alias: m.Alias,
			}
		}
		spec.MetricGroups = []MetricGroup{
			{Name: defaultMetricGroupName(req.ChartType), Label: "指标", Fields: fields},
		}
	}

	return spec
}

// QuerySpecFromRequest 将旧的 ChartQueryRequest 转换为 QuerySpec（兼容 adapter）。
func QuerySpecFromRequest(req *ChartQueryRequest) *QuerySpec {
	spec := &QuerySpec{
		Filters:    req.Filters,
		Sort:       req.Sort,
		Pagination: req.Pagination,
	}

	for _, d := range req.Dims {
		spec.Dimensions = append(spec.Dimensions, DimensionExpr{Field: d})
	}

	for _, m := range req.Metrics {
		spec.Metrics = append(spec.Metrics, MetricExpr2{
			Field: m.Field,
			Agg:   m.Agg,
			Alias: m.Alias,
		})
	}

	return spec
}

// ChartSpecFromRequestV2 将 v2 协议请求（spec_version=2，携带显式槽位组）转换为
// ChartSpec。与 ChartSpecFromRequest（v1 适配器，按 chartType 猜默认组名）不同，
// 这里直接原样映射 req.DimensionGroups/MetricGroups 的组名（槽位名）与字段绑定
// （含 BindingID），不做任何降级猜测。
// 调用场景：service 层 executeQueryOnConn 判别 spec_version=2 后进入本适配器。
func ChartSpecFromRequestV2(req *entity.ChartQueryRequest) *ChartSpec {
	spec := &ChartSpec{
		ChartType:    ChartType(req.ChartType),
		Style:        make(map[string]any),
		QueryOptions: make(map[string]any),
	}

	for _, g := range req.DimensionGroups {
		group := DimensionGroup{Name: g.Name, Label: g.Label}
		for _, f := range g.Fields {
			group.Fields = append(group.Fields, DimensionField{
				Field:       f.Field,
				Label:       f.Label,
				Granularity: f.Granularity,
				BindingID:   f.BindingID,
			})
		}
		spec.DimensionGroups = append(spec.DimensionGroups, group)
	}

	for _, g := range req.MetricGroups {
		group := MetricGroup{Name: g.Name, Label: g.Label}
		for _, f := range g.Fields {
			group.Fields = append(group.Fields, MetricField{
				Field:     f.Field,
				Label:     f.Label,
				Agg:       AggregationType(f.Agg),
				Alias:     f.Alias,
				Unit:      f.Unit,
				Format:    f.Format,
				BindingID: f.BindingID,
			})
		}
		spec.MetricGroups = append(spec.MetricGroups, group)
	}

	return spec
}

// QuerySpecFromChartSpecV2 将 v2 ChartSpec 展平为 QuerySpec，同时把每个表达式
// 的来源槽位名（GroupName）与绑定实例标识（BindingID）保留在 DimensionExpr/
// MetricExpr2 上，供 QueryPlanner.PlanAST 传入 AST、后续 processor 按槽位消费。
// ChartSpec 不承载 filters/sort/pagination，调用方（service 层）需自行从请求补齐。
func QuerySpecFromChartSpecV2(spec *ChartSpec) *QuerySpec {
	qs := &QuerySpec{}
	if spec == nil {
		return qs
	}

	for _, g := range spec.DimensionGroups {
		for _, f := range g.Fields {
			qs.Dimensions = append(qs.Dimensions, DimensionExpr{
				Field:       f.Field,
				Label:       f.Label,
				Granularity: f.Granularity,
				GroupName:   g.Name,
				BindingID:   f.BindingID,
			})
		}
	}

	for _, g := range spec.MetricGroups {
		for _, f := range g.Fields {
			qs.Metrics = append(qs.Metrics, MetricExpr2{
				Field:     f.Field,
				Agg:       f.Agg,
				Alias:     f.Alias,
				Unit:      f.Unit,
				Format:    f.Format,
				GroupName: g.Name,
				BindingID: f.BindingID,
			})
		}
	}

	return qs
}

// QuerySpecToBuildArgs 将 QuerySpec 拆解为 BunQueryBuilder.Build() 所需的平铺参数。
// 用于兼容模式：新类型 → 旧 builder 路径。
func QuerySpecToBuildArgs(spec *QuerySpec) (dims []string, metrics []MetricConfig, filters []FilterConfig) {
	dims = make([]string, 0, len(spec.Dimensions))
	for _, d := range spec.Dimensions {
		dims = append(dims, d.Field)
	}

	metrics = make([]MetricConfig, 0, len(spec.Metrics))
	for _, m := range spec.Metrics {
		metrics = append(metrics, MetricConfig{
			Field: m.Field,
			Agg:   m.Agg,
			Alias: m.Alias,
		})
	}

	filters = spec.Filters
	if filters == nil {
		filters = []FilterConfig{}
	}

	return
}

// defaultDimGroupName 根据图表类型返回默认维度组名
func defaultDimGroupName(chartType ChartType) string {
	switch chartType {
	case ChartTypePivot:
		return "rows"
	case ChartTypePie:
		return "category"
	case ChartTypeScatter:
		return "dims"
	default:
		return "x_axis"
	}
}

// defaultMetricGroupName 根据图表类型返回默认指标组名
func defaultMetricGroupName(chartType ChartType) string {
	switch chartType {
	case ChartTypeScatter:
		return "values"
	default:
		return "values"
	}
}
