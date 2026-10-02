package dataset

import (
	"context"
	"testing"

	"data-insights/internal/domain/entity"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// issue #170：虚拟字段 expr 是用户可控且直进 SQL 的表达式，写入口必须过闸。
// 恶意表达式在 UpdateColumns 落库前就要被拒绝，而不是等查询期才发现。
func TestUpdateColumnsRejectsMaliciousExpr(t *testing.T) {
	malicious := []string{
		"price; DROP TABLE users",
		"(SELECT max(id) FROM users)",
		"pg_sleep(10)",
		"price -- comment",
		"price UNION SELECT 1",
	}
	for _, expr := range malicious {
		t.Run(expr, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer sqlDB.Close()
			// 过闸前不允许触达数据库：不注册任何 Expectation，越权写入即报错。
			mock.MatchExpectationsInOrder(false)
			mock.ExpectQuery(`SELECT .+ FROM "bi_dataset"`).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

			s := &datasetService{
				db: bun.NewDB(sqlDB, pgdialect.New()),
			}

			_, err = s.UpdateColumns(context.Background(), 1, []entity.DatasetColumn{
				{ID: "col1", Name: "amount", Expr: "amount"},
				{ID: "col2", Name: "evil", Expr: expr},
			})
			if err == nil {
				t.Fatalf("UpdateColumns(expr=%q) 必须被拒绝", expr)
			}
			if err := mock.ExpectationsWereMet(); err == nil {
				t.Fatalf("恶意表达式在过闸前就触发了数据库写入")
			}
		})
	}
}

// 合法虚拟字段表达式必须照常落库。
func TestUpdateColumnsAcceptsLegitimateExpr(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	mock.ExpectQuery(`SELECT .+ FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))

	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	updated, err := s.UpdateColumns(context.Background(), 1, []entity.DatasetColumn{
		{ID: "col1", Name: "amount", Expr: "amount"},
		{Name: "total", Type: "float", Role: "metric", Expr: "price * quantity"},
	})
	if err != nil {
		t.Fatalf("合法表达式被误拒: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated dataset")
	}
}
