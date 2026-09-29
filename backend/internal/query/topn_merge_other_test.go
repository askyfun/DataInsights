package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

// ---------------------------------------------------------------------------
// SQL 构造：BuildTopNRestQuery
// ---------------------------------------------------------------------------

func TestBuildTopNRestQuery_Shape(t *testing.T) {
	ast := tableTotalPlanAST(
		[]string{"region"},
		[]MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total"}},
		[]FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}},
	)
	// 模拟 applyTopN 已经写进 AST 的截断：内层必须把它摘掉，否则「全量」只剩前 N。
	ast.Sort = &SortExpr{Field: "total", Order: "desc"}
	ast.Limit = 3

	got, args := BuildTopNRestQuery(DialectPostgreSQL, ast)

	want := `SELECT COUNT(*) AS _topn_groups, SUM("total") AS "total"` +
		` FROM (SELECT region, SUM(amount) AS "total" FROM orders WHERE status = ? GROUP BY region) AS _topn_rest`
	if got != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, got)
	}
	if len(args) != 1 || args[0] != "ok" {
		t.Fatalf("expected filter value parameterised as args=[ok], got %v", args)
	}
	if strings.Contains(got, "'ok'") {
		t.Fatalf("filter value leaked into SQL text: %s", got)
	}
	// 内层带 LIMIT/ORDER BY 会让「全量」变味；外层不该出现 LIMIT。
	if strings.Contains(got, "LIMIT") {
		t.Errorf("rest query must not carry Top N's LIMIT: %s", got)
	}
	if strings.Contains(got, "ORDER BY") {
		t.Errorf("rest query must not carry ORDER BY: %s", got)
	}
	// AST 不被改写（浅拷贝只给内层）。
	if ast.Limit != 3 || ast.Sort == nil {
		t.Errorf("BuildTopNRestQuery mutated the input AST: limit=%d sort=%+v", ast.Limit, ast.Sort)
	}
}

