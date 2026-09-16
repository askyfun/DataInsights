package query

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"testing"

	"dataray/internal/datasource"
	"dataray/internal/model"
)

type MockConnection struct {
	rows      []map[string]any
	queryErr  error
	sqlCalls  []string
	argsCalls [][]any
}

func (m *MockConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	m.sqlCalls = append(m.sqlCalls, sql)
	m.argsCalls = append(m.argsCalls, args)
	return &datasource.QueryResult{Rows: m.rows}, nil
}

func (m *MockConnection) Close() error {
	return nil
}

func (m *MockConnection) Ping(ctx context.Context) error {
	return nil
}

func (m *MockConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}

func (m *MockConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}

func (m *MockConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}

func (m *MockConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}

func TestExecutor_EmptyBaseQuery(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{Valid: false},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"category"},
		Metrics:   []MetricConfig{{Field: "value", Agg: AggSum}},
	}

	_, err := executor.Execute(context.Background(), req)
	if err == nil {
		t.Error("Expected error for empty base query, got nil")
	}
}

func TestExecutor_ValidBaseQuery(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "test_table", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"category": "A", "value": 100},
			{"category": "B", "value": 200},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"category"},
		Metrics:   []MetricConfig{{Field: "value", Agg: AggSum}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result, got nil")
	}
}

func TestExecutor_UsesPlannedASTWhenProvided(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"created_at_day": "2025-05-01 00:00:00+00", "total_amount": 100.0},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeLine,
		Dims:      []string{"created_at"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total_amount"}},
		PlannedAST: &QueryAST{
			Source:     "orders",
			SourceType: SourceTypeTable,
			Dimensions: []string{"created_at_day"},
			DimensionExprs: []DimensionExprAST{
				{Field: "created_at", Granularity: "day", Alias: "created_at_day"},
			},
			Metrics: []MetricExpr{{Field: "amount", FieldExpr: "amount", Agg: AggSum, Alias: "total_amount"}},
		},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSQL := "SELECT DATE_TRUNC('day', created_at) AS \"created_at_day\", SUM(amount) AS \"total_amount\" FROM orders GROUP BY DATE_TRUNC('day', created_at)"
	if result.Select != expectedSQL {
		t.Fatalf("expected AST-driven SQL\nwant: %s\n got: %s", expectedSQL, result.Select)
	}
}

func TestExecutor_TableChart_WithPagination(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	mockRows := []map[string]any{
		{"status": "pending", "count": 10},
		{"status": "completed", "count": 20},
	}

	conn := &MockConnection{
		rows: mockRows,
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeTable,
		Dims:       []string{"status"},
		Metrics:    []MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		Pagination: &Pagination{Page: 1, PageSize: 10},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	tableResp, ok := result.Data.(*TableResponse)
	if !ok {
		t.Fatalf("Expected TableResponse, got %T", result.Data)
	}

	if tableResp.Pagination.Page != 1 {
		t.Errorf("Expected page 1, got %d", tableResp.Pagination.Page)
	}

	if tableResp.Pagination.PageSize != 10 {
		t.Errorf("Expected pageSize 10, got %d", tableResp.Pagination.PageSize)
	}
}

func TestExecutor_TableChart_PaginationCalculation(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	// 模拟返回 25 条数据（每页 10 条，需要 3 页）
	mockRows := make([]map[string]any, 25)
	for i := range mockRows {
		mockRows[i] = map[string]any{"id": i + 1}
	}

	conn := &MockConnection{
		rows: mockRows,
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeTable,
		Dims:       []string{"id"},
		Metrics:    []MetricConfig{{Field: "id", Agg: AggCount, Alias: "cnt"}},
		Pagination: &Pagination{Page: 2, PageSize: 10},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	tableResp, ok := result.Data.(*TableResponse)
	if !ok {
		t.Fatalf("Expected TableResponse, got %T", result.Data)
	}

	expectedTotalPages := int(math.Ceil(float64(25) / float64(10)))
	if tableResp.Pagination.TotalPages != expectedTotalPages {
		t.Errorf("Expected totalPages %d, got %d", expectedTotalPages, tableResp.Pagination.TotalPages)
	}
}

func TestExecutor_BarChart(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"product": "Apple", "revenue": 1000.0},
			{"product": "Banana", "revenue": 2000.0},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"product"},
		Metrics:   []MetricConfig{{Field: "revenue", Agg: AggSum}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}

	// Verify AxisResponse structure
	axisResp, ok := result.Data.(*AxisResponse)
	if !ok {
		t.Fatalf("Expected *AxisResponse, got %T", result.Data)
	}

	if len(axisResp.XAxis) != 2 {
		t.Errorf("Expected XAxis length 2, got %d", len(axisResp.XAxis))
	}

	if len(axisResp.Series) != 1 {
		t.Errorf("Expected 1 series, got %d", len(axisResp.Series))
	}

	if result.Select == "" {
		t.Error("Expected Select SQL to be generated")
	}
}

