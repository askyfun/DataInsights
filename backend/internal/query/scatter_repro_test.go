package query

import (
	"strings"
	"testing"
)

// 回归：isAggregateFunction 曾用 HasPrefix 判断，min_price/max_price 这类以
// 聚合函数名开头的普通列名被误判为聚合表达式，renderMetricSelect 跳过聚合、
// 生成裸列 SQL（scatter 无维度查询在 StarRocks/PG 直接报 1064/42803）。
// 正确行为：普通列必须按 Agg 包聚合函数；真正的聚合表达式（列映射形态）才透传。
func TestIsAggregateFunctionDoesNotMatchPrefixedColumns(t *testing.T) {
	cases := map[string]bool{
		"min_price":       false,
		"max_price":       false,
		"summary_count":   false,
		"average_score":   false,
		"count_of_rows":   false,
		"SUM(amount)":     true,
		"count(*)":        true,
		"MIN (price)":     true,
		"GROUP_CONCAT(x)": true,
	}
	for expr, want := range cases {
		if got := isAggregateFunction(expr); got != want {
			t.Errorf("isAggregateFunction(%q) = %v, want %v", expr, got, want)
		}
	}
}

func TestScatterAggSQLWrapsPlainColumns(t *testing.T) {
	planner := NewQueryPlanner()
	req := &ChartQueryRequest{
		ChartType: ChartTypeScatter,
		Dims:      []string{},
		Metrics: []MetricConfig{
			{Field: "min_price", Agg: AggAvg},
			{Field: "retail_sales", Agg: AggSum},
		},
	}
	spec := QuerySpecFromRequest(req)
	ast := planner.PlanAST("raw_auto_sales_model_rank", SourceTypeTable, spec)
	sql, _, _ := BuildQueryStringWithBun(ParseDialect("starrocks"), ast)
	if !strings.Contains(sql, "AVG(min_price)") {
		t.Errorf("expected AVG(min_price) in sql, got: %s", sql)
	}
	if strings.Contains(sql, "SELECT min_price AS") {
		t.Errorf("plain column must not be selected raw, got: %s", sql)
	}
}
