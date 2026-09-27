package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"data-insights/internal/datasource"
)

// tableTotalConnection 按 SQL 内容把三类查询分流到不同结果集：
// 明细（含 GROUP BY）、计数（含 _count_query）、合计（两者都没有）。
// 三类 SQL 的形状差异正是被测契约的一部分——合计若误带 GROUP BY/LIMIT，
// 分流会落到错误的桶里，测试立刻失败。
type tableTotalConnection struct {
	dataRows   []map[string]any
	countRows  []map[string]any
	totalRows  []map[string]any
	totalError error

	SQLs []string
}

func (m *tableTotalConnection) Execute(_ context.Context, sql string, _ ...any) (*datasource.QueryResult, error) {
	m.SQLs = append(m.SQLs, sql)
	switch {
	case strings.Contains(sql, "_count_query"):
		return &datasource.QueryResult{Rows: m.countRows}, nil
	case strings.Contains(sql, "GROUP BY"):
		return &datasource.QueryResult{Rows: m.dataRows}, nil
	default:
		if m.totalError != nil {
			return nil, m.totalError
		}
		return &datasource.QueryResult{Rows: m.totalRows}, nil
	}
}

func (m *tableTotalConnection) Close() error { return nil }
func (m *tableTotalConnection) Ping(context.Context) error {
	return nil
}
func (m *tableTotalConnection) GetTables(context.Context) ([]datasource.TableInfo, error) {
	return nil, nil
}
func (m *tableTotalConnection) GetColumns(context.Context, string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (m *tableTotalConnection) GetPrimaryKeys(context.Context, string) ([]string, error) {
	return nil, nil
}
func (m *tableTotalConnection) Capabilities(context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}

func tableTotalFixture(opts map[string]any, dims []string) (*ChartQueryRequest, *tableTotalConnection) {
	_, _, req := tablePaginationFixture()
	req.Dims = dims
	req.QueryOptions = opts
	conn := &tableTotalConnection{
		dataRows: []map[string]any{
			{"region": "East", "total": 30.0},
			{"region": "West", "total": 12.0},
		},
		countRows: []map[string]any{{"_total": int64(2)}},
		totalRows: []map[string]any{{"total": 42.0}},
	}
	return req, conn
}

// TestExecutor_TableTotalRequested 开了 show_total 且表格有维度：额外发一条无维度/
// 无 LIMIT 的合计查询，并把结果放进 total。
func TestExecutor_TableTotalRequested(t *testing.T) {
	req, conn := tableTotalFixture(map[string]any{"show_total": true}, []string{"region"})
	dataset, ds, _ := tablePaginationFixture()

	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	table, ok := result.Data.(*TableResponse)
	if !ok {
		t.Fatalf("expected *TableResponse, got %T", result.Data)
	}
	if table.Total == nil {
		t.Fatalf("expected total row, got nil")
	}
	if got := table.Total["total"]; fmt.Sprint(got) != "42" {
		t.Errorf("total[total] = %v, want 42", got)
	}
	// 合计键必须与明细行的列键一致（指标别名），前端按列名取值。
	if _, ok := table.Total["region"]; ok {
		t.Errorf("total row must not carry dimension columns: %v", table.Total)
	}
	if len(table.Data) != 2 {
		t.Errorf("明细行不得被合计污染: len(Data) = %d, want 2", len(table.Data))
	}

	var totalSQL string
	for _, sql := range conn.SQLs {
		if !strings.Contains(sql, "GROUP BY") && !strings.Contains(sql, "_count_query") {
			totalSQL = sql
		}
	}
	if totalSQL == "" {
		t.Fatalf("expected a dedicated total query, got SQLs %v", conn.SQLs)
	}
	for _, forbidden := range []string{"LIMIT", "OFFSET", "region"} {
		if strings.Contains(totalSQL, forbidden) {
			t.Errorf("total SQL must not contain %q: %s", forbidden, totalSQL)
		}
	}
}

// TestExecutor_TableTotalAbsentWhenNotRequested 未开开关时一条合计查询都不发
// （零额外往返），total 缺省。
func TestExecutor_TableTotalAbsentWhenNotRequested(t *testing.T) {
	req, conn := tableTotalFixture(nil, []string{"region"})
	dataset, ds, _ := tablePaginationFixture()

	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	table := result.Data.(*TableResponse)
	if table.Total != nil {
		t.Errorf("total = %v, want nil", table.Total)
	}
	for _, sql := range conn.SQLs {
		if !strings.Contains(sql, "GROUP BY") && !strings.Contains(sql, "_count_query") {
			t.Fatalf("unexpected extra query: %s", sql)
		}
	}
}

// TestExecutor_TableTotalSkippedWithoutDimensions 无维度的表格主查询本身就是一行全表
// 聚合，再查一次只会得到一个与明细行完全相同的"合计"——必须跳过。
func TestExecutor_TableTotalSkippedWithoutDimensions(t *testing.T) {
	req, conn := tableTotalFixture(map[string]any{"show_total": true}, nil)
	dataset, ds, _ := tablePaginationFixture()

	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	table := result.Data.(*TableResponse)
	if table.Total != nil {
		t.Errorf("total = %v, want nil（无维度时不产合计行）", table.Total)
	}
	if len(conn.SQLs) != 2 {
		t.Errorf("期望只发明细+计数两条查询，实际 %d 条: %v", len(conn.SQLs), conn.SQLs)
	}
}

// TestExecutor_TableTotalAcceptsCamelCaseKey 持久化文档小节是 camelCase（showTotal），
// 未归一的调用面也应能打开该能力。
func TestExecutor_TableTotalAcceptsCamelCaseKey(t *testing.T) {
	req, conn := tableTotalFixture(map[string]any{"showTotal": true}, []string{"region"})
	dataset, ds, _ := tablePaginationFixture()

	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Data.(*TableResponse).Total == nil {
		t.Fatalf("expected total row for camelCase key")
	}
}

// TestExecutor_TableTotalQueryErrorPropagates 合计查询失败必须显式报错，不能静默
// 退化成「这张图没有合计」——两者对用户是完全不同的事实。
func TestExecutor_TableTotalQueryErrorPropagates(t *testing.T) {
	req, conn := tableTotalFixture(map[string]any{"show_total": true}, []string{"region"})
	conn.totalError = fmt.Errorf("boom")
	dataset, ds, _ := tablePaginationFixture()

	if _, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req); err == nil {
		t.Fatalf("expected error from failing total query")
	} else if !strings.Contains(err.Error(), "table total query failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExecutor_TableTotalWithoutPagination 不带 pagination 的 table 分支（分享页/仪表盘
// 读持久化 config 走的就是这一条）也必须给合计行——否则同一个开关在 builder 里有合计、
// 在分享页与仪表盘里没有。
func TestExecutor_TableTotalWithoutPagination(t *testing.T) {
	req, conn := tableTotalFixture(map[string]any{"show_total": true}, []string{"region"})
	req.Pagination = nil
	dataset, ds, _ := tablePaginationFixture()

	result, err := NewExecutor(conn, dataset, ds).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	table := result.Data.(*TableResponse)
	if table.Total == nil {
		t.Fatalf("expected total row on the non-paginated table path, SQLs=%v", conn.SQLs)
	}
	if got := fmt.Sprint(table.Total["total"]); got != "42" {
		t.Errorf("total[total] = %s, want 42", got)
	}
}
