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

// 覆盖 postgresql rebind 的三种情形：普通转换、多个占位符、无占位符。
// 已知风险（本任务不修）：rebind 不感知引号内的 '?'，含字面量问号的 SQL 会被错误改写。
func TestRebind(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single placeholder", "SELECT * FROM t WHERE a = ?", "SELECT * FROM t WHERE a = $1"},
		{"multiple placeholders", "SELECT * FROM t WHERE a = ? AND b = ?", "SELECT * FROM t WHERE a = $1 AND b = $2"},
		{"no placeholder", "SELECT 1", "SELECT 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rebind(tc.in); got != tc.want {
				t.Fatalf("rebind(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
