package chart

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"dataray/internal/crypto"

	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"
	"dataray/internal/query"
)

// stubConnection 测试连接替身，用于占位、验证 close，以及在 GetData 的
// 原始行回退路径中记录/应答 SQL 执行（executeFn + lastSQL）。
type stubConnection struct {
	closed    bool
	lastSQL   string
	executeFn func(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error)
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

// Execute 记录 SQL；配置了 executeFn 时（GetData 回退路径）以其应答，
// 否则保持旧行为：返回 nil 结果（Query 主链路由 executor stub 应答）。
func (c *stubConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	c.lastSQL = sql
	if c.executeFn != nil {
		return c.executeFn(ctx, sql, args...)
	}
	return nil, nil
}

// stubExecutor 记录 service 层传入的 query request，并返回预设结果。
type stubExecutor struct {
	result  query.ExecutorResult
	err     error
	called  bool
	lastReq *query.ChartQueryRequest
}

// Execute 记录调用参数并返回预设结果。
func (e *stubExecutor) Execute(ctx context.Context, req *query.ChartQueryRequest) (query.ExecutorResult, error) {
	e.called = true
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

// --- password resolution tests (C1 fix) ---

func testAESKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// TestConnectResolvesPassword 验证 chart 服务启用 key 后 connect 拿到的是
// 解密后的密码（dialFn 收到的值即写入 ConnectionConfig.Password 的值），
// 且存量明文密码被回写为 v1: 密文。
func TestConnectResolvesPassword(t *testing.T) {
	key := testAESKey()

	t.Run("encrypted password decrypted for dial", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, "fixture-credential-value")
		if err != nil {
			t.Fatal(err)
		}
		var got string
		s := NewService(nil).(*chartService)
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: ct}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-credential-value" {
			t.Fatalf("connect should dial with decrypted password, got %q", got)
		}
	})

	t.Run("legacy plaintext upgraded and used", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer sqlDB.Close()

		var got string
		s := NewService(bun.NewDB(sqlDB, pgdialect.New())).(*chartService)
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))

		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: "fixture-plaintext-credential"}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-plaintext-credential" {
			t.Fatalf("connection should use original plaintext, got %q", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("legacy password was not upgraded: %v", err)
		}
	})
}

// --- GetData: v1 config 与交互查询共用聚合管道（B4-D2）---

