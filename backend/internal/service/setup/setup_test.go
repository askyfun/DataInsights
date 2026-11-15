package setup

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func newTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	return NewService(db, "v9"), mock
}

// 空库：无管理员、无示例、无任何资源 → needs_admin=true。
func TestSetupStatus_Empty(t *testing.T) {
	svc, mock := newTestService(t)
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{"admins", "has_sample", "has_any"}).AddRow(0, false, false))
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.NeedsAdmin || st.HasSample || st.HasAnyResource {
		t.Fatalf("empty db expect needs_admin only, got %+v", st)
	}
	if st.Version != "v9" {
		t.Fatalf("version want v9 got %q", st.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// 非空库：已有管理员 + 已载示例 + 有资源 → needs_admin=false 等。
func TestSetupStatus_NonEmpty(t *testing.T) {
	svc, mock := newTestService(t)
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{"admins", "has_sample", "has_any"}).AddRow(1, true, true))
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.NeedsAdmin || !st.HasSample || !st.HasAnyResource {
		t.Fatalf("non-empty db expect admin/sample/resource true, got %+v", st)
	}
}
