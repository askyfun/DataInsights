package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

// seqConnection 按调用次序返回不同行集（同环比 = 当期 + 基线两次查询）。
type seqConnection struct {
	MockConnection
	sets  [][]map[string]any
	calls int
}

func (s *seqConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	s.MockConnection.sqlCalls = append(s.MockConnection.sqlCalls, sql)
	s.MockConnection.argsCalls = append(s.MockConnection.argsCalls, args)
	idx := s.calls
	s.calls++
	if idx < len(s.sets) {
		return &datasource.QueryResult{Rows: s.sets[idx]}, nil
	}
	return &datasource.QueryResult{Rows: []map[string]any{}}, nil
}

// --- 纯函数 ---

func TestParseComparison(t *testing.T) {
	if got := parseComparison(nil); got != nil {
		t.Errorf("nil opts: %v", got)
	}
	if got := parseComparison(map[string]any{"query": 1}); got != nil {
		t.Errorf("missing key: %v", got)
	}
	if got := parseComparison(map[string]any{"comparison": "mom"}); got != nil {
		t.Errorf("non-object: %v", got)
	}
	if got := parseComparison(map[string]any{"comparison": map[string]any{"type": "wow"}}); got != nil {
		t.Errorf("unknown type: %v", got)
	}
	got := parseComparison(map[string]any{"comparison": map[string]any{"type": "MOM", "field": " 0001 "}})
	if got == nil || got.Type != "mom" || got.Field != "0001" {
		t.Errorf("valid: %+v", got)
	}
	if got := parseComparison(map[string]any{"comparison": map[string]any{"type": "yoy"}}); got == nil || got.Field != "" {
		t.Errorf("yoy no field: %+v", got)
	}
}

func TestParseBucketTime(t *testing.T) {
	for _, in := range []any{
		"2026-09-20",
		"2026-09-20 13:45:00",
		"2026-09-20T13:45:00Z",
		"2026-09-20 00:00:00",
		time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
	} {
		if _, ok := parseBucketTime(in); !ok {
			t.Errorf("expected parse ok: %v", in)
		}
	}
	for _, in := range []any{"", "abc", "2026/09/20", 42, nil, []byte("x")} {
		if _, ok := parseBucketTime(in); ok {
			t.Errorf("expected parse fail: %v", in)
		}
	}
}

func TestOffsetBucketKey(t *testing.T) {
	if got := offsetBucketKey("2026-09-20", -8, false, false); got != "2026-09-12" {
		t.Errorf("mom day: %s", got)
	}
	if got := offsetBucketKey("2026-03-01", -1, true, false); got != "2025-03-01" {
		t.Errorf("yoy: %s", got)
	}
	if got := offsetBucketKey("2024-02-29", -1, true, false); got != "2023-03-01" {
		t.Errorf("yoy leap (AddDate 归一): %s", got)
	}
	if got := offsetBucketKey("2026-09-20 10:00:00", -1, false, true); got != "2026-09-19 10:00:00" {
		t.Errorf("sec: %s", got)
	}
	if got := offsetBucketKey("华东", -3, false, false); got != "华东" {
		t.Errorf("non-date passthrough: %s", got)
	}
}

func TestWindowBounds(t *testing.T) {
	ast := &QueryAST{Filters: []FilterExpr{
		{Field: "d", FieldExpr: "d", Op: FilterGte, Value: "2026-09-01"},
		{Field: "d", FieldExpr: "d", Op: FilterLte, Value: "2026-09-07", Logic: "and"},
		{Field: "d", FieldExpr: "d", Op: FilterIsNull, Value: "", Logic: "or"},
		{Field: "x", FieldExpr: "x", Op: FilterEq, Value: "1"},
	}}
	lo, hi, hasLo, hasHi := windowBounds(ast, "d")
	if !hasLo || !hasHi {
		t.Fatalf("bounds missing: %+v", ast)
	}
	if lo.Format(dateLayoutDay) != "2026-09-01" || hi.Format(dateLayoutDay) != "2026-09-07" {
		t.Errorf("bounds: %v ~ %v", lo, hi)
	}
	// between 收紧：两窗口取交集边界
	ast2 := &QueryAST{Filters: []FilterExpr{
		{Field: "d", FieldExpr: "d", Op: FilterBetween, Value: "2026-01-01", ValueEnd: "2026-09-30"},
		{Field: "d", FieldExpr: "d", Op: FilterBetween, Value: "2026-08-01", ValueEnd: "2026-08-31"},
	}}
	lo, hi, _, _ = windowBounds(ast2, "d")
	if lo.Format(dateLayoutDay) != "2026-08-01" || hi.Format(dateLayoutDay) != "2026-08-31" {
		t.Errorf("narrowed: %v ~ %v", lo, hi)
	}
}

