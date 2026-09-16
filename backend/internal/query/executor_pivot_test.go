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

// pivotMockConnection 是可配置 Capabilities 的 Connection mock：
// executor 的 pivot v1/v2 处理器选择依赖运行时 caps.SupportsGroupingSets，
// 现有 MockConnection（executor_full_test.go）的 caps 恒为 false，无法覆盖 v2 分支。
type pivotMockConnection struct {
	rows      []map[string]any
	caps      *datasource.DialectCapabilities
	capsErr   error
	queryErr  error
	sqlCalls  []string
	argsCalls [][]any
	capsCalls int
}

func (m *pivotMockConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	m.sqlCalls = append(m.sqlCalls, sql)
	m.argsCalls = append(m.argsCalls, args)
	return &datasource.QueryResult{Rows: m.rows}, nil
}

func (m *pivotMockConnection) Close() error { return nil }
func (m *pivotMockConnection) Ping(ctx context.Context) error {
	return nil
}
func (m *pivotMockConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}
func (m *pivotMockConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (m *pivotMockConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}
func (m *pivotMockConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	m.capsCalls++
	if m.capsErr != nil {
		return nil, m.capsErr
	}
	if m.caps != nil {
		return m.caps, nil
	}
	return &datasource.DialectCapabilities{}, nil
}

func pivotFixture() (*model.Dataset, *model.Datasource) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Pivot Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}
	ds := &model.Datasource{ID: 1, Name: "Test DS", Type: "postgresql"}
	return dataset, ds
}

// pivotV2Request 构造 service 层 v2 管道同款请求：PlannedAST 经 PlanAST 携带
// rows/columns 槽位，Dims/Metrics 为同源平铺参数。
func pivotV2Request() *ChartQueryRequest {
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotColumns},
		},
		Metrics: []MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
	}
	return &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypePivot,
		Dims:       []string{"region", "product"},
		Metrics:    []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		PlannedAST: NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec),
	}
}

func pivotGroupingSetsRows() []map[string]any {
	return []map[string]any{
		{"region": "E", "product": "A", "total": 100.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "E", "product": nil, "total": 100.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(1)},
		{"region": nil, "product": nil, "total": 100.0, "__pivot_row_grp_0": int64(1), "__pivot_col_grp_0": int64(1)},
	}
}

// TestExecutor_Pivot_GroupingSetsPath 验证 SupportsGroupingSets=true + v2 槽位请求
// 走 GROUPING SETS SQL + PivotProcessorV2（返回 *PivotResponseV2）。
func TestExecutor_Pivot_GroupingSetsPath(t *testing.T) {
	dataset, ds := pivotFixture()
	conn := &pivotMockConnection{
		rows: pivotGroupingSetsRows(),
		caps: &datasource.DialectCapabilities{SupportsGroupingSets: true},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), pivotV2Request())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn.capsCalls != 1 {
		t.Errorf("expected exactly 1 Capabilities call, got %d", conn.capsCalls)
	}
	if len(conn.sqlCalls) != 1 {
		t.Fatalf("expected exactly 1 Execute call, got %d", len(conn.sqlCalls))
	}
	if !strings.Contains(result.Select, "GROUP BY GROUPING SETS ((region, product), (region), ())") {
		t.Errorf("expected GROUPING SETS SQL, got: %s", result.Select)
	}

	pv, ok := result.Data.(*PivotResponseV2)
	if !ok {
		t.Fatalf("expected *PivotResponseV2, got %T", result.Data)
	}
	if len(pv.Cells) != 2 {
		t.Fatalf("expected 2 cells (detail + subtotal), got %d", len(pv.Cells))
	}
	if pv.Cells[0].Values[PivotValueKey("A", "total")] != 100 {
		t.Errorf("expected detail value A|total=100, got %v", pv.Cells[0].Values)
	}
	if !pv.Cells[1].IsSubtotal || pv.Cells[1].Values[PivotValueKey(PivotSubtotalColKey, "total")] != 100 {
		t.Errorf("expected subtotal cell with sentinel key, got %+v", pv.Cells[1])
	}
	if pv.GrandTotal == nil || pv.GrandTotal.Values[PivotValueKey(PivotSubtotalColKey, "total")] != 100 {
		t.Errorf("expected grand total 100, got %+v", pv.GrandTotal)
	}
}

