package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

// boxplotMockConnection 按调用次序应答多组行，并回显每次调用的 SQL/args。
// Capabilities 返回可注入的 caps（决定 executor 前置门是否放行）。
// failOnCall（1-based）模拟指定查询失败；executeCalls 记录 Execute 次数（前置门
// 拦下时应为 0）。
type boxplotMockConnection struct {
	rowSets     [][]map[string]any
	caps        *datasource.DialectCapabilities
	capsErr     error
	failOnCall  int
	err         error
	sqlCalls    []string
	argsCalls   [][]any
	executeCall int
}

func (m *boxplotMockConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	m.executeCall++
	m.sqlCalls = append(m.sqlCalls, sql)
	m.argsCalls = append(m.argsCalls, args)
	if m.failOnCall == m.executeCall {
		if m.err == nil {
			m.err = errors.New("boom")
		}
		return nil, m.err
	}
	idx := m.executeCall - 1
	if idx < len(m.rowSets) {
		return &datasource.QueryResult{Rows: m.rowSets[idx]}, nil
	}
	return &datasource.QueryResult{}, nil
}

func (m *boxplotMockConnection) Close() error { return nil }
func (m *boxplotMockConnection) Ping(ctx context.Context) error {
	return nil
}
func (m *boxplotMockConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}
func (m *boxplotMockConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (m *boxplotMockConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}
func (m *boxplotMockConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	return m.caps, m.capsErr
}

func boxplotFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Boxplot Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
	}
	ds := &model.Datasource{ID: 1, Name: "Test DS", Type: "postgresql"}
	return dataset, ds
}

func boxplotRequest() *ChartQueryRequest {
	return &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBoxplot,
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggCount}},
	}
}

// TestExecutor_Boxplot_ThreeQueryFlow 验证完整三查询流程：stats → Go 端算 fence
// （lower=q1-1.5*iqr, upper=q3+1.5*iqr）→ outliers list + count，参数化 fence、
// GeneratedSQL.Select 为 stats SQL、组装出正确 BoxplotResponse。
func TestExecutor_Boxplot_ThreeQueryFlow(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{
		caps: &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
		rowSets: [][]map[string]any{
			{boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))},
			{{boxOutlierValueAlias: float64(50)}},
			{{boxOutlierCountAlias: int64(1)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), boxplotRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.executeCall != 3 {
		t.Fatalf("expected 3 queries (stats/list/count), got %d: %v", conn.executeCall, conn.sqlCalls)
	}
	// q1=2,q3=4 → iqr=2 → lower=2-3=-1, upper=4+3=7
	wantArgs := []any{float64(-1), float64(7)}
	for i := range wantArgs {
		if conn.argsCalls[1][i] != wantArgs[i] {
			t.Fatalf("outliers-list arg[%d]: expected %v, got %v", i, wantArgs[i], conn.argsCalls[1][i])
		}
		if conn.argsCalls[2][i] != wantArgs[i] {
			t.Fatalf("outliers-count arg[%d]: expected %v, got %v", i, wantArgs[i], conn.argsCalls[2][i])
		}
	}
	if !strings.Contains(conn.sqlCalls[1], "LIMIT 1000") {
		t.Errorf("outliers list must carry display LIMIT: %s", conn.sqlCalls[1])
	}
	if strings.Contains(conn.sqlCalls[2], "LIMIT") {
		t.Errorf("outliers count must not carry LIMIT: %s", conn.sqlCalls[2])
	}

	resp, ok := result.Data.(*BoxplotResponse)
	if !ok {
		t.Fatalf("expected *BoxplotResponse, got %T", result.Data)
	}
	if resp.Q1 != 2 || resp.Median != 3 || resp.Q3 != 4 || resp.WhiskerLow != 1 || resp.WhiskerHigh != 9 {
		t.Fatalf("five-number mismatch: %+v", resp)
	}
	if len(resp.Outliers) != 1 || resp.Outliers[0] != 50 || resp.OutlierTotal != 1 || resp.Truncated {
		t.Fatalf("outliers mismatch: %+v", resp)
	}
	if result.Select != conn.sqlCalls[0] {
		t.Errorf("expected GeneratedSQL.Select == stats SQL, got: %s", result.Select)
	}
}

// TestExecutor_Boxplot_Truncated 验证展示截断传导到响应：total 远大于返回行数。
func TestExecutor_Boxplot_Truncated(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{
		caps: &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
		rowSets: [][]map[string]any{
			{boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))},
			{{boxOutlierValueAlias: float64(50)}},
			{{boxOutlierCountAlias: int64(5000)}},
		},
	}
	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), boxplotRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.Data.(*BoxplotResponse)
	if resp.OutlierTotal != 5000 || !resp.Truncated {
		t.Fatalf("expected total=5000 truncated=true, got total=%d truncated=%v", resp.OutlierTotal, resp.Truncated)
	}
}

// TestExecutor_Boxplot_EmptyDataSkipsLaterQueries 验证空数据集（stats 全 NULL）：
// 只跑 stats 一次，直接产退化结构，不跑后两个查询。
func TestExecutor_Boxplot_EmptyDataSkipsLaterQueries(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{
		caps:    &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
		rowSets: [][]map[string]any{{boxplotStatsRow(nil, nil, nil, nil, nil)}},
	}
	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), boxplotRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.executeCall != 1 {
		t.Fatalf("expected only stats query on empty data, got %d", conn.executeCall)
	}
	resp, ok := result.Data.(*BoxplotResponse)
	if !ok {
		t.Fatalf("expected *BoxplotResponse, got %T", result.Data)
	}
	if resp.Outliers == nil || len(resp.Outliers) != 0 || resp.Q1 != 0 || resp.Truncated {
		t.Fatalf("expected degenerate empty response, got %+v", resp)
	}
}

