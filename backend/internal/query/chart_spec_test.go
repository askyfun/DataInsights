package query

import (
	"encoding/json"
	"testing"

	"data-insights/internal/domain/entity"
)

func TestChartSpec_Serialization(t *testing.T) {
	spec := &ChartSpec{
		ChartType: ChartTypeLine,
		DimensionGroups: []DimensionGroup{
			{
				Name:  "x_axis",
				Label: "维度",
				Fields: []DimensionField{
					{Field: "created_at", Granularity: "month"},
				},
			},
		},
		MetricGroups: []MetricGroup{
			{
				Name:  "values",
				Label: "指标",
				Fields: []MetricField{
					{Field: "amount", Agg: AggSum, Alias: "total", Unit: "元"},
				},
			},
		},
		Style:        map[string]any{"color": "#1890ff"},
		QueryOptions: map[string]any{"limit": 100},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Failed to marshal ChartSpec: %v", err)
	}

	var roundtrip ChartSpec
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("Failed to unmarshal ChartSpec: %v", err)
	}

	if roundtrip.ChartType != spec.ChartType {
		t.Errorf("ChartType mismatch: %s vs %s", roundtrip.ChartType, spec.ChartType)
	}
	if len(roundtrip.DimensionGroups) != 1 {
		t.Fatalf("Expected 1 dim group after roundtrip, got %d", len(roundtrip.DimensionGroups))
	}
	if roundtrip.DimensionGroups[0].Fields[0].Granularity != "month" {
		t.Errorf("Expected granularity 'month', got %q", roundtrip.DimensionGroups[0].Fields[0].Granularity)
	}
	if roundtrip.MetricGroups[0].Fields[0].Unit != "元" {
		t.Errorf("Expected unit '元', got %q", roundtrip.MetricGroups[0].Fields[0].Unit)
	}
}

// TestAdapterCompatibility 验证新旧路径产生相同 SQL。
// 旧路径: Request → BunQueryBuilder.Build() → SQL
// 新路径: Request → QuerySpecFromRequest → QuerySpecToBuildArgs → BunQueryBuilder.Build() → SQL
func TestAdapterCompatibility(t *testing.T) {
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeTable,
		Dims:      []string{"status", "city"},
		Metrics: []MetricConfig{
			{Field: "amount", Agg: AggSum, Alias: "total_amount"},
			{Field: "id", Agg: AggCount},
		},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
			{Field: "amount", Op: FilterGt, Value: 100, Logic: "and"},
		},
		Sort:       &SortConfig{Field: "total_amount", Order: "desc"},
		Pagination: &Pagination{Page: 2, PageSize: 20},
	}

	// 旧路径
	qbOld := NewBunQueryBuilder()
	astOld := qbOld.Build(
		"test_table",
		SourceTypeTable,
		req.Dims,
		req.Metrics,
		req.Filters,
		req.Sort,
		req.Pagination,
	)
	sqlOld, _ := qbOld.BuildSelectQuery(astOld)

	// 新路径
	spec := QuerySpecFromRequest(req)
	dims, metrics, filters := QuerySpecToBuildArgs(spec)
	qbNew := NewBunQueryBuilder()
	astNew := qbNew.Build(
		"test_table",
		SourceTypeTable,
		dims,
		metrics,
		filters,
		spec.Sort,
		spec.Pagination,
	)
	sqlNew, _ := qbNew.BuildSelectQuery(astNew)

	if sqlOld != sqlNew {
		t.Errorf("SQL mismatch between old and new paths:\nOld: %s\nNew: %s", sqlOld, sqlNew)
	}
}

func TestQuerySpecFromRequest_Basic(t *testing.T) {
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeTable,
		Dims:      []string{"status"},
		Metrics: []MetricConfig{
			{Field: "amount", Agg: AggSum, Alias: "total"},
		},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
		},
		Sort:       &SortConfig{Field: "total", Order: "desc"},
		Pagination: &Pagination{Page: 2, PageSize: 20},
	}

	spec := QuerySpecFromRequest(req)

	if len(spec.Dimensions) != 1 || spec.Dimensions[0].Field != "status" {
		t.Errorf("Expected dims [status], got %v", spec.Dimensions)
	}
	if len(spec.Metrics) != 1 || spec.Metrics[0].Field != "amount" || spec.Metrics[0].Agg != AggSum {
		t.Errorf("Expected metrics [{amount sum}], got %v", spec.Metrics)
	}
	if len(spec.Filters) != 1 {
		t.Errorf("Expected 1 filter, got %d", len(spec.Filters))
	}
	if spec.Sort == nil || spec.Sort.Field != "total" {
		t.Errorf("Expected sort {total desc}, got %v", spec.Sort)
	}
	if spec.Pagination == nil || spec.Pagination.Page != 2 {
		t.Errorf("Expected pagination page 2, got %v", spec.Pagination)
	}
}