// TestExecutor_Pivot_FallbackWhenGroupingSetsUnsupported 验证 controller 裁定的过渡兜底：
// SupportsGroupingSets=false（MySQL/StarRocks stub）时行为与改动前完全一致——
// 通用 SQL（无 GROUPING SETS）+ 旧 PivotProcessor（*PivotResponse 平铺透传）。
func TestExecutor_Pivot_FallbackWhenGroupingSetsUnsupported(t *testing.T) {
	dataset, ds := pivotFixture()
	flatRows := []map[string]any{
		{"region": "E", "product": "A", "total": 100.0},
		{"region": "W", "product": "B", "total": 40.0},
	}
	conn := &pivotMockConnection{
		rows: flatRows,
		caps: &datasource.DialectCapabilities{SupportsGroupingSets: false},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), pivotV2Request())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.Select, "GROUPING") {
		t.Errorf("fallback path must not use GROUPING SETS SQL, got: %s", result.Select)
	}
	pr, ok := result.Data.(*PivotResponse)
	if !ok {
		t.Fatalf("expected legacy *PivotResponse, got %T", result.Data)
	}
	if len(pr.Data) != 2 {
		t.Errorf("expected passthrough of 2 rows, got %d", len(pr.Data))
	}
}

// TestExecutor_Pivot_FallbackWhenV1Request 验证 v1 平铺请求（PlannedAST 为 nil，
// AST 无 DimensionExprs/槽位）即使数据源支持 GROUPING SETS 也走旧路径，行为不变。
func TestExecutor_Pivot_FallbackWhenV1Request(t *testing.T) {
	dataset, ds := pivotFixture()
	conn := &pivotMockConnection{
		rows: []map[string]any{{"region": "E", "product": "A", "total": 100.0}},
		caps: &datasource.DialectCapabilities{SupportsGroupingSets: true},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePivot,
		Dims:      []string{"region", "product"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		// PlannedAST 为 nil：executor 内部走 qb.Build（无槽位信息）
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Select, "GROUPING") {
		t.Errorf("v1 request must not use GROUPING SETS SQL, got: %s", result.Select)
	}
	if _, ok := result.Data.(*PivotResponse); !ok {
		t.Fatalf("expected legacy *PivotResponse for v1 request, got %T", result.Data)
	}
}

// TestExecutor_Pivot_FallbackWhenNoColumnsSlot 验证 v1 协议经 PlanAST（所有维度
// GroupName="rows"，无 columns 组）时同样回退旧路径。
func TestExecutor_Pivot_FallbackWhenNoColumnsSlot(t *testing.T) {
	dataset, ds := pivotFixture()
	conn := &pivotMockConnection{
		rows: []map[string]any{{"region": "E", "product": "A", "total": 100.0}},
		caps: &datasource.DialectCapabilities{SupportsGroupingSets: true},
	}
	executor := NewExecutor(conn, dataset, ds)

	// 模拟 v1 适配器：ChartSpecFromRequest 的 defaultDimGroupName 把 pivot 所有维度标成 "rows"
	spec := &QuerySpec{
		Dimensions: []DimensionExpr{
			{Field: "region", GroupName: SlotRows},
			{Field: "product", GroupName: SlotRows},
		},
		Metrics: []MetricExpr2{{Field: "amount", Agg: AggSum, Alias: "total", GroupName: "values"}},
	}
	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypePivot,
		Dims:       []string{"region", "product"},
		Metrics:    []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		PlannedAST: NewQueryPlanner().PlanAST("orders", SourceTypeTable, spec),
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Select, "GROUPING") {
		t.Errorf("rows-only slots must fall back to legacy path, got: %s", result.Select)
	}
	if _, ok := result.Data.(*PivotResponse); !ok {
		t.Fatalf("expected legacy *PivotResponse, got %T", result.Data)
	}
}

// TestExecutor_Pivot_CapabilitiesError 验证能力探测失败时显式报错（禁止空错误块/静默降级）。
func TestExecutor_Pivot_CapabilitiesError(t *testing.T) {
	dataset, ds := pivotFixture()
	conn := &pivotMockConnection{capsErr: errors.New("probe timeout")}
	executor := NewExecutor(conn, dataset, ds)

	_, err := executor.Execute(context.Background(), pivotV2Request())
	if err == nil {
		t.Fatal("expected error when Capabilities probe fails, got nil")
	}
	if !strings.Contains(err.Error(), "capabilities probe failed") {
		t.Errorf("expected capabilities probe error, got: %v", err)
	}
	if len(conn.sqlCalls) != 0 {
		t.Errorf("no query should run after probe failure, got %v", conn.sqlCalls)
	}
}

// TestExecutor_Pivot_QueryError 验证 GROUPING SETS 查询执行失败时返回错误。
func TestExecutor_Pivot_QueryError(t *testing.T) {
	dataset, ds := pivotFixture()
	conn := &pivotMockConnection{
		caps:     &datasource.DialectCapabilities{SupportsGroupingSets: true},
		queryErr: errors.New("database error"),
	}
	executor := NewExecutor(conn, dataset, ds)

	if _, err := executor.Execute(context.Background(), pivotV2Request()); err == nil {
		t.Fatal("expected query error, got nil")
	}
}
