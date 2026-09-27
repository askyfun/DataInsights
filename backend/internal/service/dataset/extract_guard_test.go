package dataset

import (
	"context"
	"strings"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/extract"
	"data-insights/internal/response"
	"data-insights/internal/router"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestExtractGuardRejectsCreateUpdate 验证抽取存储守卫（issue #118）：
// 指定后，创建/改指到该数据源的数据集一律 20100 拒绝，且错误可被 errors.Is 识别。
func TestExtractGuardRejectsCreateUpdate(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New()), extract: extract.Guard{DatasourceID: 7}}

	ds := &entity.Dataset{Name: "n", DatasourceID: 7, QueryType: "table"}
	for name, run := range map[string]func() error{
		"Create": func() error { _, err := s.Create(context.Background(), ds); return err },
		"Update": func() error { _, err := s.Update(context.Background(), ds); return err },
	} {
		err := run()
		if err == nil {
			t.Fatalf("%s on extract datasource must fail", name)
		}
		// BusinessError 只携带 Code+Message（router 按值断言），因此按消息断言；
		// 哨兵本身由 extract 包的单测覆盖 errors.Is 语义。
		bizErr, ok := err.(router.BusinessError)
		if !ok || bizErr.Code != response.CodeBadRequest {
			t.Fatalf("%s must surface as %d BusinessError, got %#v", name, response.CodeBadRequest, err)
		}
		if !strings.Contains(bizErr.Message, "extract storage") {
			t.Fatalf("%s message must name the extract storage violation, got %q", name, bizErr.Message)
		}
	}
}

// TestExtractGuardDisabledKeepsDirectPath 验证未启用（0）时守卫恒放行——直连零回归。
func TestExtractGuardDisabledKeepsDirectPath(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id"}).AddRow(1, "n", 7))
	if _, err := s.Create(context.Background(), &entity.Dataset{Name: "n", DatasourceID: 7, QueryType: "table"}); err != nil {
		t.Fatalf("Create with guard disabled: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
