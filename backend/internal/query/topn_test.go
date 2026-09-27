package query

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"testing"

	"data-insights/internal/model"
)

func TestParseTopN(t *testing.T) {
	if got := parseTopN(nil); got != nil {
		t.Errorf("nil opts: %+v", got)
	}
	if got := parseTopN(map[string]any{"top_n": "3"}); got != nil {
		t.Errorf("non-object: %+v", got)
	}
	for _, bad := range []any{0, -1, 1.5, "abc", nil, math.Inf(1)} {
		if got := parseTopN(map[string]any{"top_n": map[string]any{"limit": bad}}); got != nil {
			t.Errorf("limit %v must be rejected: %+v", bad, got)
		}
	}
	got := parseTopN(map[string]any{"top_n": map[string]any{"limit": float64(5), "metric": " m1 ", "order": "ASC"}})
	if got == nil || got.Limit != 5 || got.Metric != "m1" || got.Order != "asc" {
		t.Errorf("valid: %+v", got)
	}
	got = parseTopN(map[string]any{"top_n": map[string]any{"limit": "10"}})
	if got == nil || got.Limit != 10 || got.Order != "desc" || got.Metric != "" {
		t.Errorf("string limit + defaults: %+v", got)
	}
	if got := parseTopN(map[string]any{"top_n": map[string]any{"limit": 3, "order": "wow"}}); got != nil {
		t.Errorf("unknown order must disable: %+v", got)
	}
}

func TestApplyTopN(t *testing.T) {
	ast := &QueryAST{Metrics: []MetricExpr{
		{Field: "c1", Alias: "amount"},
		{Field: "c2", Alias: "qty"},
	}}
	if err := applyTopN(ast, &topNConfig{Limit: 5, Order: "desc"}); err != nil {
		t.Fatal(err)
	}
	if ast.Sort == nil || ast.Sort.Field != "amount" || ast.Sort.Order != "desc" || ast.Limit != 5 {
		t.Errorf("default first metric: %+v limit=%d", ast.Sort, ast.Limit)
	}
	// 按列 ID 命中第二个指标
	if err := applyTopN(ast, &topNConfig{Limit: 3, Metric: "c2", Order: "asc"}); err != nil {
		t.Fatal(err)
	}
	if ast.Sort.Field != "qty" || ast.Sort.Order != "asc" || ast.Limit != 3 {
		t.Errorf("by id: %+v", ast.Sort)
	}
	// 按输出别名命中
	if err := applyTopN(ast, &topNConfig{Limit: 2, Metric: "amount"}); err != nil {
		t.Fatal(err)
	}
	if ast.Sort.Field != "amount" {
		t.Errorf("by alias: %+v", ast.Sort)
	}
	// 解析不到 → 显式报错，不静默换排名依据
	if err := applyTopN(ast, &topNConfig{Limit: 2, Metric: "ghost"}); err == nil {
		t.Error("expected unknown metric error")
	}
	if err := applyTopN(&QueryAST{}, &topNConfig{Limit: 2}); err == nil {
		t.Error("expected no-metric error")
	}
}

func topNExecutor(t *testing.T, conn *MockConnection) *Executor {
	t.Helper()
	dataset := &model.Dataset{
		ID:        1,
		Name:      "sales",
		QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		Columns:   `[{"id":"c1","name":"brand","expr":"brand"},{"id":"c2","name":"amount","expr":"amount"}]`,
	}
	ds := &model.Datasource{ID: 1, Name: "pg", Type: "postgresql"}
	return NewExecutor(conn, dataset, ds)
}

func TestExecutor_TopN(t *testing.T) {
	conn := &MockConnection{rows: []map[string]any{
		{"brand": "A", "amount": 30.0},
		{"brand": "B", "amount": 20.0},
	}}
	e := topNExecutor(t, conn)
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePie,
		Dims:      []string{"c1"},
		Metrics:   []MetricConfig{{Field: "c2", Agg: AggSum}},
		QueryOptions: map[string]any{
			"top_n": map[string]any{"limit": float64(2), "metric": "c2"},
		},
	}
	if _, err := e.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	sqlText := conn.sqlCalls[0]
	if !strings.Contains(sqlText, `ORDER BY "amount" DESC`) || !strings.Contains(sqlText, "LIMIT 2") {
		t.Errorf("top n not pushed into SQL: %s", sqlText)
	}
	// 未启用：SQL 不带 LIMIT
	conn2 := &MockConnection{rows: conn.rows}
	req2 := &ChartQueryRequest{
		DatasetID: 1, ChartType: ChartTypePie,
		Dims: []string{"c1"}, Metrics: []MetricConfig{{Field: "c2", Agg: AggSum}},
	}
	if _, err := topNExecutor(t, conn2).Execute(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conn2.sqlCalls[0], "LIMIT") {
		t.Errorf("unexpected LIMIT without top_n: %s", conn2.sqlCalls[0])
	}
	// 指标解析不到 → 报错
	conn3 := &MockConnection{rows: conn.rows}
	req3 := &ChartQueryRequest{
		DatasetID: 1, ChartType: ChartTypePie,
		Dims: []string{"c1"}, Metrics: []MetricConfig{{Field: "c2", Agg: AggSum}},
		QueryOptions: map[string]any{"top_n": map[string]any{"limit": float64(2), "metric": "ghost"}},
	}
	if _, err := topNExecutor(t, conn3).Execute(context.Background(), req3); err == nil {
		t.Error("expected unknown metric error")
	}
}
