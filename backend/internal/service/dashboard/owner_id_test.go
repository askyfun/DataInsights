package dashboard

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"data-insights/internal/domain/entity"
)

// R-95（#220）：dashboard / dashboard-folder 的创建路径此前连占位 0 都不写
// （owner_id 列留 NULL），现在必须把 owner_id 带进 INSERT。值级换算
// （principal→真实 id / 匿名→0）由 model.ResolveOwnerID 的单测与
// service/datasource 的值级用例钉死，这里断言 INSERT 语句本身携带 owner_id 列。
func TestDashboardCreateWritesOwnerIDColumn(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectExec(`INSERT INTO "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(1, 1))

	if _, err := svc.Create(context.Background(), entity.DashboardCreateRequest{Name: "盘"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ins := capture.find(t, `INSERT INTO "bi_dashboard"`)
	if !strings.Contains(ins, `"owner_id"`) {
		t.Fatalf("dashboard INSERT must carry owner_id, got: %s", ins)
	}
}

func TestFolderCreateWritesOwnerIDColumn(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectExec(`INSERT INTO "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(1, 1))

	if _, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{Name: "夹"}); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	ins := capture.find(t, `INSERT INTO "bi_dashboard_folder"`)
	if !strings.Contains(ins, `"owner_id"`) {
		t.Fatalf("folder INSERT must carry owner_id, got: %s", ins)
	}
}