func TestBuildComparisonPlan(t *testing.T) {
	ast := &QueryAST{
		Dimensions: []string{"order_date"},
		Filters: []FilterExpr{
			{Field: "cid", FieldExpr: "order_date", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-20"},
		},
	}
	plan, err := buildComparisonPlan(&comparisonConfig{Type: "mom"}, ast, nil)
	if err != nil {
		t.Fatal(err)
	}
	// span=19 天 → 平移 20：[2026-08-12, 2026-08-31]
	if plan.LowerKey != "2026-08-12" || plan.UpperKey != "2026-08-31" || plan.DeltaDays != 20 {
		t.Errorf("mom plan: %+v", plan)
	}
	plan, err = buildComparisonPlan(&comparisonConfig{Type: "yoy"}, ast, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.LowerKey != "2025-09-01" || plan.UpperKey != "2025-09-20" || !plan.YoY {
		t.Errorf("yoy plan: %+v", plan)
	}

	// 单侧边界：用当期结果轴补齐（gte 开放 + 数据 [09-14, 09-20]）
	astOpen := &QueryAST{
		Dimensions: []string{"order_date"},
		Filters: []FilterExpr{
			{Field: "cid", FieldExpr: "order_date", Op: FilterGte, Value: "2026-09-14"},
		},
	}
	plan, err = buildComparisonPlan(&comparisonConfig{Type: "mom"}, astOpen, []string{"2026-09-14", "2026-09-20"})
	if err != nil {
		t.Fatal(err)
	}
	// span=6 → 平移 7 → 基线窗 [09-07, 09-13]
	if plan.LowerKey != "2026-09-07" || plan.UpperKey != "2026-09-13" {
		t.Errorf("synthesized window: %+v", plan)
	}

	// 完全无边界 → 显式报错
	if _, err := buildComparisonPlan(&comparisonConfig{Type: "mom"}, &QueryAST{Dimensions: []string{"d"}}, nil); err == nil {
		t.Error("expected open-window error")
	}
	// 时刻级上界 → 秒级窗口
	astSec := &QueryAST{
		Dimensions: []string{"d"},
		Filters: []FilterExpr{
			{Field: "d", FieldExpr: "d", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-02 12:00:00"},
		},
	}
	plan, err = buildComparisonPlan(&comparisonConfig{Type: "mom"}, astSec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Sec || plan.UpperKey != "2026-08-31 12:00:00" || plan.LowerKey != "2026-08-30" {
		t.Errorf("sec plan: %+v", plan)
	}
}

func TestComparisonGrowth(t *testing.T) {
	if got := comparisonGrowth(120.0, 100.0); got != 20.0 {
		t.Errorf("growth: %v", got)
	}
	if got := comparisonGrowth(90.0, 100.0); got != -10.0 {
		t.Errorf("decline: %v", got)
	}
	if got := comparisonGrowth(10.0, 0.0); got != nil {
		t.Errorf("zero prev: %v", got)
	}
	if got := comparisonGrowth(10.0, nil); got != nil {
		t.Errorf("missing prev: %v", got)
	}
	if got := comparisonGrowth(nil, 5.0); got != nil {
		t.Errorf("missing cur: %v", got)
	}
	// 负基数：分母取绝对值，方向由差值决定
	if got := comparisonGrowth(-50.0, -100.0); got != 50.0 {
		t.Errorf("negative base: %v", got)
	}
}

// --- executor 集成 ---

func comparisonExecutor(t *testing.T, conn datasource.Connection) *Executor {
	t.Helper()
	dataset := &model.Dataset{
		ID:        1,
		Name:      "orders",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		Columns:   `[{"id":"c1","name":"order_date","expr":"order_date","type":"date"},{"id":"c2","name":"amount","expr":"amount","type":"numeric"}]`,
	}
	ds := &model.Datasource{ID: 1, Name: "pg", Type: "postgresql"}
	return NewExecutor(conn, dataset, ds)
}

func TestExecutor_ComparisonMomAxis(t *testing.T) {
	conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
		// 当期
		{{"order_date": "2026-09-01", "amount": 100.0}, {"order_date": "2026-09-02", "amount": 120.0}},
		// 基线（窗口平移后）：08-30 对齐到 09-01（+2 天）；09-01 缺失 → null；
		// 08-31 对齐到 09-02；09-02 是窗口右溢出的行，应被忽略。
		{{"order_date": "2026-08-30", "amount": 50.0}, {"order_date": "2026-08-31", "amount": 0.0}, {"order_date": "2026-09-02", "amount": 999.0}},
	}}
	e := comparisonExecutor(t, conn)
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"c1"},
		Metrics:   []MetricConfig{{Field: "c2", Agg: AggSum}},
		Filters: []FilterConfig{
			{Field: "c1", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-02", Logic: "and"},
		},
		QueryOptions: map[string]any{"comparison": map[string]any{"type": "mom", "field": "c1"}},
	}
	res, err := e.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	axis := res.Data.(*AxisResponse)
	if len(axis.Series) != 3 {
		t.Fatalf("series: %+v", axis.Series)
	}
	if axis.Series[1].Name != "amount(上期)" || axis.Series[2].Name != "amount(增长率%)" {
		t.Errorf("series names: %v %v", axis.Series[1].Name, axis.Series[2].Name)
	}
	prior := axis.Series[1].Data
	if prior[0] == nil || toF(t, prior[0]) != 50.0 {
		t.Errorf("prior[0]: %v", prior[0])
	}
	if prior[1] == nil || toF(t, prior[1]) != 0.0 {
		t.Errorf("prior[1]: %v", prior[1])
	}
	growth := axis.Series[2].Data
	if toF(t, growth[0]) != 100.0 {
		t.Errorf("growth[0]: %v", growth[0])
	}
	if growth[1] != nil {
		t.Errorf("growth on zero prev must be nil: %v", growth[1])
	}
	// 基线 SQL 的窗口：span=1 → delta=2 → [2026-08-30, 2026-08-31]
	if len(conn.argsCalls) != 2 {
		t.Fatalf("two queries expected: %v", conn.sqlCalls)
	}
	if conn.argsCalls[1][0] != "2026-08-30" || conn.argsCalls[1][1] != "2026-08-31" {
		t.Errorf("baseline window args: %v", conn.argsCalls[1])
	}
	if !strings.Contains(conn.sqlCalls[1], "BETWEEN") || strings.Contains(conn.sqlCalls[1], "LIMIT") {
		t.Errorf("baseline sql: %s", conn.sqlCalls[1])
	}
}

func TestExecutor_ComparisonYoyTable(t *testing.T) {
	conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
		{{"order_date": "2026-09-01", "amount": 110.0}},
		{{"order_date": "2025-09-01", "amount": 100.0}},
	}}
	e := comparisonExecutor(t, conn)
	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeTable,
		Dims:      []string{"c1"},
		Metrics:   []MetricConfig{{Field: "c2", Agg: AggSum}},
		Filters: []FilterConfig{
			{Field: "c1", Op: FilterGte, Value: "2026-09-01", Logic: "and"},
			{Field: "c1", Op: FilterLte, Value: "2026-09-01", Logic: "and"},
		},
		QueryOptions: map[string]any{"comparison": map[string]any{"type": "yoy"}},
	}
	res, err := e.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	table := res.Data.(*TableResponse)
	if len(table.Columns) != 4 || table.Columns[2] != "amount(上期)" || table.Columns[3] != "amount(增长率%)" {
		t.Fatalf("columns: %v", table.Columns)
	}
	row := table.Data[0]
	if toF(t, row["amount(上期)"]) != 100.0 {
		t.Errorf("prev col: %v", row)
	}
	if toF(t, row["amount(增长率%)"]) != 10.0 {
		t.Errorf("growth col: %v", row)
	}
	// yoy 基线窗口按日历年
	if conn.argsCalls[1][0] != "2025-09-01" || conn.argsCalls[1][1] != "2025-09-01" {
		t.Errorf("yoy window args: %v", conn.argsCalls[1])
	}
}

