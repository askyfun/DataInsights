package query

import (
	"strings"
	"testing"
)

// pivotPlanAST 走 service 层同款真实管道（QuerySpec → PlanAST）构造带槽位的 AST。
func pivotPlanAST(dimSpecs []DimensionExpr, metrics []MetricExpr2, filters []FilterConfig) *QueryAST {
	spec := &QuerySpec{
		Dimensions: dimSpecs,
		Metrics:    metrics,
		Filters:    filters,
	}
	return NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)
}

// TestBuildPivotGroupingSetsQuery_SingleRowCol 钉死单行维度+单列维度的完整 SQL 形状：
// SELECT（维度 + 聚合 + 单参 GROUPING 标记列）、参数化 WHERE、三个 GROUPING SETS 组合
// （明细/行小计/合计）、确定性 ORDER BY。
func TestBuildPivotGroupingSetsQuery_SingleRowCol(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)
	dims := []string{"region", "product"}
	rowDims, colDims, ok := resolvePivotSlots(dims, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, args := BuildPivotGroupingSetsQuery(DialectPostgreSQL, ast, rowDims, colDims)

	want := `SELECT region, product, SUM(amount) AS "total", ` +
		`GROUPING(region) AS "__pivot_row_grp_0", GROUPING(product) AS "__pivot_col_grp_0" ` +
		`FROM orders WHERE status = ? ` +
		`GROUP BY GROUPING SETS ((region, product), (region), ()) ` +
		`ORDER BY region, "__pivot_col_grp_0", product`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	// 过滤值必须走参数化 args，不得进入 SQL 文本
	if len(args) != 1 || args[0] != "ok" {
		t.Fatalf("expected args=[ok], got %v", args)
	}
	if strings.Contains(sql, "'ok'") {
		t.Fatalf("filter value leaked into SQL text: %s", sql)
	}
}

// TestBuildPivotGroupingSetsQuery_MultiDims 验证多行维度/多列维度时 GROUPING SETS
// 组合的自然扩展：2 行 + 2 列 → ((r1,r2,c1,c2), (r1,r2), ())，行小计保留全部行维度、
// 汇总掉全部列维度（不是逐维度 ROLLUP/CUBE）；标记列按维度数量扩展。
func TestBuildPivotGroupingSetsQuery_MultiDims(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "city", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
			{Field: "channel", GroupName: SlotColumns},
		},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
		nil,
	)
	dims := []string{"region", "city", "product", "channel"}
	rowDims, colDims, ok := resolvePivotSlots(dims, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, args := BuildPivotGroupingSetsQuery(DialectPostgreSQL, ast, rowDims, colDims)

	if !strings.Contains(sql, "GROUP BY GROUPING SETS ((region, city, product, channel), (region, city), ())") {
		t.Errorf("grouping sets combos not extended for multi-dims: %s", sql)
	}
	for _, want := range []string{
		`GROUPING(region) AS "__pivot_row_grp_0"`,
		`GROUPING(city) AS "__pivot_row_grp_1"`,
		`GROUPING(product) AS "__pivot_col_grp_0"`,
		`GROUPING(channel) AS "__pivot_col_grp_1"`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("expected SQL to contain %q, got: %s", want, sql)
		}
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

// TestBuildPivotGroupingSetsQuery_AvgAndCountDistinct 验证 avg/count_distinct 指标
// 在 GROUPING SETS 查询里以真实聚合函数下推给数据库重算（小计正确性的 SQL 侧证据，
// 与 Task 1-6 的 count_distinct SQL 字符串断言同一模式）。
func TestBuildPivotGroupingSetsQuery_AvgAndCountDistinct(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		[]MetricExpr2{
			{Field: "price", Agg: AggAvg, Alias: "avg_price", GroupName: "values"},
			{Field: "user_id", Agg: AggCountDistinct, Alias: "uniq_users", GroupName: "values"},
		},
		nil,
	)
	rowDims, colDims, ok := resolvePivotSlots([]string{"region", "product"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, _ := BuildPivotGroupingSetsQuery(DialectPostgreSQL, ast, rowDims, colDims)

	if !strings.Contains(sql, `AVG(price) AS "avg_price"`) {
		t.Errorf("expected AVG(price) pushed down to SQL, got: %s", sql)
	}
	if !strings.Contains(sql, `COUNT(DISTINCT user_id) AS "uniq_users"`) {
		t.Errorf("expected COUNT(DISTINCT user_id) pushed down to SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "GROUP BY GROUPING SETS (") {
		t.Errorf("expected GROUP BY GROUPING SETS syntax so DB recomputes aggregates per level, got: %s", sql)
	}
}

// TestBuildPivotGroupingSetsQuery_GranularityDim 验证时间粒度维度在 SELECT/GROUPING/
// GROUPING SETS/ORDER BY 里使用同一致表达式（GROUPING 参数必须与分组表达式逐字一致）。
func TestBuildPivotGroupingSetsQuery_GranularityDim(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "created_at", Granularity: "day", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
		nil,
	)
	rowDims, colDims, ok := resolvePivotSlots([]string{"created_at", "product"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, _ := BuildPivotGroupingSetsQuery(DialectPostgreSQL, ast, rowDims, colDims)

	want := `SELECT DATE_TRUNC('day', created_at) AS "created_at_day", product, SUM(amount) AS "total", ` +
		`GROUPING(DATE_TRUNC('day', created_at)) AS "__pivot_row_grp_0", GROUPING(product) AS "__pivot_col_grp_0" ` +
		`FROM orders ` +
		`GROUP BY GROUPING SETS ((DATE_TRUNC('day', created_at), product), (DATE_TRUNC('day', created_at)), ()) ` +
		`ORDER BY DATE_TRUNC('day', created_at), "__pivot_col_grp_0", product`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
}

// TestBuildPivotGroupingSetsQuery_SQLSource 验证 SQL 型数据集源被包裹为子查询。
func TestBuildPivotGroupingSetsQuery_SQLSource(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		Metrics: []MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
	}
	ast := NewQueryPlanner().PlanAST("SELECT * FROM orders WHERE year = 2026", SourceTypeSQL, spec)
	rowDims, colDims, ok := resolvePivotSlots([]string{"region", "product"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, _ := BuildPivotGroupingSetsQuery(DialectPostgreSQL, ast, rowDims, colDims)

	if !strings.Contains(sql, "FROM (SELECT * FROM orders WHERE year = 2026) AS _subq") {
		t.Errorf("expected SQL source wrapped as subquery, got: %s", sql)
	}
}