func TestQueryPlanner_PlanMatchesAdapter(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "status"},
			{Field: "city"},
		},
		Metrics: []MetricExpr2{
			{Field: "amount", Agg: AggSum, Alias: "total_amount"},
			{Field: "id", Agg: AggCount, Alias: "count"},
		},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "active"},
			{Field: "amount", Op: FilterGt, Value: 100, Logic: "and"},
		},
		Sort:       &SortConfig{Field: "total_amount", Order: "desc"},
		Pagination: &Pagination{Page: 2, PageSize: 20},
		Limit:      100,
	}

	expectedDims, expectedMetrics, expectedFilters := QuerySpecToBuildArgs(spec)
	planned := NewQueryPlanner().Plan(spec)

	if planned == nil {
		t.Fatal("expected planned query, got nil")
	}
	if len(planned.Dims) != len(expectedDims) || planned.Dims[0] != expectedDims[0] || planned.Dims[1] != expectedDims[1] {
		t.Fatalf("expected dims %v, got %v", expectedDims, planned.Dims)
	}
	if len(planned.Metrics) != len(expectedMetrics) || planned.Metrics[0].Alias != expectedMetrics[0].Alias || planned.Metrics[1].Field != expectedMetrics[1].Field {
		t.Fatalf("expected metrics %v, got %v", expectedMetrics, planned.Metrics)
	}
	if len(planned.Filters) != len(expectedFilters) || planned.Filters[0].Field != expectedFilters[0].Field || planned.Filters[1].Logic != expectedFilters[1].Logic {
		t.Fatalf("expected filters %v, got %v", expectedFilters, planned.Filters)
	}
	if planned.Sort == nil || planned.Sort.Field != "total_amount" || planned.Sort.Order != "desc" {
		t.Fatalf("expected sort total_amount desc, got %v", planned.Sort)
	}
	if planned.Pagination == nil || planned.Pagination.Page != 2 || planned.Pagination.PageSize != 20 {
		t.Fatalf("expected pagination {2,20}, got %v", planned.Pagination)
	}
	if planned.Limit != 100 {
		t.Fatalf("expected limit 100, got %d", planned.Limit)
	}
}

func TestQueryPlanner_PlanNilSpec(t *testing.T) {
	planned := NewQueryPlanner().Plan(nil)

	if planned == nil {
		t.Fatal("expected planned query, got nil")
	}
	if len(planned.Dims) != 0 || len(planned.Metrics) != 0 || len(planned.Filters) != 0 {
		t.Fatalf("expected empty planned query, got %+v", planned)
	}
	if planned.Sort != nil || planned.Pagination != nil || planned.Limit != 0 {
		t.Fatalf("expected nil sort/pagination and zero limit, got %+v", planned)
	}
}

func TestQueryPlanner_PlanAST_PreservesStructuredSemantics(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "created_at", Label: "日期", Granularity: "day"},
			{Field: "region", Label: "地区"},
		},
		Metrics: []MetricExpr2{
			{Field: "amount", Agg: AggSum, Alias: "total_amount", Unit: "CNY", Format: "currency"},
		},
		Filters: []FilterConfig{
			{Field: "region", Op: FilterEq, Value: "华东"},
		},
		Sort:       &SortConfig{Field: "total_amount", Order: "desc"},
		Pagination: &Pagination{Page: 1, PageSize: 50},
		Limit:      10,
	}

	ast := NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)

	if ast == nil {
		t.Fatal("expected ast, got nil")
	}
	if ast.Source != "orders" || ast.SourceType != SourceTypeTable {
		t.Fatalf("unexpected source info: %+v", ast)
	}
	if len(ast.DimensionExprs) != 2 {
		t.Fatalf("expected 2 dimension exprs, got %d", len(ast.DimensionExprs))
	}
	if ast.DimensionExprs[0].Field != "created_at" || ast.DimensionExprs[0].Granularity != "day" || ast.DimensionExprs[0].Alias != "created_at_day" {
		t.Fatalf("unexpected first dimension expr: %+v", ast.DimensionExprs[0])
	}
	if ast.DimensionExprs[1].Field != "region" || ast.DimensionExprs[1].Label != "地区" {
		t.Fatalf("unexpected second dimension expr: %+v", ast.DimensionExprs[1])
	}
	if len(ast.MetricExprs) != 1 {
		t.Fatalf("expected 1 metric expr, got %d", len(ast.MetricExprs))
	}
	if ast.MetricExprs[0].Field != "amount" || ast.MetricExprs[0].Unit != "CNY" || ast.MetricExprs[0].Format != "currency" {
		t.Fatalf("unexpected metric expr: %+v", ast.MetricExprs[0])
	}
	if ast.Limit != 10 {
		t.Fatalf("expected limit 10, got %d", ast.Limit)
	}
	if len(ast.Dimensions) != 2 || ast.Dimensions[0] != "created_at_day" || ast.Dimensions[1] != "region" {
		t.Fatalf("unexpected flattened dimensions: %v", ast.Dimensions)
	}
	if len(ast.Metrics) != 1 || ast.Metrics[0].Alias != "total_amount" {
		t.Fatalf("unexpected flattened metrics: %+v", ast.Metrics)
	}
	if ast.Sort == nil || ast.Sort.Field != "total_amount" {
		t.Fatalf("unexpected sort: %+v", ast.Sort)
	}
}