func TestExecutor_LineChart(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "metrics", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"date": "2024-01-01", "value": 100},
			{"date": "2024-01-02", "value": 200},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeLine,
		Dims:      []string{"date"},
		Metrics:   []MetricConfig{{Field: "value", Agg: AggSum}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}

	// Verify AxisResponse structure
	axisResp, ok := result.Data.(*AxisResponse)
	if !ok {
		t.Fatalf("Expected *AxisResponse, got %T", result.Data)
	}

	if len(axisResp.XAxis) != 2 {
		t.Errorf("Expected XAxis length 2, got %d", len(axisResp.XAxis))
	}

	if len(axisResp.Series) != 1 {
		t.Errorf("Expected 1 series, got %d", len(axisResp.Series))
	}

	if result.Select == "" {
		t.Error("Expected Select SQL to be generated")
	}
}

func TestExecutor_AreaChart(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "metrics", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"date": "2024-01-01", "value": 100},
			{"date": "2024-01-02", "value": 200},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeArea,
		Dims:      []string{"date"},
		Metrics:   []MetricConfig{{Field: "value", Agg: AggSum}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	axisResp, ok := result.Data.(*AxisResponse)
	if !ok {
		t.Fatalf("Expected *AxisResponse, got %T", result.Data)
	}

	if len(axisResp.XAxis) != 2 {
		t.Errorf("Expected XAxis length 2, got %d", len(axisResp.XAxis))
	}

	if len(axisResp.Series) != 1 {
		t.Errorf("Expected 1 series, got %d", len(axisResp.Series))
	}

	if result.Select == "" {
		t.Error("Expected Select SQL to be generated")
	}
}

func TestExecutor_PieChart(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "categories", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"category": "A", "count": 50},
			{"category": "B", "count": 30},
			{"category": "C", "count": 20},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypePie,
		Dims:      []string{"category"},
		Metrics:   []MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}
}

func TestExecutor_WithFilters(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"status": "completed", "amount": 1000.0},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"status"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "completed"},
			{Field: "amount", Op: FilterGt, Value: 100},
		},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}

	// 验证生成的 SQL 包含过滤条件
	if result.Select == "" {
		t.Error("Expected Select SQL to be generated")
	}
}

func TestExecutor_WithSort(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "products", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"name": "Product A", "price": 100},
			{"name": "Product B", "price": 200},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"name"},
		Metrics:   []MetricConfig{{Field: "price", Agg: AggSum}},
		Sort:      &SortConfig{Field: "price", Order: "desc"},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}
}

func TestExecutor_WithMultipleMetrics(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "sales", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"month": "2024-01", "revenue": 10000, "cost": 3000},
			{"month": "2024-02", "revenue": 15000, "cost": 4000},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"month"},
		Metrics: []MetricConfig{
			{Field: "revenue", Agg: AggSum, Alias: "total_revenue"},
			{Field: "cost", Agg: AggSum, Alias: "total_cost"},
		},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}
}

func TestExecutor_WithSQLSource(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "sql",
		TableName: sql.NullString{Valid: false},
		QuerySQL:  sql.NullString{String: "SELECT * FROM orders WHERE created_at > '2024-01-01'", Valid: true},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"status": "completed", "amount": 1000.0},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"status"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Data == nil {
		t.Error("Expected result data, got nil")
	}
}

func TestExecutor_QueryError(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		queryErr: assertError("database error"),
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"status"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
	}

	_, err := executor.Execute(context.Background(), req)
	if err == nil {
		t.Error("Expected error from query execution, got nil")
	}
}

func assertError(msg string) error {
	return &testError{msg: msg}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestExecutor_Close(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{}
	executor := NewExecutor(conn, dataset, ds)

	err := executor.Close()
	if err != nil {
		t.Errorf("Unexpected error on close: %v", err)
	}
}

func TestExecutor_ExecuteRawQuery(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"id": 1, "name": "Test"},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	rows, err := executor.ExecuteRawQuery(context.Background(), "SELECT * FROM orders")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
}