func TestExecutor_ComparisonErrors(t *testing.T) {
	base := func() *ChartQueryRequest {
		return &ChartQueryRequest{
			DatasetID: 1,
			ChartType: ChartTypeBar,
			Dims:      []string{"c1"},
			Metrics:   []MetricConfig{{Field: "c2", Agg: AggSum}},
			QueryOptions: map[string]any{
				"comparison": map[string]any{"type": "mom", "field": "c1"},
			},
		}
	}
	t.Run("单侧边界按数据轴补窗", func(t *testing.T) {
		conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
			{{"order_date": "2026-09-14", "amount": 10.0}, {"order_date": "2026-09-20", "amount": 20.0}},
			{{"order_date": "2026-09-07", "amount": 1.0}, {"order_date": "2026-09-13", "amount": 2.0}},
		}}
		e := comparisonExecutor(t, conn)
		req := base()
		req.Filters = []FilterConfig{{Field: "c1", Op: FilterGte, Value: "2026-09-14", Logic: "and"}}
		res, err := e.Execute(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		// 数据轴 [09-14, 09-20] → span 6 → 平移 7 → 基线窗 [09-07, 09-13]
		if conn.argsCalls[1][0] != "2026-09-07" || conn.argsCalls[1][1] != "2026-09-13" {
			t.Errorf("synthesized window: %v", conn.argsCalls[1])
		}
		prior := res.Data.(*AxisResponse).Series[1].Data
		if toF(t, prior[0]) != 1.0 || toF(t, prior[1]) != 2.0 {
			t.Errorf("synthesized prior: %v", prior)
		}
	})
	t.Run("既无边界又无数据才报错", func(t *testing.T) {
		conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
			{},
		}}
		e := comparisonExecutor(t, conn)
		if _, err := e.Execute(context.Background(), base()); err == nil {
			t.Fatal("expected open-window error")
		} else if !strings.Contains(err.Error(), "comparison") {
			t.Errorf("error text: %v", err)
		}
	})
	t.Run("多维度报错", func(t *testing.T) {
		conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
			{{"order_date": "2026-09-01", "amount": 1.0}},
		}}
		e := comparisonExecutor(t, conn)
		req := base()
		req.Dims = []string{"c1", "c2"}
		req.Filters = []FilterConfig{{Field: "c1", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-02"}}
		if _, err := e.Execute(context.Background(), req); err == nil {
			t.Fatal("expected multi-dim error")
		}
	})
	t.Run("不支持图型报错", func(t *testing.T) {
		conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
			{{"order_date": "2026-09-01", "amount": 1.0}},
		}}
		e := comparisonExecutor(t, conn)
		req := base()
		req.ChartType = ChartTypeScatter
		req.Filters = []FilterConfig{{Field: "c1", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-02"}}
		if _, err := e.Execute(context.Background(), req); err == nil {
			t.Fatal("expected unsupported chart type error")
		}
	})
	t.Run("未启用时零基线查询", func(t *testing.T) {
		conn := &seqConnection{MockConnection: MockConnection{}, sets: [][]map[string]any{
			{{"order_date": "2026-09-01", "amount": 1.0}},
		}}
		e := comparisonExecutor(t, conn)
		req := base()
		req.QueryOptions = nil
		req.Filters = []FilterConfig{{Field: "c1", Op: FilterBetween, Value: "2026-09-01", ValueEnd: "2026-09-02"}}
		res, err := e.Execute(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Data.(*AxisResponse).Series) != 1 || conn.calls != 1 {
			t.Errorf("no comparison expected: calls=%d", conn.calls)
		}
	})
}

func toF(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := toFloat64(v)
	if !ok {
		t.Fatalf("not numeric: %#v", v)
	}
	return f
}
