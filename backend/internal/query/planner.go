package query

// PlannedQuery 代表 QueryPlanner 为旧执行链路产出的兼容结果。
// 调用场景：兼容期内由 QuerySpec 规划出旧 builder / executor 仍可消费的平铺参数。
type PlannedQuery struct {
	Dims       []string
	Metrics    []MetricConfig
	Filters    []FilterConfig
	Sort       *SortConfig
	Pagination *Pagination
	Limit      int
}

// QueryPlanner 负责将 QuerySpec 规划为当前执行链路可消费的查询参数。
// 当前仅支持基础维度、基础聚合、线性过滤、排序和分页能力。
type QueryPlanner struct{}

// NewQueryPlanner 创建最小 QueryPlanner。
func NewQueryPlanner() *QueryPlanner {
	return &QueryPlanner{}
}

// Plan 根据 QuerySpec 生成兼容旧执行链路的 PlannedQuery。
// 当前实现保持与 QuerySpecToBuildArgs 等价，为后续扩展更复杂规划能力预留入口。
func (p *QueryPlanner) Plan(spec *QuerySpec) *PlannedQuery {
	if spec == nil {
		return &PlannedQuery{
			Dims:    []string{},
			Metrics: []MetricConfig{},
			Filters: []FilterConfig{},
		}
	}

	dims, metrics, filters := QuerySpecToBuildArgs(spec)

	return &PlannedQuery{
		Dims:       dims,
		Metrics:    metrics,
		Filters:    filters,
		Sort:       spec.Sort,
		Pagination: spec.Pagination,
		Limit:      spec.Limit,
	}
}

// PlanAST 根据 QuerySpec 生成增强后的 QueryAST。
// 调用场景：P2 阶段开始让 QueryPlanner 直接产出数据库无关的结构化 AST，同时保留旧的平铺字段以兼容现有执行链路。
func (p *QueryPlanner) PlanAST(source string, sourceType SourceType, spec *QuerySpec) *QueryAST {
	if spec == nil {
		return &QueryAST{
			Source:         source,
			SourceType:     sourceType,
			Dimensions:     []string{},
			DimensionExprs: []DimensionExprAST{},
			Metrics:        []MetricExpr{},
			MetricExprs:    []MetricPlanExpr{},
			Filters:        []FilterExpr{},
		}
	}

	ast := &QueryAST{
		Source:         source,
		SourceType:     sourceType,
		Dimensions:     make([]string, 0, len(spec.Dimensions)),
		DimensionExprs: make([]DimensionExprAST, 0, len(spec.Dimensions)),
		Metrics:        make([]MetricExpr, 0, len(spec.Metrics)),
		MetricExprs:    make([]MetricPlanExpr, 0, len(spec.Metrics)),
		Filters:        make([]FilterExpr, 0, len(spec.Filters)),
		Pagination:     spec.Pagination,
		Limit:          spec.Limit,
		ColumnMappings:       map[string]string{},
		LegacyColumnMappings: map[string]string{},
		ColumnNames:          map[string]string{},
	}

	if spec.Sort != nil {
		ast.Sort = &SortExpr{
			Field:     spec.Sort.Field,
			FieldExpr: spec.Sort.Field,
			Order:     spec.Sort.Order,
		}
	}

	for _, dim := range spec.Dimensions {
		alias := resolveDimensionAlias(dim)
		ast.Dimensions = append(ast.Dimensions, alias)
		ast.DimensionExprs = append(ast.DimensionExprs, DimensionExprAST{
			Field:       dim.Field,
			FieldExpr:   dim.Field,
			Alias:       alias,
			Label:       dim.Label,
			Granularity: dim.Granularity,
			GroupName:   dim.GroupName,
			BindingID:   dim.BindingID,
		})
	}

	for _, metric := range spec.Metrics {
		alias := metric.Field
		if metric.Alias != "" {
			alias = metric.Alias
		}

		fieldExpr := metric.Field
		isAgg := isAggregateFunction(fieldExpr)
		if expr, ok := ast.ColumnMappings[metric.Field]; ok && expr != "" {
			fieldExpr = expr
			isAgg = isAggregateFunction(expr)
		}

		ast.Metrics = append(ast.Metrics, MetricExpr{
			Field:     metric.Field,
			FieldExpr: fieldExpr,
			Agg:       metric.Agg,
			Alias:     alias,
			IsAgg:     isAgg,
		})
		ast.MetricExprs = append(ast.MetricExprs, MetricPlanExpr{
			Field:     metric.Field,
			FieldExpr: fieldExpr,
			Agg:       metric.Agg,
			Alias:     alias,
			Unit:      metric.Unit,
			Format:    metric.Format,
			GroupName: metric.GroupName,
			BindingID: metric.BindingID,
		})
	}

	for _, filter := range spec.Filters {
		ast.Filters = append(ast.Filters, FilterExpr{
			Field:     filter.Field,
			FieldExpr: filter.Field,
			Op:        filter.Op,
			Value:     filter.Value,
			ValueEnd:  filter.ValueEnd,
			Logic:     filter.Logic,
		})
	}

	return ast
}

// resolveDimensionAlias 为结构化维度生成稳定别名。
// 调用场景：时间粒度维度在进入 SQL builder 前需要有可复用别名，普通维度保持字段名不变。
func resolveDimensionAlias(dim DimensionExpr) string {
	if dim.Granularity == "" {
		return dim.Field
	}
	return dim.Field + "_" + dim.Granularity
}