func TestQueryPlanner_PlanAST_MarksAggregatedColumnMappings(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{{Field: "project_id"}},
		Metrics: []MetricExpr2{{Field: "cnt", Agg: AggSum, Alias: "cnt"}},
	}

	ast := NewQueryPlanner().PlanAST("test_table", SourceTypeTable, spec)
	ast.ApplyColumnMappings(map[string]string{"cnt": "count(*)"})

	sql, _ := NewBunSQLBuilder(DialectMySQL).BuildSelect(ast)

	expected := "SELECT project_id, count(*) AS `cnt` FROM test_table GROUP BY project_id"
	if sql != expected {
		t.Fatalf("expected SQL %q, got %q", expected, sql)
	}
	if len(ast.Metrics) != 1 || !ast.Metrics[0].IsAgg {
		t.Fatalf("expected planned metric to be marked aggregated, got %+v", ast.Metrics)
	}
}

// TestChartSpecFromRequestV2_PreservesSlotsAndBindings 验证 v2 适配器原样映射
// 请求携带的槽位组名与 binding_id，不做任何"猜组名"降级（对比 v1 适配器按
// chartType 生成默认组名）。
func TestChartSpecFromRequestV2_PreservesSlotsAndBindings(t *testing.T) {
	req := &entity.ChartQueryRequest{
		DatasetID:   1,
		ChartType:   "bar",
		SpecVersion: intPtr(2),
		DimensionGroups: []entity.DimensionGroupIn{
			{Name: "x_axis", Label: "X 轴", Fields: []entity.DimensionFieldIn{
				{Field: "region", Label: "地区", BindingID: "b-0"},
			}},
			{Name: "color_group", Label: "颜色分组", Fields: []entity.DimensionFieldIn{
				{Field: "product", BindingID: "b-1"},
			}},
		},
		MetricGroups: []entity.MetricGroupIn{
			{Name: "values", Label: "指标", Fields: []entity.MetricFieldIn{
				{Field: "amount", Agg: "sum", Alias: "销售额", Unit: "元", Format: "currency", BindingID: "b-2"},
			}},
		},
	}

	spec := ChartSpecFromRequestV2(req)

	if spec.ChartType != ChartTypeBar {
		t.Fatalf("expected chart type bar, got %q", spec.ChartType)
	}
	if len(spec.DimensionGroups) != 2 {
		t.Fatalf("expected 2 dimension groups, got %d", len(spec.DimensionGroups))
	}
	if spec.DimensionGroups[0].Name != "x_axis" || spec.DimensionGroups[1].Name != "color_group" {
		t.Fatalf("expected slot names [x_axis color_group], got [%s %s]",
			spec.DimensionGroups[0].Name, spec.DimensionGroups[1].Name)
	}
	if spec.DimensionGroups[0].Fields[0].Field != "region" ||
		spec.DimensionGroups[0].Fields[0].Label != "地区" ||
		spec.DimensionGroups[0].Fields[0].BindingID != "b-0" {
		t.Fatalf("unexpected dim field[0]: %+v", spec.DimensionGroups[0].Fields[0])
	}
	if spec.DimensionGroups[1].Fields[0].Field != "product" || spec.DimensionGroups[1].Fields[0].BindingID != "b-1" {
		t.Fatalf("unexpected dim field[1]: %+v", spec.DimensionGroups[1].Fields[0])
	}
	if len(spec.MetricGroups) != 1 || spec.MetricGroups[0].Name != "values" {
		t.Fatalf("expected metric slot values, got %+v", spec.MetricGroups)
	}
	mf := spec.MetricGroups[0].Fields[0]
	if mf.Field != "amount" || mf.Agg != AggSum || mf.Alias != "销售额" ||
		mf.Unit != "元" || mf.Format != "currency" || mf.BindingID != "b-2" {
		t.Fatalf("unexpected metric field: %+v", mf)
	}
}

