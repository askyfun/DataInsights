package datasource

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestListHidesSampleDatasource 钉死内置示例数据源（R-79）的隐藏口径：
// 数据源列表的 SQL 恒带 is_sample = FALSE，与抽取存储守卫是否启用无关。
func TestListHidesSampleDatasource(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasourceService{db: bun.NewDB(sqlDB, pgdialect.New())}
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type"}).AddRow(1, "ds", "postgresql"))

	if _, err := s.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, stmt := range executed {
		if strings.Contains(stmt, `FROM "bi_datasource"`) {
			if !strings.Contains(stmt, "is_sample = FALSE") {
				t.Fatalf("sample datasource must always be hidden, got: %s", stmt)
			}
			return
		}
	}
	t.Fatalf("no SELECT on bi_datasource captured: %q", executed)
}
