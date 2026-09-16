package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"dataray/internal/datasource"
	"dataray/internal/model"
)

// histogramMockConnection 按调用次序应答多组行的 Connection mock：histogram 是
// 两阶段查询，阶段1（stats）与阶段2（bins）各吃一组预设行；记录每次调用的
// SQL 与 args 供断言。failOnCall（1-based）模拟指定阶段的查询失败。
type histogramMockConnection struct {
	rowSets    [][]map[string]any
	failOnCall int
	err        error
	sqlCalls   []string
	argsCalls  [][]any
}

func (m *histogramMockConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	m.sqlCalls = append(m.sqlCalls, sql)
	m.argsCalls = append(m.argsCalls, args)
	if m.failOnCall == len(m.sqlCalls) {
		if m.err == nil {
			m.err = errors.New("boom")
		}
		return nil, m.err
	}
	idx := len(m.sqlCalls) - 1
	if idx < len(m.rowSets) {
		return &datasource.QueryResult{Rows: m.rowSets[idx]}, nil
	}
	return &datasource.QueryResult{}, nil
}

func (m *histogramMockConnection) Close() error { return nil }
func (m *histogramMockConnection) Ping(ctx context.Context) error {
	return nil
}
func (m *histogramMockConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}
func (m *histogramMockConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (m *histogramMockConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}
func (m *histogramMockConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}

func histogramFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Histogram Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}
	ds := &model.Datasource{ID: 1, Name: "Test DS", Type: "postgresql"}
	return dataset, ds
}

// histogramRequest 构造 v1 平铺请求（histogram 的被分箱字段 = Metrics[0].Field）。
func histogramRequest(queryOptions map[string]any, metrics ...MetricConfig) *ChartQueryRequest {
	if len(metrics) == 0 {
		metrics = []MetricConfig{{Field: "amount", Agg: AggCount}}
	}
	return &ChartQueryRequest{
		DatasetID:    1,
		ChartType:    ChartTypeHistogram,
		Metrics:      metrics,
		QueryOptions: queryOptions,
	}
}

func histogramStatsRow(mn, mx any, cnt any) map[string]any {
	return map[string]any{"mn": mn, "mx": mx, "cnt": cnt}
}

// TestExecutor_Histogram_TwoPhaseFlow 验证完整两阶段流程：bin_count 从
// req.QueryOptions 流入（plumbing 末端）、阶段2 的 min/binWidth 是参数化
// float args（binWidth=(mx-mn)/bin_count）、GeneratedSQL.Select 为阶段2 SQL、
// bins 组装正确。
func TestExecutor_Histogram_TwoPhaseFlow(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(10), int64(100))},
			{
				{"bin": float64(0), "cnt": int64(60)},
				{"bin": float64(1), "cnt": int64(40)},
			},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(map[string]any{"bin_count": float64(2)}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.sqlCalls) != 2 {
		t.Fatalf("expected 2 phases, got %d: %v", len(conn.sqlCalls), conn.sqlCalls)
	}
	if !strings.Contains(conn.sqlCalls[0], `SELECT MIN(amount) AS "mn", MAX(amount) AS "mx", COUNT(*) AS "cnt" FROM orders`) {
		t.Errorf("phase-1 SQL unexpected: %s", conn.sqlCalls[0])
	}
	if !strings.Contains(conn.sqlCalls[1], `SELECT FLOOR((amount - ?) / ?) AS "bin"`) {
		t.Errorf("phase-2 SQL unexpected: %s", conn.sqlCalls[1])
	}
	// bin_count=2 → binWidth=(10-0)/2=5，min/binWidth 为参数化 args（内层派生表一份）
	wantArgs := []any{float64(0), float64(5)}
	if len(conn.argsCalls[1]) != len(wantArgs) {
		t.Fatalf("phase-2 args: expected %v, got %v", wantArgs, conn.argsCalls[1])
	}
	for i := range wantArgs {
		if conn.argsCalls[1][i] != wantArgs[i] {
			t.Fatalf("phase-2 args[%d]: expected %v, got %v", i, wantArgs[i], conn.argsCalls[1][i])
		}
	}
	if result.Select != conn.sqlCalls[1] {
		t.Errorf("expected GeneratedSQL.Select == phase-2 SQL, got: %s", result.Select)
	}

	resp, ok := result.Data.(*HistogramResponse)
	if !ok {
		t.Fatalf("expected *HistogramResponse, got %T", result.Data)
	}
	wantBins := []HistogramBin{
		{BinStart: 0, BinEnd: 5, Count: 60},
		{BinStart: 5, BinEnd: 10, Count: 40},
	}
	if len(resp.Bins) != 2 || resp.Bins[0] != wantBins[0] || resp.Bins[1] != wantBins[1] {
		t.Fatalf("expected bins %+v, got %+v", wantBins, resp.Bins)
	}
}

