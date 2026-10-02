package chart

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
)

func chartOwnerCapture(record *[]string) sqlmock.QueryMatcherFunc {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*record = append(*record, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

func chartOwnerStmt(executed []string, prefix string) string {
	for _, q := range executed {
		if strings.HasPrefix(q, prefix) {
			return q
		}
	}
	return ""
}

func TestChartCreateWritesOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartOwnerCapture(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &chartService{db: bun.NewDB(sqlDB, pgdialect.New())}

	// chart.Create 的 INSERT 被 bun 自动加了 RETURNING（自增主键 + nullzero deleted_at），
	// 于是走 Query 通道而非纯 Exec。
	returnRows := sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(1, nil)
	mock.ExpectQuery(`INSERT INTO "bi_chart"`).WillReturnRows(returnRows)

	if _, err := s.Create(context.Background(), &entity.Chart{
		Name: "c", DatasetID: 1, ChartType: "bar", Config: "{}",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ins := chartOwnerStmt(executed, `INSERT INTO "bi_chart"`)
	if !strings.Contains(ins, `"owner_id"`) {
		t.Fatalf("Create INSERT must carry owner_id, got: %s", ins)
	}
	// 占位常量必须是 0（与 bi_user.id 自增起点区分）。
	if model.SystemOwnerID != 0 {
		t.Fatalf("SystemOwnerID placeholder must be 0, got %d", model.SystemOwnerID)
	}
}

func TestChartUpdateDoesNotWriteOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(chartOwnerCapture(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &chartService{db: bun.NewDB(sqlDB, pgdialect.New())}

	reselect := sqlmock.NewRows([]string{"id", "name", "dataset_id", "chart_type", "config"}).
		AddRow(2, "c", 1, "bar", "{}")
	mock.ExpectExec(`UPDATE "bi_chart"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_chart"`).WillReturnRows(reselect)

	if _, err := s.Update(context.Background(), &entity.Chart{ID: 2, Name: "c", DatasetID: 1, ChartType: "bar", Config: "{}"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	upd := chartOwnerStmt(executed, `UPDATE "bi_chart"`)
	if strings.Contains(upd, `"owner_id" =`) {
		t.Fatalf("Update must exclude owner_id, got: %s", upd)
	}
}