func TestBuildTopNRestQuery_MultipleMetricsAndDialects(t *testing.T) {
	ast := tableTotalPlanAST(
		[]string{"region"},
		[]MetricExpr2{
			{Field: "amount", Agg: AggSum, Alias: "total"},
			{Field: "order_id", Agg: AggCount, Alias: "cnt"},
		},
		nil,
	)

	got, args := BuildTopNRestQuery(DialectMySQL, ast)

	want := "SELECT COUNT(*) AS _topn_groups, SUM(`total`) AS `total`, SUM(`cnt`) AS `cnt`" +
		" FROM (SELECT region, SUM(amount) AS `total`, COUNT(order_id) AS `cnt` FROM orders GROUP BY region) AS _topn_rest"
	if got != want {
		t.Fatalf("unexpected SQL\nwant: %s\n got: %s", want, got)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

func TestBuildTopNRestQuery_NoMetrics(t *testing.T) {
	ast := tableTotalPlanAST([]string{"region"}, nil, nil)
	got, args := BuildTopNRestQuery(DialectPostgreSQL, ast)
	if got != "" || args != nil {
		t.Fatalf("no metrics must yield empty SQL, got %q args=%v", got, args)
	}
}

// ---------------------------------------------------------------------------
// 开关解析与可加性门
// ---------------------------------------------------------------------------

func TestParseTopN_MergeOther(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want bool
	}{
		{"absent", map[string]any{"limit": 3}, false},
		{"snake true", map[string]any{"limit": 3, "merge_other": true}, true},
		{"snake false", map[string]any{"limit": 3, "merge_other": false}, false},
		{"camel true（持久化文档未归一）", map[string]any{"limit": 3, "mergeOther": true}, true},
		{"numeric 1", map[string]any{"limit": 3, "merge_other": float64(1)}, true},
		{"numeric 0", map[string]any{"limit": 3, "merge_other": float64(0)}, false},
		{"字符串不作数", map[string]any{"limit": 3, "merge_other": "yes"}, false},
		{"snake 显式 false 优先于 camel true", map[string]any{"limit": 3, "merge_other": false, "mergeOther": true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTopN(map[string]any{"top_n": tc.opts})
			if got == nil {
				t.Fatalf("section disabled: %+v", tc.opts)
			}
			if got.MergeOther != tc.want {
				t.Errorf("MergeOther=%v want %v", got.MergeOther, tc.want)
			}
		})
	}
}

func TestApplyTopN_MergeOtherAdditivity(t *testing.T) {
	merge := &topNConfig{Limit: 3, Order: "desc", MergeOther: true}

	additive := func(agg AggregationType) *QueryAST {
		return &QueryAST{Dimensions: []string{"brand"}, Metrics: []MetricExpr{
			{Field: "amount", Alias: "amount", Agg: agg},
		}}
	}
	for _, agg := range []AggregationType{AggSum, AggCount} {
		ast := additive(agg)
		if err := applyTopN(ast, merge); err != nil {
			t.Errorf("%s must be accepted: %v", agg, err)
		}
	}
	for _, agg := range []AggregationType{AggAvg, AggMin, AggMax, AggCountDistinct, AggMedian} {
		ast := additive(agg)
		if err := applyTopN(ast, merge); err == nil {
			t.Errorf("%s must be rejected (its 其他 value is not derivable)", agg)
		}
	}

	// IsAgg（列映射带来的整段聚合表达式）形态不可知 → 按不可加处理。
	isAgg := &QueryAST{Dimensions: []string{"brand"}, Metrics: []MetricExpr{
		{Field: "amount", FieldExpr: "SUM(amount)", Alias: "amount", Agg: AggSum, IsAgg: true},
	}}
	if err := applyTopN(isAgg, merge); err == nil {
		t.Error("IsAgg metrics must be rejected")
	}

	// 多指标里混一个不可加 → 整节拒绝（缺一列的「其他」行就是半真半假）。
	mixed := &QueryAST{Dimensions: []string{"brand"}, Metrics: []MetricExpr{
		{Field: "amount", Alias: "amount", Agg: AggSum},
		{Field: "price", Alias: "price", Agg: AggAvg},
	}}
	if err := applyTopN(mixed, merge); err == nil {
		t.Error("mixed additive/non-additive metrics must be rejected")
	}
	// 同样的组合不开 merge_other → 与改动前一致，照常工作。
	if err := applyTopN(mixed, &topNConfig{Limit: 3, Order: "desc"}); err != nil {
		t.Errorf("top_n without merge_other must stay unaffected: %v", err)
	}
}

func TestGuardTopNMergeOther(t *testing.T) {
	ast := &QueryAST{Dimensions: []string{"brand"}}
	for _, ct := range []ChartType{ChartTypeBar, ChartTypeLine, ChartTypeArea, ChartTypePie} {
		if err := guardTopNMergeOther(ct, ast); err != nil {
			t.Errorf("%s must pass: %v", ct, err)
		}
	}
	for _, ct := range []ChartType{ChartTypeTable, ChartTypePivot, ChartTypeHistogram, ChartTypeBoxplot, ChartTypeScatter} {
		if err := guardTopNMergeOther(ct, ast); err == nil {
			t.Errorf("%s must be rejected", ct)
		}
	}
	if err := guardTopNMergeOther(ChartTypeBar, &QueryAST{Dimensions: []string{"a", "b"}}); err == nil {
		t.Error("multi-dimension must be rejected")
	}
	if err := guardTopNMergeOther(ChartTypeBar, &QueryAST{}); err == nil {
		t.Error("zero-dimension must be rejected")
	}
}

func TestTopNRowDimKey(t *testing.T) {
	metrics := []MetricExpr{{Alias: "amount"}, {Alias: "cnt"}}
	key, err := topNRowDimKey(map[string]any{"brand": "A", "amount": 1.0, "cnt": 2.0}, metrics)
	if err != nil || key != "brand" {
		t.Fatalf("got %q err=%v", key, err)
	}
	if _, err := topNRowDimKey(map[string]any{"a": "", "b": "", "amount": 1.0}, metrics); err == nil {
		t.Error("two non-metric columns must be rejected")
	}
	if _, err := topNRowDimKey(map[string]any{"amount": 1.0}, metrics); err == nil {
		t.Error("zero dimension columns must be rejected")
	}
}

// ---------------------------------------------------------------------------
// 执行器编排：两条查询 + 追加行
// ---------------------------------------------------------------------------

// topNRestConnection 按调用次序给不同结果：第一次是主查询（前 N 行），
// 第二次是「其余」汇总。同时记录 SQL 便于断言只发了该有的那两条。
type topNRestConnection struct {
	MockConnection
	mainRows []map[string]any
	restRows []map[string]any
}

func (c *topNRestConnection) Execute(ctx context.Context, sqlText string, args ...any) (*datasource.QueryResult, error) {
	c.sqlCalls = append(c.sqlCalls, sqlText)
	c.argsCalls = append(c.argsCalls, args)
	if strings.HasPrefix(sqlText, "SELECT COUNT(*) AS _topn_groups") {
		return &datasource.QueryResult{Rows: c.restRows}, nil
	}
	return &datasource.QueryResult{Rows: c.mainRows}, nil
}

func topNMergeReq(limit int) *ChartQueryRequest {
	return &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"c1"},
		Metrics:   []MetricConfig{{Field: "c2", Agg: AggSum}},
		QueryOptions: map[string]any{
			"top_n": map[string]any{"limit": limit, "metric": "c2", "merge_other": true},
		},
	}
}

