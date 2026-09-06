package query

import (
	"encoding/json"
	"testing"
)

func TestChartSpecFromRequest_BasicTable(t *testing.T) {
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
		},
		Pagination: &Pagination{Page: 1, PageSize: 10},
		Sort:       &SortConfig{Field: "total_amount", Order: "desc"},
	}

	spec := ChartSpecFromRequest(req)

	if spec.ChartType != ChartTypeTable {
		t.Errorf("Expected chart type %s, got %s", ChartTypeTable, spec.ChartType)
	}

	if len(spec.DimensionGroups) != 1 {
		t.Fatalf("Expected 1 dimension group, got %d", len(spec.DimensionGroups))
	}

	dg := spec.DimensionGroups[0]
	if dg.Name != "x_axis" {
		t.Errorf("Expected dim group name 'x_axis', got %q", dg.Name)
	}
	if len(dg.Fields) != 2 {
		t.Fatalf("Expected 2 dimension fields, got %d", len(dg.Fields))
	}
	if dg.Fields[0].Field != "status" || dg.Fields[1].Field != "city" {
		t.Errorf("Expected dim fields [status, city], got [%s, %s]", dg.Fields[0].Field, dg.Fields[1].Field)
	}

	if len(spec.MetricGroups) != 1 {
		t.Fatalf("Expected 1 metric group, got %d", len(spec.MetricGroups))
	}

	mg := spec.MetricGroups[0]
	if mg.Name != "values" {
		t.Errorf("Expected metric group name 'values', got %q", mg.Name)
	}
	if len(mg.Fields) != 2 {
		t.Fatalf("Expected 2 metric fields, got %d", len(mg.Fields))
	}
	if mg.Fields[0].Field != "amount" || mg.Fields[0].Agg != AggSum {
		t.Errorf("Expected metric field[0] = {amount, sum}, got {%s, %s}", mg.Fields[0].Field, mg.Fields[0].Agg)
	}
	if mg.Fields[1].Field != "id" || mg.Fields[1].Agg != AggCount {
		t.Errorf("Expected metric field[1] = {id, count}, got {%s, %s}", mg.Fields[1].Field, mg.Fields[1].Agg)
	}
}

func TestChartSpecFromRequest_PivotUsesRowsGroup(t *testing.T) {
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePivot,
		Dims:      []string{"region"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
	}

	spec := ChartSpecFromRequest(req)

	if spec.DimensionGroups[0].Name != "rows" {
		t.Errorf("Expected pivot dim group name 'rows', got %q", spec.DimensionGroups[0].Name)
	}
}

func TestChartSpecFromRequest_PieUsesCategoryGroup(t *testing.T) {
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePie,
		Dims:      []string{"category"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
	}

	spec := ChartSpecFromRequest(req)

	if spec.DimensionGroups[0].Name != "category" {
		t.Errorf("Expected pie dim group name 'category', got %q", spec.DimensionGroups[0].Name)
	}
}

func TestChartSpecFromRequest_EmptyDimsAndMetrics(t *testing.T) {
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeTable,
		Dims:      []string{},
		Metrics:   []MetricConfig{},
	}

	spec := ChartSpecFromRequest(req)

	if len(spec.DimensionGroups) != 0 {
		t.Errorf("Expected 0 dimension groups, got %d", len(spec.DimensionGroups))
	}
	if len(spec.MetricGroups) != 0 {
		t.Errorf("Expected 0 metric groups, got %d", len(spec.MetricGroups))
	}
}

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

	expected := "SELECT project_id, count(*) AS cnt FROM test_table GROUP BY project_id"
	if sql != expected {
		t.Fatalf("expected SQL %q, got %q", expected, sql)
	}
	if len(ast.Metrics) != 1 || !ast.Metrics[0].IsAgg {
		t.Fatalf("expected planned metric to be marked aggregated, got %+v", ast.Metrics)
	}
}