// newGetDataTestService 构造 GetData 路径的全部外部依赖替身：
// 图表/数据集/数据源模型读取、连接与 executor，供聚合与回退测试共用。
func newGetDataTestService(config string) (*chartService, *stubConnection, *stubExecutor) {
	conn := &stubConnection{}
	executor := &stubExecutor{
		result: query.ExecutorResult{
			Data: &query.AxisResponse{
				XAxis:  []string{"华北"},
				Series: []query.AxisSeries{{Name: "Revenue", Data: []any{1234.0}}},
			},
			GeneratedSQL: query.GeneratedSQL{Select: `SELECT region, SUM(amount) AS revenue FROM sales_orders GROUP BY region`},
		},
	}
	service := NewService(nil).(*chartService)
	service.getChartModelFn = func(ctx context.Context, id int) (*model.Chart, error) {
		return &model.Chart{ID: id, Name: "c", DatasetID: 10, ChartType: "bar", Config: config}, nil
	}
	service.getDatasetModelFn = func(ctx context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{ID: id, DatasourceID: 1, QueryType: "table", TableName: sql.NullString{String: "sales_orders", Valid: true}}, nil
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
	return service, conn, executor
}

// TestChartServiceGetData_V1ConfigRunsAggregationPipeline 验证 v1 配置的分享
// 取数走与 POST /api/charts/query 相同的聚合管道，dims/metrics/filters/sort/limit
// 的映射与前端 buildChartQueryRequest 一致。
func TestChartServiceGetData_V1ConfigRunsAggregationPipeline(t *testing.T) {
	config := `{"version":1,"chartType":"bar","query":{"dimensionGroups":[{"id":"dim-1","fields":["region"]}],"metricGroups":[{"id":"met-1","fields":["amount"]}],"filters":[{"field":"region","operator":"eq","value":"华北","logic":"and"}],"sort":{"field":"amount","order":"desc"},"limit":5},"fieldMeta":{"amount":{"aggregation":"sum","alias":"Revenue"}}}`
	service, conn, executor := newGetDataTestService(config)

	result, err := service.GetData(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executor.called {
		t.Fatal("expected v1 config to run the aggregation pipeline")
	}
	req := executor.lastReq
	if req == nil {
		t.Fatal("expected executor request to be recorded")
	}
	if req.DatasetID != 10 {
		t.Fatalf("expected dataset id 10 from chart, got %d", req.DatasetID)
	}
	if req.ChartType != query.ChartTypeBar {
		t.Fatalf("expected chart type bar, got %q", req.ChartType)
	}
	if len(req.Dims) != 1 || req.Dims[0] != "region" {
		t.Fatalf("expected dims [region], got %v", req.Dims)
	}
	// 指标映射对齐 builder：agg 取 fieldMeta.aggregation（默认 sum），
	// alias 取 fieldMeta.alias（builder 的 fallback 是列名，见 ChartBuilder 注释）。
	if len(req.Metrics) != 1 || req.Metrics[0].Field != "amount" ||
		req.Metrics[0].Agg != query.AggSum || req.Metrics[0].Alias != "Revenue" {
		t.Fatalf("expected metric amount/sum/Revenue, got %+v", req.Metrics)
	}
	if len(req.Filters) != 1 || req.Filters[0].Op != query.FilterEq || req.Filters[0].Value != "华北" {
		t.Fatalf("expected eq filter on 华北, got %+v", req.Filters)
	}
	if req.Sort == nil || req.Sort.Field != "amount" || req.Sort.Order != "desc" {
		t.Fatalf("expected sort amount desc, got %+v", req.Sort)
	}
	if req.Pagination == nil || req.Pagination.Page != 1 || req.Pagination.PageSize != 5 {
		t.Fatalf("expected pagination {1,5} from config limit, got %+v", req.Pagination)
	}
	if req.PlannedAST == nil {
		t.Fatal("expected planned AST")
	}
	if !conn.closed {
		t.Fatal("expected connection closed")
	}

	axis, ok := result.Data.(*query.AxisResponse)
	if !ok {
		t.Fatalf("expected AxisResponse, got %T", result.Data)
	}
	if len(axis.Series) != 1 || axis.Series[0].Name != "Revenue" {
		t.Fatalf("expected processed series passthrough, got %+v", axis.Series)
	}
	if result.SelectSQL == "" {
		t.Fatal("expected select sql forwarded from pipeline")
	}
}

// TestChartServiceGetData_V1ConfigMetricDefaults 验证 fieldMeta 缺省时指标
// 回落到 sum + 列名别名（与 builder 的 `|| 'sum'` / `|| f.name` 对齐），
// 无 limit 时不带 pagination，chartType 缺省时回落到 chart.ChartType。
func TestChartServiceGetData_V1ConfigMetricDefaults(t *testing.T) {
	config := `{"version":1,"query":{"dimensionGroups":[],"metricGroups":[{"id":"met-1","fields":["amount","qty"]}]}}`
	service, _, executor := newGetDataTestService(config)

	if _, err := service.GetData(context.Background(), 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := executor.lastReq
	if req.ChartType != query.ChartTypeBar {
		t.Fatalf("expected fallback chart type bar, got %q", req.ChartType)
	}
	if len(req.Metrics) != 2 ||
		req.Metrics[0].Field != "amount" || req.Metrics[0].Agg != query.AggSum || req.Metrics[0].Alias != "amount" ||
		req.Metrics[1].Field != "qty" || req.Metrics[1].Alias != "qty" {
		t.Fatalf("expected default sum + name aliases, got %+v", req.Metrics)
	}
	if req.Pagination != nil {
		t.Fatalf("expected no pagination without limit, got %+v", req.Pagination)
	}
}

// assertRawRowsFallback 钉死"非 v1 可用配置 → 保持原始行回退"：不触发
// executor，走 WrapPreviewSQL(…, 100) 并原样返回行。
func assertRawRowsFallback(t *testing.T, config string) {
	t.Helper()
	rows := []map[string]any{{"field-0": "x", "field-1": 1.0}}
	service, conn, executor := newGetDataTestService(config)
	conn.executeFn = func(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
		return &datasource.QueryResult{Rows: rows}, nil
	}

	result, err := service.GetData(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executor.called {
		t.Fatal("fallback config must not run the aggregation pipeline")
	}
	if !strings.Contains(conn.lastSQL, "LIMIT 100") {
		t.Fatalf("expected raw preview SQL, got %q", conn.lastSQL)
	}
	if !reflect.DeepEqual(result.Data, rows) {
		t.Fatalf("expected raw rows verbatim, got %#v", result.Data)
	}
}

// TestChartServiceGetData_LegacyConfigFallsBackToRawRows 钉死旧结构
// （queryConfig + 位置 field-N，列名不可恢复）保持原始行回退。
func TestChartServiceGetData_LegacyConfigFallsBackToRawRows(t *testing.T) {
	legacy := `{"chartType":"bar","queryConfig":{"dimensionGroups":[{"id":"dim","fields":["field-0"]}],"metricGroups":[{"id":"met","fields":["field-1"]}]}}`
	assertRawRowsFallback(t, legacy)
}

// TestChartServiceGetData_MalformedConfigFallsBackToRawRows 钉死损坏 JSON
// 配置的原始行回退。
func TestChartServiceGetData_MalformedConfigFallsBackToRawRows(t *testing.T) {
	assertRawRowsFallback(t, `{not valid json`)
}

// TestChartServiceGetData_EmptyConfigFallsBackToRawRows 钉死空配置 "{}"
// （toChartModel 的默认值）的原始行回退。
func TestChartServiceGetData_EmptyConfigFallsBackToRawRows(t *testing.T) {
	assertRawRowsFallback(t, `{}`)
}

// TestChartServiceGetData_V1EmptyGroupsFallsBackToRawRows 钉死 v1 文档但
// 维度/指标组皆空（无可执行查询）时的原始行回退。
func TestChartServiceGetData_V1EmptyGroupsFallsBackToRawRows(t *testing.T) {
	assertRawRowsFallback(t, `{"version":1,"chartType":"bar","query":{"dimensionGroups":[],"metricGroups":[]}}`)
}