// TestChartSpecFromRequestV2_EmptyGroupsStayEmpty 验证空组不被降级填充默认组。
func TestChartSpecFromRequestV2_EmptyGroupsStayEmpty(t *testing.T) {
	req := &entity.ChartQueryRequest{
		DatasetID:   1,
		ChartType:   "table",
		SpecVersion: intPtr(2),
	}

	spec := ChartSpecFromRequestV2(req)

	if len(spec.DimensionGroups) != 0 || len(spec.MetricGroups) != 0 {
		t.Fatalf("expected no groups, got dims=%v metrics=%v", spec.DimensionGroups, spec.MetricGroups)
	}
}

// TestQuerySpecFromChartSpecV2_KeepsGroupAndBinding 验证展平为 QuerySpec 时
// 每个表达式保留来源槽位名与 binding_id。
func TestQuerySpecFromChartSpecV2_KeepsGroupAndBinding(t *testing.T) {
	spec := &ChartSpec{
		ChartType: ChartTypeBar,
		DimensionGroups: []DimensionGroup{
			{Name: "x_axis", Fields: []DimensionField{{Field: "region", BindingID: "b-0"}}},
			{Name: "color_group", Fields: []DimensionField{{Field: "product", Granularity: "day", BindingID: "b-1"}}},
		},
		MetricGroups: []MetricGroup{
			{Name: "values", Fields: []MetricField{{Field: "amount", Agg: AggSum, Alias: "销售额", BindingID: "b-2"}}},
		},
	}

	qs := QuerySpecFromChartSpecV2(spec)

	if len(qs.Dimensions) != 2 {
		t.Fatalf("expected 2 dimensions, got %d", len(qs.Dimensions))
	}
	if qs.Dimensions[0].Field != "region" || qs.Dimensions[0].GroupName != "x_axis" || qs.Dimensions[0].BindingID != "b-0" {
		t.Fatalf("unexpected dimension[0]: %+v", qs.Dimensions[0])
	}
	if qs.Dimensions[1].GroupName != "color_group" || qs.Dimensions[1].BindingID != "b-1" || qs.Dimensions[1].Granularity != "day" {
		t.Fatalf("unexpected dimension[1]: %+v", qs.Dimensions[1])
	}
	if len(qs.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(qs.Metrics))
	}
	if qs.Metrics[0].Field != "amount" || qs.Metrics[0].Agg != AggSum || qs.Metrics[0].Alias != "销售额" ||
		qs.Metrics[0].GroupName != "values" || qs.Metrics[0].BindingID != "b-2" {
		t.Fatalf("unexpected metric[0]: %+v", qs.Metrics[0])
	}
}

// TestQueryPlanner_PlanAST_PreservesGroupAndBinding 验证 PlanAST 把 QuerySpec 上的
// 槽位名/binding_id 传递到 DimensionExprAST/MetricPlanExpr。
func TestQueryPlanner_PlanAST_PreservesGroupAndBinding(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "region", GroupName: "x_axis", BindingID: "b-0"},
			{Field: "product", GroupName: "color_group", BindingID: "b-1"},
		},
		Metrics: []MetricExpr2{
			{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values", BindingID: "b-2"},
		},
	}

	ast := NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)

	if len(ast.DimensionExprs) != 2 {
		t.Fatalf("expected 2 dimension exprs, got %d", len(ast.DimensionExprs))
	}
	if ast.DimensionExprs[0].GroupName != "x_axis" || ast.DimensionExprs[0].BindingID != "b-0" {
		t.Fatalf("unexpected dimension expr[0]: %+v", ast.DimensionExprs[0])
	}
	if ast.DimensionExprs[1].GroupName != "color_group" || ast.DimensionExprs[1].BindingID != "b-1" {
		t.Fatalf("unexpected dimension expr[1]: %+v", ast.DimensionExprs[1])
	}
	if len(ast.MetricExprs) != 1 {
		t.Fatalf("expected 1 metric expr, got %d", len(ast.MetricExprs))
	}
	if ast.MetricExprs[0].GroupName != "values" || ast.MetricExprs[0].BindingID != "b-2" {
		t.Fatalf("unexpected metric expr[0]: %+v", ast.MetricExprs[0])
	}
}

func intPtr(v int) *int { return &v }