// TestExecutor_Histogram_DefaultBinCount 验证 query_options 缺省时 bin_count=20：
// binWidth=(100-0)/20=5，补全 20 个 bin。
func TestExecutor_Histogram_DefaultBinCount(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(100), int64(50))},
			nil, // 阶段2 无行：全部 bin 补 0
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.argsCalls[1][1] != float64(5) {
		t.Errorf("expected default bin_count=20 → binWidth=(100-0)/20=5, got arg %v", conn.argsCalls[1][1])
	}
	resp := result.Data.(*HistogramResponse)
	if len(resp.Bins) != 20 {
		t.Fatalf("expected 20 bins, got %d", len(resp.Bins))
	}
	for i, b := range resp.Bins {
		if b.Count != 0 {
			t.Errorf("bin[%d]: expected empty-bin fill count=0, got %d", i, b.Count)
		}
	}
}

// TestExecutor_Histogram_UserBinWidthOverrides 验证用户 bin_width 覆盖
// bin_count 推算的宽度：numBins=ceil((mx-mn)/bin_width)。
func TestExecutor_Histogram_UserBinWidthOverrides(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(10), int64(8))},
			{{"bin": float64(0), "cnt": int64(2)}, {"bin": float64(3), "cnt": int64(6)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	// bin_count=100 会被 bin_width=2.5 覆盖：numBins=ceil(10/2.5)=4
	result, err := executor.Execute(context.Background(), histogramRequest(map[string]any{
		"bin_count": float64(100),
		"bin_width": 2.5,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.argsCalls[1][1] != 2.5 {
		t.Errorf("expected user bin_width 2.5 as parameterized arg, got %v", conn.argsCalls[1][1])
	}
	resp := result.Data.(*HistogramResponse)
	if len(resp.Bins) != 4 {
		t.Fatalf("expected 4 bins (ceil(10/2.5)), got %d: %+v", len(resp.Bins), resp.Bins)
	}
	var sum int64
	for _, b := range resp.Bins {
		sum += b.Count
	}
	if sum != 8 {
		t.Errorf("expected sum(counts)==8, got %d", sum)
	}
}

// assertHistogramBinsClampedContiguous 断言钳制后的 bins 仍正确：箱数 ==
// maxHistogramBins、连续铺满 [mn, mx]（BinEnd[i]==BinStart[i+1]，首 BinStart==mn、
// 末 BinEnd==mx）、sum(count)==total（钳制不破坏不变式）。
func assertHistogramBinsClampedContiguous(t *testing.T, resp *HistogramResponse, mn, mx float64, wantTotal int64) {
	t.Helper()
	if len(resp.Bins) != maxHistogramBins {
		t.Fatalf("expected numBins clamped to %d, got %d", maxHistogramBins, len(resp.Bins))
	}
	if resp.Bins[0].BinStart != mn {
		t.Errorf("expected first BinStart==%v, got %v", mn, resp.Bins[0].BinStart)
	}
	if last := resp.Bins[len(resp.Bins)-1]; last.BinEnd != mx {
		t.Errorf("expected last BinEnd==%v, got %v", mx, last.BinEnd)
	}
	var sum int64
	for i, b := range resp.Bins {
		if i > 0 && b.BinStart != resp.Bins[i-1].BinEnd {
			t.Fatalf("bins not contiguous at %d: prev BinEnd=%v, BinStart=%v", i, resp.Bins[i-1].BinEnd, b.BinStart)
		}
		sum += b.Count
	}
	if sum != wantTotal {
		t.Errorf("expected sum(counts)==%d, got %d", wantTotal, sum)
	}
}

// TestExecutor_Histogram_AbsurdBinCountClamped 验证 bin_count 上限钳制：请求体
// bin_count=100000000 被钳到 maxHistogramBins（防 ProcessBins 无界分配），
// binWidth=(mx-mn)/钳定箱数，bins 仍连续铺满 [mn, mx]、sum(count)==total。
func TestExecutor_Histogram_AbsurdBinCountClamped(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(100000), int64(50))},
			{{"bin": float64(0), "cnt": int64(20)}, {"bin": float64(maxHistogramBins - 1), "cnt": int64(30)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(map[string]any{"bin_count": float64(100000000)}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 阶段2 参数化 binWidth 按钳定箱数计算：(100000-0)/maxHistogramBins=10
	if conn.argsCalls[1][1] != float64(10) {
		t.Errorf("expected clamped binWidth=(mx-mn)/maxHistogramBins=10, got arg %v", conn.argsCalls[1][1])
	}
	resp := result.Data.(*HistogramResponse)
	assertHistogramBinsClampedContiguous(t, resp, 0, 100000, 50)
}

// TestExecutor_Histogram_TinyBinWidthClamped 验证 bin_width 过小路径的钳制：
// bin_width=1e-9 over [0, 1e6] → ceil=1e15，钳到 maxHistogramBins，钳后重算
// 宽度=(mx-mn)/maxHistogramBins，bins 仍连续铺满 [mn, mx]、sum(count)==total。
func TestExecutor_Histogram_TinyBinWidthClamped(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(1e6), int64(7))},
			{{"bin": float64(0), "cnt": int64(3)}, {"bin": float64(maxHistogramBins - 1), "cnt": int64(4)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(map[string]any{"bin_width": 1e-9}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 阶段2 参数化 binWidth 按钳定箱数重算：1e6/maxHistogramBins=100（不再是 1e-9）
	if conn.argsCalls[1][1] != float64(100) {
		t.Errorf("expected recomputed binWidth=(mx-mn)/maxHistogramBins=100, got arg %v", conn.argsCalls[1][1])
	}
	resp := result.Data.(*HistogramResponse)
	assertHistogramBinsClampedContiguous(t, resp, 0, 1e6, 7)
}

// TestExecutor_Histogram_EmptyData 验证 total==0 边界：不跑阶段2，返回空
// （非 nil）Bins，不报错。
func TestExecutor_Histogram_EmptyData(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(nil, nil, int64(0))},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.sqlCalls) != 1 {
		t.Fatalf("expected phase-2 to be skipped, got %d calls", len(conn.sqlCalls))
	}
	resp, ok := result.Data.(*HistogramResponse)
	if !ok {
		t.Fatalf("expected *HistogramResponse, got %T", result.Data)
	}
	if resp.Bins == nil || len(resp.Bins) != 0 {
		t.Fatalf("expected empty non-nil Bins, got %+v", resp.Bins)
	}
}

// TestExecutor_Histogram_SingleValue 验证 mx==mn 边界：不除 0，binWidth 兜底 1，
// 产单个 bin [mn, mn+1)，count=total。
func TestExecutor_Histogram_SingleValue(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(7), float64(7), int64(3))},
			{{"bin": float64(0), "cnt": int64(3)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), histogramRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantArgs := []any{float64(7), float64(1)}
	for i := range wantArgs {
		if conn.argsCalls[1][i] != wantArgs[i] {
			t.Fatalf("phase-2 args[%d]: expected %v, got %v", i, wantArgs[i], conn.argsCalls[1][i])
		}
	}
	resp := result.Data.(*HistogramResponse)
	if len(resp.Bins) != 1 || resp.Bins[0] != (HistogramBin{BinStart: 7, BinEnd: 8, Count: 3}) {
		t.Fatalf("expected single bin [7,8) count=3, got %+v", resp.Bins)
	}
}

// TestExecutor_Histogram_AllNullColumn 验证有行但 MIN/MAX 为 NULL（值列全
// NULL）时显式报错，不静默产出空图。
func TestExecutor_Histogram_AllNullColumn(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(nil, nil, int64(5))},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	_, err := executor.Execute(context.Background(), histogramRequest(nil))
	if err == nil || !strings.Contains(err.Error(), "no numeric values") {
		t.Fatalf("expected explicit no-numeric-values error, got %v", err)
	}
}

// TestExecutor_Histogram_MissingValueField 验证空 metrics 显式报错（不静默）。
func TestExecutor_Histogram_MissingValueField(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{}
	executor := NewExecutor(conn, dataset, ds)

	for _, req := range []*ChartQueryRequest{
		{DatasetID: 1, ChartType: ChartTypeHistogram}, // 无 metrics
		histogramRequest(nil, MetricConfig{Field: "", Agg: AggCount}),
	} {
		_, err := executor.Execute(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "histogram requires a value field") {
			t.Fatalf("expected explicit missing-value-field error, got %v", err)
		}
	}
	if len(conn.sqlCalls) != 0 {
		t.Fatalf("expected no SQL executed, got %v", conn.sqlCalls)
	}
}

// TestExecutor_Histogram_QueryErrorsPropagate 验证阶段1/阶段2 查询失败都带
// 上下文返回错误（无空错误块）。
func TestExecutor_Histogram_QueryErrorsPropagate(t *testing.T) {
	dataset, ds := histogramFixture()

	phase1 := &histogramMockConnection{failOnCall: 1}
	_, err := NewExecutor(phase1, dataset, ds).Execute(context.Background(), histogramRequest(nil))
	if err == nil || !strings.Contains(err.Error(), "histogram stats query failed") {
		t.Fatalf("expected phase-1 error, got %v", err)
	}

	phase2 := &histogramMockConnection{
		rowSets:    [][]map[string]any{{histogramStatsRow(float64(0), float64(10), int64(5))}},
		failOnCall: 2,
	}
	_, err = NewExecutor(phase2, dataset, ds).Execute(context.Background(), histogramRequest(nil))
	if err == nil || !strings.Contains(err.Error(), "histogram bin query failed") {
		t.Fatalf("expected phase-2 error, got %v", err)
	}
}

// TestExecutor_Histogram_FiltersParameterizedBothPhases 验证过滤条件在两个阶段
// 都参数化下传，且阶段2 的 args 顺序与 ? 占位一一对应
// （[min, width, 过滤值...]，内层派生表持占位、GROUP BY 引用派生列不再吃参数）。
func TestExecutor_Histogram_FiltersParameterizedBothPhases(t *testing.T) {
	dataset, ds := histogramFixture()
	conn := &histogramMockConnection{
		rowSets: [][]map[string]any{
			{histogramStatsRow(float64(0), float64(10), int64(4))},
			{{"bin": float64(0), "cnt": int64(4)}},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := histogramRequest(nil)
	req.Filters = []FilterConfig{{Field: "status", Op: FilterEq, Value: "ok"}}
	// 补全 bin 需要 numBins=20（默认），此处只断言 args 形态
	_, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.argsCalls[0]) != 1 || conn.argsCalls[0][0] != "ok" {
		t.Fatalf("phase-1 args: expected [ok], got %v", conn.argsCalls[0])
	}
	width := 10.0 / 20 // 默认 bin_count=20
	wantArgs := []any{float64(0), width, "ok"}
	if len(conn.argsCalls[1]) != len(wantArgs) {
		t.Fatalf("phase-2 args: expected %v, got %v", wantArgs, conn.argsCalls[1])
	}
	for i := range wantArgs {
		if conn.argsCalls[1][i] != wantArgs[i] {
			t.Fatalf("phase-2 args[%d]: expected %v, got %v", i, wantArgs[i], conn.argsCalls[1][i])
		}
	}
	for _, s := range conn.sqlCalls {
		if strings.Contains(s, "'ok'") {
			t.Fatalf("filter value leaked into SQL text: %s", s)
		}
	}
}
