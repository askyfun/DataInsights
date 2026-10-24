package alert

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"data-insights/internal/domain/entity"
)

// R-95（#220）：预警规则的创建路径此前不写归属（owner_id 列留 NULL），现在必须
// 把 owner_id 带进 INSERT。值级换算由 model.ResolveOwnerID 的单测与
// service/datasource 的值级用例钉死，这里断言 INSERT 语句本身携带 owner_id 列。
func TestAlertCreateRuleWritesOwnerIDColumn(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "bi_alert_rule"`).WillReturnResult(sqlmock.NewResult(1, 1))

	if _, err := svc.CreateRule(context.Background(), entity.AlertRuleCreateRequest{
		Name: "n", ChartID: 1, Metric: "v", Operator: entity.AlertOperatorGT, Threshold: 1,
	}); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	ins := strings.Join(capture.stmts, "\n")
	if !strings.Contains(ins, `INSERT INTO "bi_alert_rule"`) {
		t.Fatalf("no alert rule INSERT captured: %q", capture.stmts)
	}
	// INSERT 语句必须显式携带 owner_id 列（此前整列缺席，落库为 NULL）。
	for _, stmt := range capture.stmts {
		if strings.HasPrefix(stmt, `INSERT INTO "bi_alert_rule"`) && !strings.Contains(stmt, `"owner_id"`) {
			t.Fatalf("alert rule INSERT must carry owner_id, got: %s", stmt)
		}
	}
}
