package chart

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"dataray/internal/domain/entity"
)

// chartCaptureMatcher 记录实际执行的 SQL，同时保留默认 regexp 匹配语义。
func chartCaptureMatcher(record *[]string) sqlmock.QueryMatcherFunc {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*record = append(*record, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

func containsStmtChart(executed []string, sub string) bool {
	for _, q := range executed {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

// TestChartDeleteSoftDeletesAndCascades 验证 Delete 是软删：只发 UPDATE 打
// deleted_at、绝不发 DELETE，并按 chart→share 顺序在同一事务内级联软删。
func TestChartDeleteSoftDeletesAndCascades(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &chartService{db: db}

	mock.ExpectBegin()
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
	if !containsStmtChart(executed, `SET deleted_at = now()`) {
		t.Fatalf("chart soft delete must stamp deleted_at, got: %q", executed)
	}
	if !containsStmtChart(executed, `UPDATE "bi_share"`) || !containsStmtChart(executed, `"bi_chart"`) {
		t.Fatalf("share cascade must scope to the chart, got: %q", executed)
	}
}

// TestChartDeleteIdempotent 验证重复删除（影响 0 行）不报错，保持幂等。
func TestChartDeleteIdempotent(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &chartService{db: db}

	mock.ExpectBegin()
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

// TestChartListFiltersDeletedAt 验证 List 只返回未删除行。
func TestChartListFiltersDeletedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &chartService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "dataset_id", "chart_type", "config", "created_at", "updated_at"}).
			AddRow(1, "c", 1, "bar", "{}", nil, nil))

	if _, err := s.List(context.Background(), 0, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	sel := firstStmtChart(executed, "SELECT")
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Fatalf("List must filter soft-deleted rows, got: %s", sel)
	}
}

// firstStmtChart 返回记录到的第一条以 prefix 开头的语句。
func firstStmtChart(executed []string, prefix string) string {
	for _, q := range executed {
		if strings.HasPrefix(q, prefix) {
			return q
		}
	}
	return ""
}

// TestChartGetByIDReturnsNotFoundForDeleted 验证软删后 GetByID 返回 NotFound。
func TestChartGetByIDReturnsNotFoundForDeleted(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &chartService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "dataset_id", "chart_type", "config", "created_at", "updated_at"}))

	if _, err := s.GetByID(context.Background(), 1); err == nil {
		t.Fatal("expected error for deleted/non-existent chart")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// TestChartUpdateDeletedRecordFails 验证对已软删记录做 Update 失败。
func TestChartUpdateDeletedRecordFails(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &chartService{db: db}

	mock.ExpectExec(`UPDATE "bi_chart"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "dataset_id", "chart_type", "config", "created_at", "updated_at"}))

	chart := &entity.Chart{ID: 1, Name: "c", DatasetID: 1, ChartType: "bar", Config: "{}"}
	if _, err := s.Update(context.Background(), chart); err == nil {
		t.Fatal("Update on a soft-deleted record must fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