// TestExecutor_Boxplot_UnsupportedDialectBlocksQuery 验证 dialect 不支持 percentile
// 时前置门拦下：显式报错、Execute 一次都不调。
func TestExecutor_Boxplot_UnsupportedDialectBlocksQuery(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{
		caps: &datasource.DialectCapabilities{PercentileStrategy: "unsupported"},
	}
	_, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), boxplotRequest())
	if err == nil || !strings.Contains(err.Error(), "不支持箱线图") {
		t.Fatalf("expected boxplot-unsupported error, got %v", err)
	}
	if conn.executeCall != 0 {
		t.Fatalf("expected no Execute call when gated, got %d", conn.executeCall)
	}
}

// TestExecutor_Boxplot_CapabilityProbeError 验证 caps 探针失败时带上下文报错、不建 SQL。
func TestExecutor_Boxplot_CapabilityProbeError(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{capsErr: errors.New("probe down")}
	_, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), boxplotRequest())
	if err == nil || !strings.Contains(err.Error(), "capability probe failed") {
		t.Fatalf("expected capability probe error, got %v", err)
	}
	if conn.executeCall != 0 {
		t.Fatalf("expected no Execute call on probe failure, got %d", conn.executeCall)
	}
}

// TestExecutor_Boxplot_MissingValueField 验证空 metrics / 空字段显式报错，不跑 SQL。
func TestExecutor_Boxplot_MissingValueField(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{caps: boxplotCaps()}
	executor := NewExecutor(conn, dataset, ds)

	for _, req := range []*ChartQueryRequest{
		{DatasetID: 1, ChartType: ChartTypeBoxplot},
		{DatasetID: 1, ChartType: ChartTypeBoxplot, Metrics: []MetricConfig{{Field: "", Agg: AggCount}}},
	} {
		_, err := executor.Execute(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "boxplot requires a value field") {
			t.Fatalf("expected missing-value-field error, got %v", err)
		}
	}
	if conn.executeCall != 0 {
		t.Fatalf("expected no SQL executed, got %d", conn.executeCall)
	}
}

// TestExecutor_Boxplot_QueryErrorsPropagate 验证 stats / outliers list / count 三段
// 查询失败各自带上下文报错。
func TestExecutor_Boxplot_QueryErrorsPropagate(t *testing.T) {
	dataset, ds := boxplotFixture()
	pc := &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"}
	statsRow := boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))

	stats := &boxplotMockConnection{caps: pc, failOnCall: 1}
	if _, err := NewExecutor(stats, dataset, ds).Execute(context.Background(), boxplotRequest()); err == nil ||
		!strings.Contains(err.Error(), "boxplot stats query failed") {
		t.Fatalf("expected stats error, got %v", err)
	}

	list := &boxplotMockConnection{
		caps:       pc,
		rowSets:    [][]map[string]any{{statsRow}},
		failOnCall: 2,
	}
	if _, err := NewExecutor(list, dataset, ds).Execute(context.Background(), boxplotRequest()); err == nil ||
		!strings.Contains(err.Error(), "outliers list query failed") {
		t.Fatalf("expected list error, got %v", err)
	}

	count := &boxplotMockConnection{
		caps: pc,
		rowSets: [][]map[string]any{
			{statsRow},
			{{boxOutlierValueAlias: float64(50)}},
		},
		failOnCall: 3,
	}
	if _, err := NewExecutor(count, dataset, ds).Execute(context.Background(), boxplotRequest()); err == nil ||
		!strings.Contains(err.Error(), "outliers count query failed") {
		t.Fatalf("expected count error, got %v", err)
	}
}

// TestExecutor_Boxplot_FiltersParameterized 验证过滤值在三个查询里都参数化下传，
// 且 outliers 的 args 顺序为 [lower, upper, 过滤值...]，SQL 文本不含裸值。
func TestExecutor_Boxplot_FiltersParameterized(t *testing.T) {
	dataset, ds := boxplotFixture()
	conn := &boxplotMockConnection{
		caps: &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
		rowSets: [][]map[string]any{
			{boxplotStatsRow(float64(1), float64(2), float64(3), float64(4), float64(9))},
			{{boxOutlierValueAlias: float64(50)}},
			{{boxOutlierCountAlias: int64(1)}},
		},
	}
	req := boxplotRequest()
	req.Filters = []FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}}
	if _, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// stats args = [ok]
	if len(conn.argsCalls[0]) != 1 || conn.argsCalls[0][0] != "ok" {
		t.Fatalf("stats args: expected [ok], got %v", conn.argsCalls[0])
	}
	// outliers args = [lower, upper, ok]
	want := []any{float64(-1), float64(7), "ok"}
	if len(conn.argsCalls[1]) != len(want) {
		t.Fatalf("outliers args: expected %v, got %v", want, conn.argsCalls[1])
	}
	for i := range want {
		if conn.argsCalls[1][i] != want[i] {
			t.Fatalf("outliers arg[%d]: expected %v, got %v", i, want[i], conn.argsCalls[1][i])
		}
	}
	for _, s := range conn.sqlCalls {
		if strings.Contains(s, "'ok'") {
			t.Fatalf("filter value leaked into SQL text: %s", s)
		}
	}
}