func execTopNMerge(t *testing.T, conn *topNRestConnection, req *ChartQueryRequest) interface{} {
	t.Helper()
	dataset := &model.Dataset{
		ID:        1,
		Name:      "sales",
		QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		Columns:   `[{"id":"c1","name":"brand","expr":"brand"},{"id":"c2","name":"amount","expr":"amount"}]`,
	}
	e := NewExecutor(conn, dataset, &model.Datasource{ID: 1, Name: "pg", Type: "postgresql"})
	res, err := e.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res.Data
}

func TestExecutor_TopNMergeOtherAppendsRow(t *testing.T) {
	conn := &topNRestConnection{
		mainRows: []map[string]any{
			{"brand": "A", "amount": 30.0},
			{"brand": "B", "amount": 20.0},
		},
		// 全量 5 组、总额 100 → 其他 = 100 − 50 = 50。
		restRows: []map[string]any{{"_topn_groups": 5.0, "amount": 100.0}},
	}
	data := execTopNMerge(t, conn, topNMergeReq(2))

	axis, ok := data.(*AxisResponse)
	if !ok {
		t.Fatalf("unexpected payload %T", data)
	}
	if len(axis.XAxis) != 3 || axis.XAxis[2] != TopNOtherLabel {
		t.Fatalf("expected trailing %q axis entry, got %v", TopNOtherLabel, axis.XAxis)
	}
	if len(axis.Series) != 1 || len(axis.Series[0].Data) != 3 {
		t.Fatalf("series shape: %+v", axis.Series)
	}
	if got, ok := toFloat64(axis.Series[0].Data[2]); !ok || got != 50 {
		t.Errorf("其他 value = %v, want 50", axis.Series[0].Data[2])
	}
	if len(conn.sqlCalls) != 2 {
		t.Fatalf("expected main + rest queries, got %d: %v", len(conn.sqlCalls), conn.sqlCalls)
	}
	if !strings.Contains(conn.sqlCalls[0], "LIMIT 2") {
		t.Errorf("main query must keep the Top N truncation: %s", conn.sqlCalls[0])
	}
	if !strings.HasPrefix(conn.sqlCalls[1], "SELECT COUNT(*) AS _topn_groups") {
		t.Errorf("second query is not the rest aggregate: %s", conn.sqlCalls[1])
	}
}

func TestExecutor_TopNMergeOtherNoTruncationSkipsRestQuery(t *testing.T) {
	conn := &topNRestConnection{
		mainRows: []map[string]any{{"brand": "A", "amount": 30.0}},
	}
	execTopNMerge(t, conn, topNMergeReq(5)) // 只回 1 行，没到 Top 5 → 后面没有其余

	if len(conn.sqlCalls) != 1 {
		t.Fatalf("rest query must not run when nothing was truncated, got %v", conn.sqlCalls)
	}
}

