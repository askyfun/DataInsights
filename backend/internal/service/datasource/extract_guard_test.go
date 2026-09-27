package datasource

import (
	"context"
	"strings"
	"testing"

	"data-insights/internal/extract"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestListHidesExtractDatasource 验证抽取存储数据源不进常规列表（issue #118 预留）：
// 守卫启用时 SELECT 带 id <> ? 排除（SQL 级，limit/offset 仍对可见集合成立），
// 未启用时不带该条件（直连零回归）。
func TestListHidesExtractDatasource(t *testing.T) {
	run := func(extractID int) string {
		var executed []string
		sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer sqlDB.Close()
		s := &datasourceService{
			db:      bun.NewDB(sqlDB, pgdialect.New()),
			extract: extract.Guard{DatasourceID: extractID},
		}
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type"}).AddRow(1, "ds", "postgresql"))
		if _, err := s.List(context.Background(), 0, 0); err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, stmt := range executed {
			if strings.Contains(stmt, "FROM \"bi_datasource\"") {
				return stmt
			}
		}
		t.Fatalf("no SELECT on bi_datasource captured: %q", executed)
		return ""
	}

	if stmt := run(0); strings.Contains(stmt, "id <>") {
		t.Fatalf("disabled guard must not add exclusion, got: %s", stmt)
	}
	if stmt := run(7); !strings.Contains(stmt, "id <> ") {
		t.Fatalf("enabled guard must exclude extract datasource, got: %s", stmt)
	}
}