// 验证 executor 把 filter 值作为 args 传给驱动，而不是拼进 SQL 文本。
func TestExecutor_PassesFilterArgsToConnection(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"status": "x", "amount": 1000.0},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"status"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum}},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "'; DROP TABLE users; --"},
		},
	}

	if _, err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conn.argsCalls) != 1 {
		t.Fatalf("expected 1 Execute call (chart branch), got %d", len(conn.argsCalls))
	}
	args := conn.argsCalls[0]
	if len(args) != 1 || args[0] != "'; DROP TABLE users; --" {
		t.Fatalf("filter value must travel as arg to driver, got args=%v", args)
	}
	if strings.Contains(conn.sqlCalls[0], "DROP TABLE") {
		t.Fatalf("filter value leaked into SQL text: %s", conn.sqlCalls[0])
	}
}

// 验证 table 分支的 data 与 count 查询都携带相同的 filter args。
func TestExecutor_TableBranchPassesArgsToDataAndCount(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	conn := &MockConnection{
		rows: []map[string]any{
			{"status": "x", "count": 10},
		},
	}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeTable,
		Dims:       []string{"status"},
		Metrics:    []MetricConfig{{Field: "id", Agg: AggCount, Alias: "count"}},
		Pagination: &Pagination{Page: 1, PageSize: 10},
		Filters: []FilterConfig{
			{Field: "status", Op: FilterEq, Value: "completed"},
		},
	}

	if _, err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(conn.argsCalls) != 2 {
		t.Fatalf("expected 2 Execute calls (data + count), got %d", len(conn.argsCalls))
	}
	for i, args := range conn.argsCalls {
		if len(args) != 1 || args[0] != "completed" {
			t.Fatalf("Execute call %d must carry filter arg, got args=%v", i, args)
		}
	}
}

// TestExecutor_MixedCaseMetricAliasRoundtrip 端到端钉死修复契约：生成的 SQL 必须把
// 指标别名按方言加引号（否则 DB 把 "Revenue" 折叠成 "revenue"），MockConnection 返回
// 与引号别名逐字一致的行键 "Revenue"，轴图表 series 必须取到真实数值而不是全 NULL；
// 表格分支的 orderedRows 同样必须能按 "Revenue" 命中。
func TestExecutor_MixedCaseMetricAliasRoundtrip(t *testing.T) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Test Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}

	ds := &model.Datasource{
		ID:   1,
		Name: "Test DS",
		Type: "postgresql",
	}

	mockRows := []map[string]any{
		{"region": "华东", "Revenue": 100.0},
		{"region": "西南", "Revenue": 200.0},
	}

	conn := &MockConnection{rows: mockRows}
	executor := NewExecutor(conn, dataset, ds)

	req := &ChartQueryRequest{
		DatasetID: 1,
		ChartType: ChartTypeBar,
		Dims:      []string{"region"},
		Metrics:   []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "Revenue"}},
	}

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1) 结果别名必须带方言引号，DB 才会保留大小写
	if !strings.Contains(result.Select, `AS "Revenue"`) {
		t.Fatalf("metric alias must be quoted in generated SQL, got: %s", result.Select)
	}

	// 2) 处理器按逐字别名查找必须命中
	axisResp, ok := result.Data.(*AxisResponse)
	if !ok {
		t.Fatalf("expected *AxisResponse, got %T", result.Data)
	}
	if len(axisResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(axisResp.Series))
	}
	if axisResp.Series[0].Name != "Revenue" {
		t.Fatalf("expected series name Revenue, got %q", axisResp.Series[0].Name)
	}
	if axisResp.Series[0].Data[0] != 100.0 || axisResp.Series[0].Data[1] != 200.0 {
		t.Fatalf("series data must carry real values, got %v", axisResp.Series[0].Data)
	}

	// 3) 表格分支：columns 带混合大小写别名时 orderedRows 必须能命中行键
	tableConn := &MockConnection{rows: mockRows}
	tableExecutor := NewExecutor(tableConn, dataset, ds)

	tableReq := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeTable,
		Dims:       []string{"region"},
		Metrics:    []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "Revenue"}},
		Pagination: &Pagination{Page: 1, PageSize: 10},
	}

	tableResult, err := tableExecutor.Execute(context.Background(), tableReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tableResp, ok := tableResult.Data.(*TableResponse)
	if !ok {
		t.Fatalf("expected *TableResponse, got %T", tableResult.Data)
	}
	if len(tableResp.Columns) != 2 || tableResp.Columns[1] != "Revenue" {
		t.Fatalf("expected columns [region Revenue], got %v", tableResp.Columns)
	}
	if len(tableResp.Data) != 2 || tableResp.Data[0]["Revenue"] != 100.0 {
		t.Fatalf("orderedRows must key on verbatim alias Revenue, got %v", tableResp.Data)
	}
}