func TestExecutor_TopNMergeOtherExactlyNGroupsAppendsNothing(t *testing.T) {
	conn := &topNRestConnection{
		mainRows: []map[string]any{
			{"brand": "A", "amount": 30.0},
			{"brand": "B", "amount": 20.0},
		},
		restRows: []map[string]any{{"_topn_groups": 2.0, "amount": 50.0}}, // 恰好 2 组
	}
	data := execTopNMerge(t, conn, topNMergeReq(2))

	axis := data.(*AxisResponse)
	if len(axis.XAxis) != 2 {
		t.Fatalf("no remainder must not produce a 其他 row, got %v", axis.XAxis)
	}
	if len(conn.sqlCalls) != 2 {
		t.Fatalf("expected exactly main + rest, got %v", conn.sqlCalls)
	}
}

func TestExecutor_TopNMergeOtherNonAdditiveFailsBeforeQuery(t *testing.T) {
	conn := &topNRestConnection{}
	req := topNMergeReq(2)
	req.Metrics = []MetricConfig{{Field: "c2", Agg: AggAvg}}

	dataset := &model.Dataset{
		ID: 1, QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		Columns:   `[{"id":"c1","name":"brand","expr":"brand"},{"id":"c2","name":"amount","expr":"amount"}]`,
	}
	e := NewExecutor(conn, dataset, &model.Datasource{ID: 1, Type: "postgresql"})
	if _, err := e.Execute(context.Background(), req); err == nil {
		t.Fatal("avg + merge_other must fail")
	} else if !strings.Contains(err.Error(), "可加") {
		t.Errorf("unhelpful error: %v", err)
	}
	if len(conn.sqlCalls) != 0 {
		t.Errorf("must fail before any SQL goes out, got %v", conn.sqlCalls)
	}
}

func TestExecutor_TopNMergeOtherRestQueryErrorPropagates(t *testing.T) {
	conn := &topNRestConnection{
		mainRows: []map[string]any{
			{"brand": "A", "amount": 30.0},
			{"brand": "B", "amount": 20.0},
		},
		restRows: []map[string]any{{"_topn_groups": 9.0, "amount": nil}}, // 该列全 NULL
	}
	dataset := &model.Dataset{
		ID: 1, QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		Columns:   `[{"id":"c1","name":"brand","expr":"brand"},{"id":"c2","name":"amount","expr":"amount"}]`,
	}
	e := NewExecutor(conn, dataset, &model.Datasource{ID: 1, Type: "postgresql"})
	if _, err := e.Execute(context.Background(), topNMergeReq(2)); err == nil {
		t.Fatal("NULL total must fail rather than fabricate a 其他 value")
	}
}

func TestExecutor_TopNMergeOtherUnsupportedChartType(t *testing.T) {
	conn := &topNRestConnection{}
	req := topNMergeReq(2)
	req.ChartType = ChartTypeTable

	dataset := &model.Dataset{
		ID: 1, QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		Columns:   `[{"id":"c1","name":"brand","expr":"brand"},{"id":"c2","name":"amount","expr":"amount"}]`,
	}
	e := NewExecutor(conn, dataset, &model.Datasource{ID: 1, Type: "postgresql"})
	if _, err := e.Execute(context.Background(), req); err == nil {
		t.Fatal("table + merge_other must be rejected explicitly, not silently ignored")
	}
}

// 不开 merge_other 时，Top N 的行为必须与本改动前逐字一致：只一条 SQL、不追加行。
func TestExecutor_TopNWithoutMergeOtherUnchanged(t *testing.T) {
	conn := &topNRestConnection{
		mainRows: []map[string]any{
			{"brand": "A", "amount": 30.0},
			{"brand": "B", "amount": 20.0},
		},
		restRows: []map[string]any{{"_topn_groups": 9.0, "amount": 100.0}},
	}
	req := topNMergeReq(2)
	req.QueryOptions = map[string]any{"top_n": map[string]any{"limit": 2, "metric": "c2"}}
	data := execTopNMerge(t, conn, req)

	if len(conn.sqlCalls) != 1 {
		t.Fatalf("no rest query without the switch, got %v", conn.sqlCalls)
	}
	if axis := data.(*AxisResponse); len(axis.XAxis) != 2 {
		t.Errorf("rows must stay as returned: %v", axis.XAxis)
	}
}
