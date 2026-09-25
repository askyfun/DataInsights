package dataset

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
)

// containsStmt 报告记录到的语句中是否有一条包含 sub（用于断言级联 UPDATE 的作用域）。
func containsStmtDS(executed []string, sub string) bool {
	for _, q := range executed {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

// TestDatasetDeleteSoftDeletesAndCascades 验证 Delete 是软删：只发 UPDATE 打
// deleted_at、绝不发 DELETE，并按 dataset→chart→share 顺序在同一事务内级联软删。
func TestDatasetDeleteSoftDeletesAndCascades(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "bi_chart"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "bi_share"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	for _, q := range executed {
		if strings.HasPrefix(strings.ToUpper(q), "DELETE") {
			t.Fatalf("soft delete must not issue DELETE, got: %s", q)
		}
	}
	if !containsStmtDS(executed, `SET deleted_at = now()`) {
		t.Fatalf("dataset soft delete must stamp deleted_at, got: %q", executed)
	}
	// 级联：分享经图表子查询作用域。
	if !containsStmtDS(executed, `UPDATE "bi_chart"`) {
		t.Fatalf("chart cascade missing, got: %q", executed)
	}
	if !containsStmtDS(executed, `UPDATE "bi_share"`) || !containsStmtDS(executed, `"bi_chart"`) {
		t.Fatalf("share cascade must scope to the dataset's charts, got: %q", executed)
	}
}

// TestDatasetDeleteIdempotent 验证重复删除（影响 0 行）不报错，保持幂等。
func TestDatasetDeleteIdempotent(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "bi_chart"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "bi_share"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := s.Delete(context.Background(), 999); err != nil {
		t.Fatalf("idempotent re-delete must not error, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestDatasetListFiltersDeletedAt 验证 List 只返回未删除行。
func TestDatasetListFiltersDeletedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
			AddRow(1, "n", 1, "table", nil, nil))

	if _, err := s.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	sel := findStmt(executed, "SELECT")
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Fatalf("List must filter soft-deleted rows, got: %s", sel)
	}
}

// TestDatasetListOrdersByIDDesc 先红：List 之前不带 ORDER BY，PostgreSQL 返回的是
// 堆物理序——UPDATE 写新行版本追加到堆尾，编辑过的数据集会漂到列表后面，用户看到
// 的"随机顺序"就是这么来的。修复：显式 ORDER BY id DESC，让新建/导入的靠前且稳定。
// 只喂 1 行即可，本用例断言的是 SQL 文本而非行序（sqlmock 不重排结果集）。
func TestDatasetListOrdersByIDDesc(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
			AddRow(2, "b", 1, "table", nil, nil))

	if _, err := s.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	sel := findStmt(executed, "SELECT")
	if !strings.Contains(sel, "ORDER BY id DESC") {
		t.Fatalf("List must order by id DESC, got: %s", sel)
	}
}

// TestDatasetGetByIDReturnsNotFoundForDeleted 验证软删后 GetByID 返回 NotFound。
func TestDatasetGetByIDReturnsNotFoundForDeleted(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}))

	if _, err := s.GetByID(context.Background(), 1); err == nil {
		t.Fatal("expected error for deleted/non-existent dataset")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// TestDatasetUpdateDeletedRecordFails 验证对已软删记录做 Update 失败。
func TestDatasetUpdateDeletedRecordFails(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasetService{db: db}

	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}))

	ds := &entity.Dataset{ID: 1, Name: "n", DatasourceID: 1, QueryType: "table"}
	if _, err := s.Update(context.Background(), ds); err == nil {
		t.Fatal("Update on a soft-deleted record must fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
