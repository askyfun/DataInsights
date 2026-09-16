package share

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// TestShareListFiltersDeletedAt 验证分享列表只返回未删除行（被级联软删的分享不出现）。
func TestShareListFiltersDeletedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &shareService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows().AddRow(1, "tok", 1, nil, nil, nil))

	if _, err := s.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	sel := firstStmtShare(executed, "SELECT")
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Fatalf("share List must filter soft-deleted rows, got: %s", sel)
	}
}

// TestShareGetByTokenFiltersDeletedAt 验证被级联软删的分享经 token 取不到（NotFound）。
func TestShareGetByTokenFiltersDeletedAt(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db := bun.NewDB(sqlDB, pgdialect.New())
	s := &shareService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "bi_share"`).
		WillReturnRows(shareRows())

	if _, err := s.GetByToken(context.Background(), "gone"); err == nil {
		t.Fatal("expected error for soft-deleted/non-existent share")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// firstStmtShare 返回记录到的第一条以 prefix 开头的语句。
func firstStmtShare(executed []string, prefix string) string {
	for _, q := range executed {
		if strings.HasPrefix(q, prefix) {
			return q
		}
	}
	return ""
}
