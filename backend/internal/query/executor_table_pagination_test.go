package query

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

// tablePaginationConnection 按 SQL 内容分流返回结果：数据查询返回明细行，计数查询返回
// 聚合单行。真实驱动就是这么区分的——沿用 MockConnection 的"同一份 rows 回给所有 SQL"
// 无法覆盖"总数到底取哪来的"这条路径。
type tablePaginationConnection struct {
	dataRows  []map[string]any
	countRows []map[string]any
}

func (m *tablePaginationConnection) Execute(ctx context.Context, sql string, args ...any) (*datasource.QueryResult, error) {
	if strings.Contains(sql, "_count_query") {
		return &datasource.QueryResult{Rows: m.countRows}, nil
	}
	return &datasource.QueryResult{Rows: m.dataRows}, nil
}

func (m *tablePaginationConnection) Close() error { return nil }
func (m *tablePaginationConnection) Ping(ctx context.Context) error {
	return nil
}
func (m *tablePaginationConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}
func (m *tablePaginationConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (m *tablePaginationConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}
func (m *tablePaginationConnection) Capabilities(ctx context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}

// tablePaginationFixture 构造一个走 table 分支（带分页）的最小请求。
func tablePaginationFixture() (*model.Dataset, *model.Datasource, *ChartQueryRequest) {
	dataset := &model.Dataset{
		ID:        1,
		Name:      "Table Dataset",
		QueryType: "table",
		TableName: sql.NullString{String: "orders", Valid: true},
		QuerySQL:  sql.NullString{Valid: false},
	}
	ds := &model.Datasource{ID: 1, Name: "Test DS", Type: "postgresql"}
	req := &ChartQueryRequest{
		DatasetID:  1,
		ChartType:  ChartTypeTable,
		Dims:       []string{"region"},
		Metrics:    []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}},
		Pagination: &Pagination{Page: 1, PageSize: 10},
	}
	return dataset, ds, req
}

// 分页总数必须取计数查询返回的 _total 聚合值。
// 回归：曾按 len(countResult.Rows) 取总数，而计数 SQL 恒返回一行，导致有 GROUP BY 的
// 表格查询总数永远是 1、分页器永远只有一页。
func TestExecutor_TablePaginationTotalUsesCountAggregate(t *testing.T) {
	dataset, ds, req := tablePaginationFixture()
	conn := &tablePaginationConnection{
		dataRows: []map[string]any{
			{"region": "East", "total": 30},
			{"region": "West", "total": 12},
		},
		countRows: []map[string]any{{"_total": int64(42)}},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	table, ok := result.Data.(*TableResponse)
	if !ok {
		t.Fatalf("expected *TableResponse, got %T", result.Data)
	}
	if table.Pagination.Total != 42 {
		t.Errorf("Total = %d, want 42 (COUNT(*) 的 _total 值)", table.Pagination.Total)
	}
	if table.Pagination.TotalPages != 5 {
		t.Errorf("TotalPages = %d, want 5", table.Pagination.TotalPages)
	}
	if len(table.Data) != 2 {
		t.Errorf("len(Data) = %d, want 2", len(table.Data))
	}
}

// 计数结果里没有可解析的 _total（异常驱动/别名漂移）时回退为本次返回行数，不能变成 0。
func TestExecutor_TablePaginationTotalFallsBackWhenCountUnavailable(t *testing.T) {
	dataset, ds, req := tablePaginationFixture()
	conn := &tablePaginationConnection{
		dataRows: []map[string]any{
			{"region": "East", "total": 30},
			{"region": "West", "total": 12},
		},
		countRows: []map[string]any{{"unexpected_alias": int64(42)}},
	}
	executor := NewExecutor(conn, dataset, ds)

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	table, ok := result.Data.(*TableResponse)
	if !ok {
		t.Fatalf("expected *TableResponse, got %T", result.Data)
	}
	if table.Pagination.Total != 2 {
		t.Errorf("Total = %d, want 2 (回退为返回行数)", table.Pagination.Total)
	}
}
