package dataset

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/listpage"
)

// TestListSecondPageSortsAndCountsTheFullSet 钉住 issue #205 的核心口径：
// 第 101 条起可取到（LIMIT 10 OFFSET 100），total 来自 COUNT 而非返回条数，
// 且 ORDER BY 作用在 LIMIT 之前的全量结果上。
func TestListSecondPageSortsAndCountsTheFullSet(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(120))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
			AddRow(101, "row-101", 1, "table", nil, nil))

	got, err := s.List(context.Background(), ListOptions{
		Params: listpage.Params{Limit: 10, Offset: 100, Sort: "name", Order: "asc"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 120 {
		t.Errorf("total = %d, want 120 (COUNT of the filtered set)", got.Total)
	}
	if len(got.Items) != 1 || got.Items[0].ID != 101 {
		t.Fatalf("second page must contain row 101, got %+v", got.Items)
	}

	sel := findStmt(executed, "SELECT")
	for _, want := range []string{"ORDER BY name ASC", "LIMIT 10", "OFFSET 100"} {
		if !strings.Contains(sel, want) {
			t.Errorf("data query missing %q: %s", want, sel)
		}
	}
	// ORDER BY must precede LIMIT: sorting the whole filtered set, not the page.
	if strings.Index(sel, "ORDER BY") > strings.Index(sel, "LIMIT") {
		t.Errorf("ORDER BY must be applied before LIMIT: %s", sel)
	}
}

// TestListOutOfRangeOffsetReturnsEmptyPage：越界 offset 返回空页 + 真实 total，
// 不是错误（旧客户端翻过头不该 500）。
func TestListOutOfRangeOffsetReturnsEmptyPage(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}))

	got, err := s.List(context.Background(), ListOptions{Params: listpage.Params{Limit: 10, Offset: 900}})
	if err != nil {
		t.Fatalf("out-of-range offset must not error: %v", err)
	}
	if got.Total != 5 || len(got.Items) != 0 {
		t.Errorf("got total=%d items=%d, want total=5 items=0", got.Total, len(got.Items))
	}
}

// TestListUnknownSortFallsBackToDefault：sort 是白名单，未知值回落 id DESC，
// 绝不把请求值拼进 SQL。
func TestListUnknownSortFallsBackToDefault(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}))

	_, err = s.List(context.Background(), ListOptions{
		Params: listpage.Params{Limit: 10, Sort: "name; DROP TABLE bi_dataset", Order: "asc"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sel := findStmt(executed, "SELECT")
	if !strings.Contains(sel, "ORDER BY id DESC") {
		t.Errorf("unknown sort must fall back to id DESC: %s", sel)
	}
	if strings.Contains(sel, "DROP") {
		t.Errorf("client sort leaked into SQL: %s", sel)
	}
}

// TestListAppliesToolbarFilters：关键字走 ILIKE、数据源/查询类型走等值过滤，
// 并且这些条件同时进 COUNT 与数据查询（否则总数会与实际页对不上）。
func TestListAppliesToolbarFilters(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}))

	_, err = s.List(context.Background(), ListOptions{
		Params:       listpage.Params{Limit: 10, Keyword: "orders"},
		DatasourceID: 7,
		QueryType:    "table",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(executed) < 2 {
		t.Fatalf("expected COUNT + data query, got %v", executed)
	}
	for _, sel := range executed {
		if !strings.Contains(sel, "datasource_id = ") || !strings.Contains(sel, "query_type = ") {
			t.Errorf("filters must apply to both COUNT and page query: %s", sel)
		}
		if !strings.Contains(sel, "name ILIKE") || !strings.Contains(sel, "table_name ILIKE") {
			t.Errorf("keyword must use ILIKE on name/table_name: %s", sel)
		}
	}
}
