package chart

import (
	"context"
	"testing"

	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"
	"dataray/internal/query"
)

// stubConnection 测试连接替身，仅用于占位和验证 close 被调用。
type stubConnection struct {
	closed bool
}

// Close 关闭测试连接。
func (c *stubConnection) Close() error {
	c.closed = true
	return nil
}

// Ping 测试替身不做真实探活。
func (c *stubConnection) Ping(ctx context.Context) error {
	return nil
}

// GetTables service 层测试不依赖该方法。
func (c *stubConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}

// GetColumns service 层测试不依赖该方法。
func (c *stubConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}

// GetPrimaryKeys service 层测试不依赖该方法。
func (c *stubConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}

// Execute service 层测试通过 executor stub 断言，不应直接调用连接执行 SQL。
func (c *stubConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	return nil, nil
}

// stubExecutor 记录 service 层传入的 query request，并返回预设结果。
type stubExecutor struct {
	result  query.ExecutorResult
	err     error
	lastReq *query.ChartQueryRequest
}

// Execute 记录调用参数并返回预设结果。
func (e *stubExecutor) Execute(ctx context.Context, req *query.ChartQueryRequest) (query.ExecutorResult, error) {
	e.lastReq = req
	return e.result, e.err
}

// TestChartServiceQuery_TableRequestCompatibility 验证表格查询请求被完整转换到 query 层，并透传 SQL/响应。
func TestChartServiceQuery_TableRequestCompatibility(t *testing.T) {
	conn := &stubConnection{}
	executor := &stubExecutor{
		result: query.ExecutorResult{
			Data: &query.TableResponse{
				Columns: []string{"status", "total_amount"},
				Data: []map[string]any{
					{"status": "paid", "total_amount": 100.5},
				},
				Pagination: query.TablePagination{Page: 2, PageSize: 20, Total: 31, TotalPages: 2},
			},
			GeneratedSQL: query.GeneratedSQL{
				Select: `SELECT "status", SUM("amount") AS "total_amount" FROM "orders" WHERE "status" = 'paid' GROUP BY "status" ORDER BY "total_amount" DESC LIMIT 20 OFFSET 20`,
				Count:  `SELECT COUNT(*) FROM (SELECT "status", SUM("amount") AS "total_amount" FROM "orders" WHERE "status" = 'paid' GROUP BY "status") AS count_subquery`,
			},
		},
	}

	service := NewService(nil).(*chartService)
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table"}, nil
	}
	service.getDatasourceModelFn = func(ctx context.Context, id int) (*model.Datasource, error) {
		return &model.Datasource{ID: id, Type: "postgresql"}, nil
	}
	service.connectFn = func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
		return conn, nil
	}
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return executor
	}

	result, err := service.Query(context.Background(), &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "table",
		Dims:      []string{"status"},
		Metrics: []entity.MetricConfig{
			{Field: "amount", Agg: "sum", Alias: "total_amount"},
		},
		Filters: []entity.Filter{
			{Field: "status", Operator: "eq", Value: "paid"},
		},
		Pagination: &entity.Pagination{Page: 2, PageSize: 20},
		Sort:       &entity.SortConfig{Field: "total_amount", Order: "desc"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executor.lastReq == nil {
		t.Fatal("expected executor to receive request")
	}
	if executor.lastReq.ChartType != query.ChartTypeTable {
		t.Fatalf("expected chart type table, got %q", executor.lastReq.ChartType)
	}
	if executor.lastReq.PlannedAST == nil {
		t.Fatal("expected planned AST to be attached to executor request")
	}
	if len(executor.lastReq.PlannedAST.Dimensions) != 1 || executor.lastReq.PlannedAST.Dimensions[0] != "status" {
		t.Fatalf("expected planned AST dims [status], got %v", executor.lastReq.PlannedAST.Dimensions)
	}
	if len(executor.lastReq.Dims) != 1 || executor.lastReq.Dims[0] != "status" {
		t.Fatalf("expected dims [status], got %v", executor.lastReq.Dims)
	}
	if len(executor.lastReq.Metrics) != 1 || executor.lastReq.Metrics[0].Alias != "total_amount" {
		t.Fatalf("expected metric alias total_amount, got %v", executor.lastReq.Metrics)
	}
	if len(executor.lastReq.Filters) != 1 || executor.lastReq.Filters[0].Op != query.FilterEq {
		t.Fatalf("expected eq filter, got %v", executor.lastReq.Filters)
	}
	if executor.lastReq.Sort == nil || executor.lastReq.Sort.Field != "total_amount" || executor.lastReq.Sort.Order != "desc" {
		t.Fatalf("expected sort by total_amount desc, got %v", executor.lastReq.Sort)
	}
	if executor.lastReq.Pagination == nil || executor.lastReq.Pagination.Page != 2 || executor.lastReq.Pagination.PageSize != 20 {
		t.Fatalf("expected pagination {2,20}, got %v", executor.lastReq.Pagination)
	}
	if !conn.closed {
		t.Fatal("expected connection to be closed after query")
	}

	tableResp, ok := result.Data.(*query.TableResponse)
	if !ok {
		t.Fatalf("expected TableResponse, got %T", result.Data)
	}
	if tableResp.Pagination.Total != 31 {
		t.Fatalf("expected total 31, got %d", tableResp.Pagination.Total)
	}
	if result.SelectSQL == "" || result.CountSQL == "" {
		t.Fatalf("expected select/count sql to be forwarded, got select=%q count=%q", result.SelectSQL, result.CountSQL)
	}
}

// TestChartServiceQuery_LineRequestCompatibility 验证折线图请求继续保持旧请求到 query 层的兼容转换。
func TestChartServiceQuery_LineRequestCompatibility(t *testing.T) {
	executor := &stubExecutor{
		result: query.ExecutorResult{
			Data: &query.AxisResponse{
				XAxis: []string{"2025-01", "2025-02"},
				Series: []query.AxisSeries{
					{Name: "revenue", Data: []any{1200.0, 1800.0}},
				},
			},
			GeneratedSQL: query.GeneratedSQL{Select: `SELECT ...`},
		},
	}

	service := NewService(nil).(*chartService)
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table"}, nil
	}
	service.getDatasourceModelFn = func(ctx context.Context, id int) (*model.Datasource, error) {
		return &model.Datasource{ID: id, Type: "postgresql"}, nil
	}
	service.connectFn = func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
		return &stubConnection{}, nil
	}
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return executor
	}

	result, err := service.Query(context.Background(), &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "line",
		Dims:      []string{"month"},
		Metrics: []entity.MetricConfig{
			{Field: "revenue", Agg: "sum", Alias: "revenue"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executor.lastReq == nil || executor.lastReq.ChartType != query.ChartTypeLine {
		t.Fatalf("expected line request, got %v", executor.lastReq)
	}
	if executor.lastReq.PlannedAST == nil {
		t.Fatal("expected planned AST for line request")
	}
	if len(executor.lastReq.Dims) != 1 || executor.lastReq.Dims[0] != "month" {
		t.Fatalf("expected dims [month], got %v", executor.lastReq.Dims)
	}

	axisResp, ok := result.Data.(*query.AxisResponse)
	if !ok {
		t.Fatalf("expected AxisResponse, got %T", result.Data)
	}
	if len(axisResp.Series) != 1 || axisResp.Series[0].Name != "revenue" {
		t.Fatalf("expected revenue series, got %v", axisResp.Series)
	}
	if result.CountSQL != "" {
		t.Fatalf("expected non-table chart count sql to stay empty, got %q", result.CountSQL)
	}
}

// TestChartServiceQuery_ScatterRequestCompatibility 验证散点图双 metric 语义在 service 层不会被改写。
func TestChartServiceQuery_ScatterRequestCompatibility(t *testing.T) {
	executor := &stubExecutor{
		result: query.ExecutorResult{
			Data:         &query.ScatterResponse{Data: [][]float64{{1.2, 3.4}, {5.6, 7.8}}},
			GeneratedSQL: query.GeneratedSQL{Select: `SELECT ...`},
		},
	}

	service := NewService(nil).(*chartService)
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table"}, nil
	}
	service.getDatasourceModelFn = func(ctx context.Context, id int) (*model.Datasource, error) {
		return &model.Datasource{ID: id, Type: "postgresql"}, nil
	}
	service.connectFn = func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
		return &stubConnection{}, nil
	}
	service.executorFactory = func(conn datasource.Connection, dataset *model.Dataset, ds *model.Datasource) queryExecutor {
		return executor
	}

	result, err := service.Query(context.Background(), &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "scatter",
		Dims:      []string{"city"},
		Metrics: []entity.MetricConfig{
			{Field: "x_value", Agg: "avg", Alias: "avg_x"},
			{Field: "y_value", Agg: "sum", Alias: "sum_y"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executor.lastReq == nil || executor.lastReq.ChartType != query.ChartTypeScatter {
		t.Fatalf("expected scatter request, got %v", executor.lastReq)
	}
	if len(executor.lastReq.Metrics) != 2 {
		t.Fatalf("expected 2 metrics for scatter, got %v", executor.lastReq.Metrics)
	}
	if executor.lastReq.Metrics[0].Field != "x_value" || executor.lastReq.Metrics[1].Field != "y_value" {
		t.Fatalf("expected metrics [x_value y_value], got %v", executor.lastReq.Metrics)
	}

	scatterResp, ok := result.Data.(*query.ScatterResponse)
	if !ok {
		t.Fatalf("expected ScatterResponse, got %T", result.Data)
	}
	if len(scatterResp.Data) != 2 {
		t.Fatalf("expected 2 scatter points, got %d", len(scatterResp.Data))
	}
}

// TestBuildQuerySpecFromEntityRequest 验证 service 层已统一先进入 QuerySpec，再下沉回兼容请求。
func TestBuildQuerySpecFromEntityRequest(t *testing.T) {
	spec := buildQuerySpecFromEntityRequest(&entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "table",
		Dims:      []string{"status", "city"},
		Metrics: []entity.MetricConfig{
			{Field: "amount", Agg: "sum", Alias: "total_amount"},
		},
		Filters: []entity.Filter{
			{Field: "status", Operator: "eq", Value: "paid"},
		},
		Pagination: &entity.Pagination{Page: 3, PageSize: 15},
		Sort:       &entity.SortConfig{Field: "total_amount", Order: "desc"},
	})

	if spec == nil {
		t.Fatal("expected query spec, got nil")
	}
	if len(spec.Dimensions) != 2 || spec.Dimensions[0].Field != "status" || spec.Dimensions[1].Field != "city" {
		t.Fatalf("expected dimensions [status city], got %v", spec.Dimensions)
	}
	if len(spec.Metrics) != 1 || spec.Metrics[0].Field != "amount" || spec.Metrics[0].Alias != "total_amount" {
		t.Fatalf("expected metric amount/total_amount, got %v", spec.Metrics)
	}
	if len(spec.Filters) != 1 || spec.Filters[0].Op != query.FilterEq {
		t.Fatalf("expected eq filter, got %v", spec.Filters)
	}
	if spec.Sort == nil || spec.Sort.Field != "total_amount" || spec.Sort.Order != "desc" {
		t.Fatalf("expected sort total_amount desc, got %v", spec.Sort)
	}
	if spec.Pagination == nil || spec.Pagination.Page != 3 || spec.Pagination.PageSize != 15 {
		t.Fatalf("expected pagination {3,15}, got %v", spec.Pagination)
	}

	dims, metrics, filters := query.QuerySpecToBuildArgs(spec)
	if len(dims) != 2 || dims[0] != "status" || dims[1] != "city" {
		t.Fatalf("expected flattened dims [status city], got %v", dims)
	}
	if len(metrics) != 1 || metrics[0].Field != "amount" || metrics[0].Alias != "total_amount" {
		t.Fatalf("expected flattened metric amount/total_amount, got %v", metrics)
	}
	if len(filters) != 1 || filters[0].Field != "status" {
		t.Fatalf("expected flattened filter on status, got %v", filters)
	}
}
