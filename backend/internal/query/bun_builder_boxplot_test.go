package query

import (
	"strings"
	"testing"

	"dataray/internal/datasource"
)

// boxplotCaps 是 boxplot stats builder 需要的能力声明（PG percentile_cont）。
// executor 前置门保证只有支持 percentile 的 dialect 才走到这里。
func boxplotCaps() *datasource.DialectCapabilities {
	return &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"}
}

// TestBuildBoxplotStatsQuery_FullSQL 钉死阶段1（stats）SQL 完整形状：
// MIN AS wlo + percentile_cont(0.25/0.5/0.75) AS q1/med/q3 + MAX AS whi +
// 引号保留别名 + 参数化 WHERE。
func TestBuildBoxplotStatsQuery_FullSQL(t *testing.T) {
	ast := histogramPlanAST(
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)

	sql, args, err := BuildBoxplotStatsQuery(DialectPostgreSQL, ast, "amount", boxplotCaps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT MIN(amount) AS "wlo", ` +
		`percentile_cont(0.25) WITHIN GROUP (ORDER BY amount) AS "q1", ` +
		`percentile_cont(0.5) WITHIN GROUP (ORDER BY amount) AS "med", ` +
		`percentile_cont(0.75) WITHIN GROUP (ORDER BY amount) AS "q3", ` +
		`MAX(amount) AS "whi" ` +
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

// TestBuildBoxplotStatsQuery_SQLSource 验证 SQL 数据集源包一层 _subq 派生表。
func TestBuildBoxplotStatsQuery_SQLSource(t *testing.T) {
	spec := &QuerySpec{Metrics: []MetricExpr2{{Field: "v", Agg: AggCount}}}
	ast := NewQueryPlanner().PlanAST("SELECT v FROM raw_events", SourceTypeSQL, spec)

	sql, args, err := BuildBoxplotStatsQuery(DialectPostgreSQL, ast, "v", boxplotCaps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "FROM (SELECT v FROM raw_events) AS _subq") {
		t.Fatalf("SQL source not wrapped: %s", sql)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

// TestBuildBoxplotStatsQuery_UnsupportedDialect 验证 builder 层不静默近似：
// 若调用者绕过 executor 前置门、传入不支持的 caps，则返回明确错误。
func TestBuildBoxplotStatsQuery_UnsupportedDialect(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)
	_, _, err := BuildBoxplotStatsQuery(DialectPostgreSQL, ast, "amount",
		&datasource.DialectCapabilities{PercentileStrategy: "unsupported"})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected explicit unsupported error, got %v", err)
	}
}

// TestBuildBoxplotOutliersQuery_FullSQL 钉死离群点列表查询：fence 两个 ? 参数化
// （args 先入 [lower, upper]，过滤值在其后）、ORDER BY field 升序、LIMIT 常量、
// 值不泄漏进 SQL 文本。
func TestBuildBoxplotOutliersQuery_FullSQL(t *testing.T) {
	ast := histogramPlanAST(
		[]MetricExpr2{{Field: "amount", Agg: AggSum}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)

	sql, args := BuildBoxplotOutliersQuery(DialectPostgreSQL, ast, "amount", 1.5, 8.5)

	want := `SELECT amount AS "val" FROM orders WHERE (amount < ? OR amount > ?) AND status = ? ORDER BY amount LIMIT 1000`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	wantArgs := []any{float64(1.5), float64(8.5), "ok"}
	if len(args) != len(wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, args)
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Fatalf("arg[%d]: expected %v, got %v", i, wantArgs[i], args[i])
		}
	}
	for _, leaked := range []string{"1.5", "8.5", "'ok'"} {
		if strings.Contains(sql, leaked) {
			t.Fatalf("value %q leaked into SQL text: %s", leaked, sql)
		}
	}
}

// TestBuildBoxplotOutliersQuery_NoFilters 无过滤时只有两份 fence 参数。
func TestBuildBoxplotOutliersQuery_NoFilters(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)
	sql, args := BuildBoxplotOutliersQuery(DialectPostgreSQL, ast, "amount", 0, 10)
	want := `SELECT amount AS "val" FROM orders WHERE (amount < ? OR amount > ?) ORDER BY amount LIMIT 1000`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 2 || args[0] != float64(0) || args[1] != float64(10) {
		t.Fatalf("expected args [0 10], got %v", args)
	}
}

// TestBuildBoxplotOutlierCountQuery_FullSQL 验证总数查询无 LIMIT、返回 COUNT(*)。
func TestBuildBoxplotOutlierCountQuery_FullSQL(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)
	sql, args := BuildBoxplotOutlierCountQuery(DialectPostgreSQL, ast, "amount", 1.5, 8.5)
	want := `SELECT COUNT(*) AS "ocnt" FROM orders WHERE (amount < ? OR amount > ?)`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %v", args)
	}
	if strings.Contains(sql, "LIMIT") {
		t.Fatalf("count query must not carry display LIMIT: %s", sql)
	}
}

// TestBuildBoxplot_FieldExprResolution 验证字段解析与其他 builder 同源：列映射
// 命中时用映射表达式；注入形态字段被 safeIdentifier 拒绝为 _invalid_identifier。
func TestBuildBoxplot_FieldExprResolution(t *testing.T) {
	ast := histogramPlanAST([]MetricExpr2{{Field: "amount", Agg: AggSum}}, nil)
	ast.ColumnMappings = map[string]string{"amount": "amt_col"}

	sql, _, err := BuildBoxplotStatsQuery(DialectPostgreSQL, ast, "amount", boxplotCaps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "MIN(amt_col)") || !strings.Contains(sql, "ORDER BY amt_col)") {
		t.Fatalf("column mapping not applied: %s", sql)
	}

	outSQL, _ := BuildBoxplotOutliersQuery(DialectPostgreSQL, ast, "amount", 0, 1)
	if !strings.Contains(outSQL, "(amt_col < ? OR amt_col > ?)") {
		t.Fatalf("column mapping not applied to outlier fence: %s", outSQL)
	}

	evilSQL, _, err := BuildBoxplotStatsQuery(DialectPostgreSQL, ast, "amount; DROP TABLE orders", boxplotCaps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(evilSQL, "MIN(_invalid_identifier)") {
		t.Fatalf("injection field not rejected: %s", evilSQL)
	}
	if strings.Contains(evilSQL, "DROP TABLE") {
		t.Fatalf("injection leaked into SQL: %s", evilSQL)
	}
}
