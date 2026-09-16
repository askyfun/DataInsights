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

// TestBuildPivotUnionAllQuery_SingleRowCol 钉死单行维度+单列维度+单指标（SUM）的完整
// UNION ALL SQL 形状：三个分支（明细 GROUP BY rows+cols / 行小计 GROUP BY rows /
// 合计无 GROUP BY）、标记列字面常量（0/0、0/1、1/1）、被汇总掉维度的 NULL 占位、
// 每分支各一份参数化 WHERE、末尾引用输出列别名的 ORDER BY。
func TestBuildPivotUnionAllQuery_SingleRowCol(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)
	rowDims, colDims, ok := resolvePivotSlots([]string{"region", "product"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, args := BuildPivotUnionAllQuery(DialectPostgreSQL, ast, rowDims, colDims)

	want := `SELECT region, product, SUM(amount) AS "total", 0 AS "__pivot_row_grp_0", 0 AS "__pivot_col_grp_0" ` +
		`FROM orders WHERE status = ? GROUP BY region, product` +
		` UNION ALL ` +
		`SELECT region, NULL AS "product", SUM(amount) AS "total", 0 AS "__pivot_row_grp_0", 1 AS "__pivot_col_grp_0" ` +
		`FROM orders WHERE status = ? GROUP BY region` +
		` UNION ALL ` +
		`SELECT NULL AS "region", NULL AS "product", SUM(amount) AS "total", 1 AS "__pivot_row_grp_0", 1 AS "__pivot_col_grp_0" ` +
		`FROM orders WHERE status = ?` +
		` ORDER BY "region", "__pivot_col_grp_0", "product"`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	// WHERE + args 出现三份（每分支一份），顺序与 ? 占位一一对应；过滤值不得进入 SQL 文本。
	if len(args) != 3 || args[0] != "ok" || args[1] != "ok" || args[2] != "ok" {
		t.Fatalf("expected args=[ok ok ok] (one per UNION branch), got %v", args)
	}
	if strings.Contains(sql, "'ok'") {
		t.Fatalf("filter value leaked into SQL text: %s", sql)
	}
}

// TestBuildPivotUnionAllQuery_MultiDims 验证多行/多列维度时 UNION ALL 分支的自然扩展：
// 标记列扩展成 R+C 个、NULL 占位数量正确、三分支 GROUP BY 元组正确
// （分支1 (r1,r2,c1,c2)、分支2 (r1,r2)、分支3 无 GROUP BY）。
func TestBuildPivotUnionAllQuery_MultiDims(t *testing.T) {
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
	rowDims, colDims, ok := resolvePivotSlots([]string{"region", "city", "product", "channel"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, args := BuildPivotUnionAllQuery(DialectPostgreSQL, ast, rowDims, colDims)

	want := `SELECT region, city, product, channel, SUM(amount) AS "total", ` +
		`0 AS "__pivot_row_grp_0", 0 AS "__pivot_row_grp_1", 0 AS "__pivot_col_grp_0", 0 AS "__pivot_col_grp_1" ` +
		`FROM orders GROUP BY region, city, product, channel` +
		` UNION ALL ` +
		`SELECT region, city, NULL AS "product", NULL AS "channel", SUM(amount) AS "total", ` +
		`0 AS "__pivot_row_grp_0", 0 AS "__pivot_row_grp_1", 1 AS "__pivot_col_grp_0", 1 AS "__pivot_col_grp_1" ` +
		`FROM orders GROUP BY region, city` +
		` UNION ALL ` +
		`SELECT NULL AS "region", NULL AS "city", NULL AS "product", NULL AS "channel", SUM(amount) AS "total", ` +
		`1 AS "__pivot_row_grp_0", 1 AS "__pivot_row_grp_1", 1 AS "__pivot_col_grp_0", 1 AS "__pivot_col_grp_1" ` +
		`FROM orders` +
		` ORDER BY "region", "city", "__pivot_col_grp_0", "__pivot_col_grp_1", "product", "channel"`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

// TestBuildPivotUnionAllQuery_AvgAndCountDistinct 验证 avg/count_distinct 指标在三个
// UNION 分支里都以真实聚合函数出现、且各分支带正确的 GROUP BY——小计/合计由数据库在
// 每个分组层级独立重算（R-53 正确性红线在 SQL 侧的钉子，Go 端不做二次聚合）。
func TestBuildPivotUnionAllQuery_AvgAndCountDistinct(t *testing.T) {
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

	sql, _ := BuildPivotUnionAllQuery(DialectPostgreSQL, ast, rowDims, colDims)

	branches := strings.Split(sql, " UNION ALL ")
	if len(branches) != 3 {
		t.Fatalf("expected 3 UNION ALL branches, got %d: %s", len(branches), sql)
	}
	for i, branch := range branches {
		if !strings.Contains(branch, `AVG(price) AS "avg_price"`) {
			t.Errorf("branch %d: expected AVG(price) recomputed by DB, got: %s", i+1, branch)
		}
		if !strings.Contains(branch, `COUNT(DISTINCT user_id) AS "uniq_users"`) {
			t.Errorf("branch %d: expected COUNT(DISTINCT user_id) recomputed by DB, got: %s", i+1, branch)
		}
	}
	if !strings.HasSuffix(branches[0], "GROUP BY region, product") {
		t.Errorf("detail branch must GROUP BY rows+cols, got: %s", branches[0])
	}
	if !strings.HasSuffix(branches[1], "GROUP BY region") {
		t.Errorf("subtotal branch must GROUP BY rows only, got: %s", branches[1])
	}
	if strings.Contains(branches[2], "GROUP BY") {
		t.Errorf("grand total branch must have no GROUP BY (whole-table aggregate), got: %s", branches[2])
	}
}

// TestBuildPivotUnionAllQuery_GranularityDim 锁定 UNION 的 ORDER BY 陷阱：带时间粒度
// 的维度在 SELECT/GROUP BY 里是 DATE_TRUNC(...) 表达式，但末尾 ORDER BY 必须引用
// UNION 的输出列别名（"created_at_day"），不能引用底层表达式——UNION 结果是匿名关系，
// ORDER BY DATE_TRUNC(...) 在多数方言直接报错。
func TestBuildPivotUnionAllQuery_GranularityDim(t *testing.T) {
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

	sql, _ := BuildPivotUnionAllQuery(DialectPostgreSQL, ast, rowDims, colDims)

	want := `SELECT DATE_TRUNC('day', created_at) AS "created_at_day", product, SUM(amount) AS "total", ` +
		`0 AS "__pivot_row_grp_0", 0 AS "__pivot_col_grp_0" ` +
		`FROM orders GROUP BY DATE_TRUNC('day', created_at), product` +
		` UNION ALL ` +
		`SELECT DATE_TRUNC('day', created_at) AS "created_at_day", NULL AS "product", SUM(amount) AS "total", ` +
		`0 AS "__pivot_row_grp_0", 1 AS "__pivot_col_grp_0" ` +
		`FROM orders GROUP BY DATE_TRUNC('day', created_at)` +
		` UNION ALL ` +
		`SELECT NULL AS "created_at_day", NULL AS "product", SUM(amount) AS "total", ` +
		`1 AS "__pivot_row_grp_0", 1 AS "__pivot_col_grp_0" ` +
		`FROM orders` +
		` ORDER BY "created_at_day", "__pivot_col_grp_0", "product"`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	// 显式钉子：末尾 ORDER BY 子句里不得出现底层粒度表达式（只能引用输出别名）。
	orderBy := sql[strings.LastIndex(sql, " ORDER BY "):]
	if strings.Contains(orderBy, "DATE_TRUNC") {
		t.Errorf("ORDER BY must reference UNION output aliases, not the underlying expression: %s", orderBy)
	}
}

// TestBuildPivotUnionAllQuery_FilterArgsTripled 验证 WHERE + args 出现三份：三个分支
// 各调用一次 buildWhereClause，args 数量 = 3 ×（单分支过滤参数个数），顺序与 ? 占位
// 从左到右一一对应；过滤值字面量不得进入 SQL 文本（与 GROUPING SETS 测试同款安全钉子）。
func TestBuildPivotUnionAllQuery_FilterArgsTripled(t *testing.T) {
	ast := pivotPlanAST(
		[]DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)
	rowDims, colDims, ok := resolvePivotSlots([]string{"region", "product"}, ast)
	if !ok {
		t.Fatal("expected pivot slots to resolve")
	}

	sql, args := BuildPivotUnionAllQuery(DialectPostgreSQL, ast, rowDims, colDims)

	if got := strings.Count(sql, "WHERE status = ?"); got != 3 {
		t.Errorf("expected 3 parameterized WHERE clauses (one per branch), got %d: %s", got, sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args (3 branches x 1 filter param), got %v", args)
	}
	for i, a := range args {
		if a != "ok" {
			t.Errorf("args[%d] = %v, want ok", i, a)
		}
	}
	if strings.Contains(sql, "'ok'") {
		t.Errorf("filter value leaked into SQL text: %s", sql)
	}
}

// TestBuildPivotUnionAllQuery_MySQLDialect 验证回退路径的主要目标方言（MySQL/
// StarRocks，caps.SupportsGroupingSets=false）下的完整 SQL：别名用反引号、
// 时间粒度用 DATE()，且 ORDER BY 仍只引用输出别名。
func TestBuildPivotUnionAllQuery_MySQLDialect(t *testing.T) {
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

	sql, _ := BuildPivotUnionAllQuery(DialectMySQL, ast, rowDims, colDims)

	want := "SELECT DATE(created_at) AS `created_at_day`, product, SUM(amount) AS `total`, " +
		"0 AS `__pivot_row_grp_0`, 0 AS `__pivot_col_grp_0` " +
		"FROM orders GROUP BY DATE(created_at), product" +
		" UNION ALL " +
		"SELECT DATE(created_at) AS `created_at_day`, NULL AS `product`, SUM(amount) AS `total`, " +
		"0 AS `__pivot_row_grp_0`, 1 AS `__pivot_col_grp_0` " +
		"FROM orders GROUP BY DATE(created_at)" +
		" UNION ALL " +
		"SELECT NULL AS `created_at_day`, NULL AS `product`, SUM(amount) AS `total`, " +
		"1 AS `__pivot_row_grp_0`, 1 AS `__pivot_col_grp_0` " +
		"FROM orders" +
		" ORDER BY `created_at_day`, `__pivot_col_grp_0`, `product`"
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
}
