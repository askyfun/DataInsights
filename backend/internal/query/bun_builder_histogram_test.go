package query

import (
	"strings"
	"testing"
)

// histogramPlanAST 走 service 层同款真实管道（QuerySpec → PlanAST）构造 AST。
func histogramPlanAST(metrics []MetricExpr2, filters []FilterConfig) *QueryAST {
	spec := &QuerySpec{
		Metrics: metrics,
		Filters: filters,
	}
	return NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)
}

// TestBuildHistogramStatsQuery_FullSQL 钉死阶段1（统计）SQL 的完整形状：
// MIN/MAX/COUNT(*) + 引号保留别名 + 参数化 WHERE（照 bun_builder_pivot_test.go
// 的全字符串相等风格）。
func TestBuildHistogramStatsQuery_FullSQL(t *testing.T) {
	ast := histogramPlanAST(
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)

	sql, args := BuildHistogramStatsQuery(DialectPostgreSQL, ast, "amount")

	want := `SELECT MIN(amount) AS "mn", MAX(amount) AS "mx", COUNT(*) AS "cnt" ` +
		`FROM orders WHERE status = ?`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 1 || args[0] != "ok" {
		t.Fatalf("expected args=[ok], got %v", args)
	}
	if strings.Contains(sql, "'ok'") {
		t.Fatalf("filter value leaked into SQL text: %s", sql)
	}
}

// TestBuildHistogramStatsQuery_SQLSource 验证 SQL 数据集源包一层 _subq 派生表
// （与通用/透视 builder 同口径）。
func TestBuildHistogramStatsQuery_SQLSource(t *testing.T) {
	spec := &QuerySpec{Metrics: []MetricExpr2{{Field: "v", Agg: AggCount}}}
	ast := NewQueryPlanner().PlanAST("SELECT v FROM raw_events", SourceTypeSQL, spec)

	sql, args := BuildHistogramStatsQuery(DialectPostgreSQL, ast, "v")

	want := `SELECT MIN(v) AS "mn", MAX(v) AS "mx", COUNT(*) AS "cnt" ` +
		`FROM (SELECT v FROM raw_events) AS _subq`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

// TestBuildHistogramBinQuery_FullSQL 钉死阶段2（分箱）SQL 的完整形状：
// FLOOR((field - ?) / ?) + 引号保留别名 + 参数化 WHERE + GROUP BY 派生表列
// （子查询包一层，GROUP BY "bin" 引用派生列而非重复表达式）+ ORDER BY "bin"；
// min/binWidth 必须是参数化 ? 占位（args 一份：[min, binWidth, 过滤值...]，
// FLOOR 参数只出现一次），绝不能是裸浮点拼进 SQL 文本。
func TestBuildHistogramBinQuery_FullSQL(t *testing.T) {
	ast := histogramPlanAST(
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)

	sql, args := BuildHistogramBinQuery(DialectPostgreSQL, ast, "amount", 0, 12.5)

	want := `SELECT "bin", COUNT(*) AS "cnt" ` +
		`FROM (SELECT FLOOR((amount - ?) / ?) AS "bin" FROM orders WHERE status = ?) AS _hist_bins ` +
		`GROUP BY "bin" ORDER BY "bin"`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	// args 与 ? 占位从左到右一一对应：内层 SELECT(min,width) → WHERE(过滤值)
	wantArgs := []any{float64(0), float64(12.5), "ok"}
	if len(args) != len(wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, args)
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Fatalf("arg[%d]: expected %v (%T), got %v (%T)", i, wantArgs[i], wantArgs[i], args[i], args[i])
		}
	}
	// 裸浮点/过滤值不得进入 SQL 文本
	for _, leaked := range []string{"12.5", "'ok'", "0 /"} {
		if strings.Contains(sql, leaked) {
			t.Fatalf("value %q leaked into SQL text: %s", leaked, sql)
		}
	}
}

// TestBuildHistogramBinQuery_NoFilters 无过滤时 args 只有两份 (min, binWidth)。
func TestBuildHistogramBinQuery_NoFilters(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)

	sql, args := BuildHistogramBinQuery(DialectPostgreSQL, ast, "amount", 3, 2)

	want := `SELECT "bin", COUNT(*) AS "cnt" ` +
		`FROM (SELECT FLOOR((amount - ?) / ?) AS "bin" FROM orders) AS _hist_bins ` +
		`GROUP BY "bin" ORDER BY "bin"`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 2 || args[0] != float64(3) || args[1] != float64(2) {
		t.Fatalf("expected args [3 2], got %v", args)
	}
}

// TestBuildHistogram_FieldExprResolution 验证字段解析与其他 builder 同源：
// 列映射命中时用映射表达式；非法字段（注入形态）被 safeIdentifier 拒绝为
// _invalid_identifier，不进 SQL。
func TestBuildHistogram_FieldExprResolution(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)
	ast.ColumnMappings = map[string]string{"amount": "amt_col"}

	sql, _ := BuildHistogramStatsQuery(DialectPostgreSQL, ast, "amount")
	if !strings.Contains(sql, "MIN(amt_col)") || !strings.Contains(sql, "MAX(amt_col)") {
		t.Fatalf("column mapping not applied: %s", sql)
	}

	binSQL, _ := BuildHistogramBinQuery(DialectPostgreSQL, ast, "amount", 0, 1)
	if !strings.Contains(binSQL, "FLOOR((amt_col - ?) / ?)") {
		t.Fatalf("column mapping not applied to bin expr: %s", binSQL)
	}

	evilSQL, _ := BuildHistogramStatsQuery(DialectPostgreSQL, ast, "amount; DROP TABLE orders")
	if !strings.Contains(evilSQL, "MIN(_invalid_identifier)") {
		t.Fatalf("injection field not rejected: %s", evilSQL)
	}
	if strings.Contains(evilSQL, "DROP TABLE") {
		t.Fatalf("injection leaked into SQL: %s", evilSQL)
	}
}
