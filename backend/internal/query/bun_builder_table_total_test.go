package query

import (
	"strings"
	"testing"
)

// tableTotalPlanAST 走 service 层同款真实管道（QuerySpec → PlanAST）构造 AST，
// 再按需回填列映射（模拟 executor 的 ApplyColumnIndex）。
func tableTotalPlanAST(dims []string, metrics []MetricExpr2, filters []FilterConfig) *QueryAST {
	spec := &QuerySpec{
		Metrics: metrics,
		Filters: filters,
	}
	for _, dim := range dims {
		spec.Dimensions = append(spec.Dimensions, DimensionExpr{Field: dim})
	}
	return NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec)
}

// TestBuildTableTotalQuery_Shape 钉死合计行 SQL 的形状：只 SELECT 指标、无维度、
// 无 GROUP BY、无 ORDER BY、无 LIMIT，过滤值走参数化 args。
// 「无 LIMIT」是本能力的正确性核心：带 LIMIT 就变成只汇总当前页。
func TestBuildTableTotalQuery_Shape(t *testing.T) {
	ast := tableTotalPlanAST(
		[]string{"region"},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)

	sql, args := BuildTableTotalQuery(DialectPostgreSQL, ast)

	want := `SELECT SUM(amount) AS "total" FROM orders WHERE status = ?`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 1 || args[0] != "ok" {
		t.Fatalf("expected args=[ok], got %v", args)
	}
	if strings.Contains(sql, "'ok'") {
		t.Fatalf("filter value leaked into SQL text: %s", sql)
	}
	for _, forbidden := range []string{"GROUP BY", "ORDER BY", "LIMIT", "OFFSET", "region"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("total SQL must not contain %q: %s", forbidden, sql)
		}
	}
}

// TestBuildTableTotalQuery_MultipleMetrics 验证多指标按 AST 顺序全部进入 SELECT，
// 别名各自带引号（结果键与明细行列名逐字一致，前端才能按列名取值）。
func TestBuildTableTotalQuery_MultipleMetrics(t *testing.T) {
	ast := tableTotalPlanAST(
		[]string{"region"},
		[]MetricExpr2{
			{Field: "amount", Agg: AggSum, Alias: "total"},
			{Field: "order_id", Agg: AggCount, Alias: "cnt"},
		},
		nil,
	)

	sql, args := BuildTableTotalQuery(DialectMySQL, ast)

	want := "SELECT SUM(amount) AS `total`, COUNT(order_id) AS `cnt` FROM orders"
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

// TestBuildTableTotalQuery_NoMetrics 无指标时没有可合计的量，返回空 SQL 让调用方跳过，
// 不得产出 `SELECT  FROM ...` 这种畸形语句。
func TestBuildTableTotalQuery_NoMetrics(t *testing.T) {
	ast := tableTotalPlanAST([]string{"region"}, nil, nil)

	sql, args := BuildTableTotalQuery(DialectPostgreSQL, ast)
	if sql != "" {
		t.Fatalf("expected empty SQL without metrics, got %q", sql)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

// TestBuildTableTotalQuery_AggExprColumnNotDoubleWrapped 列映射里的虚拟字段本身已是
// 聚合形态（SUM(amount)）时不得再套一层聚合函数——与明细行的 renderMetricSelect 同规则，
// 否则合计口径与明细不一致。
func TestBuildTableTotalQuery_AggExprColumnNotDoubleWrapped(t *testing.T) {
	ast := tableTotalPlanAST(
		[]string{"region"},
		[]MetricExpr2{{Field: "amount_sum", Agg: AggSum, Alias: "total"}},
		nil,
	)
	ast.ColumnMappings = map[string]string{"amount_sum": "SUM(amount)"}
	ast.ApplyColumnMappings(ast.ColumnMappings)

	sql, _ := BuildTableTotalQuery(DialectPostgreSQL, ast)

	want := `SELECT SUM(amount) AS "total" FROM orders`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
	if strings.Contains(sql, "SUM(SUM") {
		t.Fatalf("aggregated column double wrapped: %s", sql)
	}
}

// TestBuildTableTotalQuery_VirtualSourceWrappedAsSubquery 数据集是 SQL 形态时，合计查询
// 必须与主查询一样把 source 包成子查询，否则引用到不存在的表。
func TestBuildTableTotalQuery_VirtualSourceWrappedAsSubquery(t *testing.T) {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{{Field: "region"}},
		Metrics:    []MetricExpr2{{Field: "amount", Agg: AggAvg, Alias: "avg_amount"}},
	}
	ast := NewQueryPlanner().PlanAST("SELECT * FROM orders", SourceTypeSQL, spec)

	sql, _ := BuildTableTotalQuery(DialectPostgreSQL, ast)

	want := `SELECT AVG(amount) AS "avg_amount" FROM (SELECT * FROM orders) AS _subq`
	if sql != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, sql)
	}
}

// TestTableTotalOptions 覆盖开关读取：snake_case（wire）与 camelCase（持久化文档）都认，
// 数值 1 视同为真，其余形态一律关闭（未知/脏数据不得凭空打开一条额外查询）。
func TestTableTotalOptions(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want bool
	}{
		{"nil", nil, false},
		{"empty", map[string]any{}, false},
		{"snake true", map[string]any{"show_total": true}, true},
		{"snake false", map[string]any{"show_total": false}, false},
		{"camel true", map[string]any{"showTotal": true}, true},
		{"numeric 1", map[string]any{"show_total": float64(1)}, true},
		{"numeric 0", map[string]any{"show_total": float64(0)}, false},
		{"string junk", map[string]any{"show_total": "yes"}, false},
		{"other key only", map[string]any{"bin_count": float64(5)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tableTotalOptions(tc.opts); got != tc.want {
				t.Errorf("tableTotalOptions(%v) = %v, want %v", tc.opts, got, tc.want)
			}
		})
	}
}
