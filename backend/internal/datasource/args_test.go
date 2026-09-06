package datasource

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// 验证 mysql 驱动把 args 透传给底层 QueryContext（占位符 ? 与参数个数匹配才不报错）。
func TestMySQLExecutePassesArgs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT \\?").
		WithArgs("hello").
		WillReturnRows(rows)

	c := &mysqlConnection{db: db}
	res, err := c.Execute(context.Background(), "SELECT ?", "hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("args were not passed through: %v", err)
	}
}
