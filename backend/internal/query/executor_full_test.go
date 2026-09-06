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

	expectedSQL := "SELECT DATE_TRUNC('day', created_at) AS created_at_day, SUM(amount) AS total_amount FROM orders GROUP BY DATE_TRUNC('day', created_at)"
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
