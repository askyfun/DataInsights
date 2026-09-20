package datasource

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
func containsStmt(executed []string, sub string) bool {
	for _, q := range executed {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

// TestDatasourceDeleteSoftDeletesAndCascades 验证 Delete 是软删：只发 UPDATE 打
// deleted_at、绝不发 DELETE，并按 datasource→dataset→chart→share 顺序在同一事务内
// 级联软删子实体。
func TestDatasourceDeleteSoftDeletesAndCascades(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasourceService{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))
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

	// 软删契约：绝无物理删除。
	for _, q := range executed {
		if strings.HasPrefix(strings.ToUpper(q), "DELETE") {
			t.Fatalf("soft delete must not issue DELETE, got: %s", q)
		}
	}
	// 数据源自身被打 deleted_at，且带 IS NULL 幂等条件。
	if !containsStmt(executed, `SET deleted_at = now()`) {
		t.Fatalf("datasource soft delete must stamp deleted_at, got: %q", executed)
	}
	// 级联：图表经数据集子查询、分享经图表+数据集子查询作用域。
	if !containsStmt(executed, `UPDATE "bi_chart"`) || !containsStmt(executed, `"bi_dataset"`) {
		t.Fatalf("chart cascade must scope to the datasource's datasets, got: %q", executed)
	}
	if !containsStmt(executed, `UPDATE "bi_share"`) || !containsStmt(executed, `"bi_chart"`) {
		t.Fatalf("share cascade must scope to the datasource's charts, got: %q", executed)
	}
}

// TestDatasourceDeleteIdempotent 验证重复删除（影响 0 行）不报错，保持幂等。
func TestDatasourceDeleteIdempotent(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasourceService{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 0))
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

// TestDatasourceListFiltersDeletedAt 验证 List 只返回未删除行。
func TestDatasourceListFiltersDeletedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasourceService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_datasource"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at"}).
			AddRow(1, "ds", "postgresql", "h", 5432, "db", "u", "", nil, nil))

	if _, err := s.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	sel := firstStmtWithPrefix(executed, "SELECT")
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Fatalf("List must filter soft-deleted rows, got: %s", sel)
	}
}

// TestDatasourceGetByIDReturnsNotFoundForDeleted 验证软删后 GetByID 返回 NotFound。
func TestDatasourceGetByIDReturnsNotFoundForDeleted(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasourceService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_datasource"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at"}))

	if _, err := s.GetByID(context.Background(), 1); err == nil {
		t.Fatal("expected error for deleted/non-existent datasource")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// TestDatasourceUpdateDeletedRecordFails 验证对已软删记录做 Update 失败
// （UPDATE 影响 0 行，回读因 deleted_at IS NULL 无匹配行）。
func TestDatasourceUpdateDeletedRecordFails(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &datasourceService{db: db}

	mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT .* FROM "bi_datasource"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at"}))

	ds := &entity.Datasource{ID: 3, Name: "ds", Type: "postgresql", Host: "h", Port: 5432, DatabaseName: "db", Username: "u", Password: "fixture-credential-value"}
	if _, err := s.Update(context.Background(), ds); err == nil {
		t.Fatal("Update on a soft-deleted record must fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
