package alert

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/query"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// 复用 dashboard 包的 sqlmock 测试模式：captureMatcher 记录真实下发的 SQL，
// 便于对「语句确实执行了」做断言（bun 生成的 SQL 形状在正则里锚定关键片段）。
type sqlCapture struct{ stmts []string }

type captureMatcher struct {
	inner   sqlmock.QueryMatcher
	capture *sqlCapture
}

func (m *captureMatcher) Match(expectedSQL, actualSQL string) error {
	m.capture.stmts = append(m.capture.stmts, actualSQL)
	return m.inner.Match(expectedSQL, actualSQL)
}

// 固定时钟（UTC）与固定 id：与 folder_test.go 的测试缝同款。
var testNow = time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)

const testRuleID = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e"

func newTestService(t *testing.T) (*alertService, sqlmock.Sqlmock, *sqlCapture) {
	t.Helper()
	capture := &sqlCapture{}
	sqldb, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(&captureMatcher{
		inner:   sqlmock.QueryMatcherRegexp,
		capture: capture,
	}))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	svc := &alertService{
		db:    bun.NewDB(sqldb, pgdialect.New()),
		now:   func() time.Time { return testNow },
		newID: func() (uuid.UUID, error) { return uuid.MustParse(testRuleID), nil },
	}
	return svc, mock, capture
}

// alertRuleColumns / alertRuleRow 构造评估器扫描用的规则行。
func alertRuleColumns() []string {
	return []string{"id", "name", "chart_id", "metric", "operator", "threshold",
		"enabled", "last_triggered_date", "owner_id", "tenant_id", "created_at", "updated_at", "deleted_at"}
}

func alertRuleRow(id string, lastTriggered *time.Time) *sqlmock.Rows {
	var last any
	if lastTriggered != nil {
		last = *lastTriggered
	}
	return sqlmock.NewRows(alertRuleColumns()).AddRow(
		id, "收入预警", 7, "revenue", "gt", 100.0,
		true, last, nil, nil, testNow, testNow, nil)
}

// fakeChartProvider 是 chartDataProvider 的假实现，记录取数次数并返回固定结果。
type fakeChartProvider struct {
	calls  int
	result entity.ChartDataResult
	err    error
}

func (f *fakeChartProvider) GetData(ctx context.Context, id int) (entity.ChartDataResult, error) {
	f.calls++
	if f.err != nil {
		return entity.ChartDataResult{}, f.err
	}
	return f.result, nil
}

// fakeNotifier 记录通知并按脚本返回错误。
type fakeNotifier struct {
	calls int
	err   error
}

func (f *fakeNotifier) Notify(ctx context.Context, rule *model.AlertRule, value, threshold float64) error {
	f.calls++
	return f.err
}

// expectTx 注册一次触发落库事务的四条语句（BEGIN / INSERT / UPDATE / COMMIT）。
func expectTx(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "bi_alert_trigger"`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE "bi_alert_rule"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// expectRuleScan 注册评估轮的规则扫描查询（必须先于 expectTx 注册：sqlmock
// 的期望按顺序消费）。
func expectRuleScan(mock sqlmock.Sqlmock, lastTriggered *time.Time) {
	mock.ExpectQuery(`SELECT .* FROM "bi_alert_rule"`).
		WillReturnRows(alertRuleRow(testRuleID, lastTriggered))
}

func runEvaluateAll(svc *alertService) {
	svc.evaluateAll(context.Background())
}

// --------------------------------------------------------------- evaluateValue

func TestEvaluateValue(t *testing.T) {
	cases := []struct {
		op        entity.AlertOperator
		value     float64
		threshold float64
		want      bool
	}{
		{entity.AlertOperatorGT, 101, 100, true},
		{entity.AlertOperatorGT, 100, 100, false},
		{entity.AlertOperatorGT, 99, 100, false},
		{entity.AlertOperatorLT, 99, 100, true},
		{entity.AlertOperatorLT, 100, 100, false},
		{entity.AlertOperatorEQ, 100, 100, true},
		{entity.AlertOperatorEQ, 100.5, 100, false},
		{"bogus", 100, 100, false},
	}
	for _, tc := range cases {
		if got := evaluateValue(tc.op, tc.value, tc.threshold); got != tc.want {
			t.Errorf("evaluateValue(%s, %v, %v) = %v, want %v", tc.op, tc.value, tc.threshold, got, tc.want)
		}
	}
}

// --------------------------------------------------------------- extractMetricValue

func TestExtractMetricValue(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]any
		key  string
		want float64
		ok   bool
	}{
		{"float", map[string]any{"v": 12.5}, "v", 12.5, true},
		{"int64", map[string]any{"v": int64(7)}, "v", 7, true},
		{"string number", map[string]any{"v": "123.5"}, "v", 123.5, true},
		{"missing column", map[string]any{"other": 1.0}, "v", 0, false},
		{"nil value", map[string]any{"v": nil}, "v", 0, false},
		{"not a number string", map[string]any{"v": "abc"}, "v", 0, false},
		{"NaN", map[string]any{"v": "NaN"}, "v", 0, false},
		{"unsupported type", map[string]any{"v": []any{1}}, "v", 0, false},
	}
	for _, tc := range cases {
		got, ok := extractMetricValue(tc.row, tc.key)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%s: extractMetricValue = (%v, %v), want (%v, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// --------------------------------------------------------------- evaluator 主链路

// 触发：首个越线行被记录，事务落 INSERT + UPDATE（去重标记）。
func TestEvaluateRule_TriggersOnFirstViolatingRow(t *testing.T) {
	svc, mock, capture := newTestService(t)
	provider := &fakeChartProvider{result: entity.ChartDataResult{Data: []map[string]any{
		{"revenue": "50"},
		{"revenue": 120.5},
	}}}
	svc.chartProvider = provider
	notifier := &fakeNotifier{}
	svc.notifier = notifier

	expectRuleScan(mock, nil)
	expectTx(mock)
	runEvaluateAll(svc)

	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
	joined := strings.Join(capture.stmts, "\n")
	if !strings.Contains(joined, `INSERT INTO "bi_alert_trigger"`) {
		t.Errorf("trigger insert missing, got: %s", joined)
	}
	if !strings.Contains(joined, `UPDATE "bi_alert_rule"`) {
		t.Errorf("dedup update missing, got: %s", joined)
	}
}

// KPI 形状（*query.KpiResponse，无行）：按单值评估，越线即触发。
func TestEvaluateRule_KpiResponseTriggers(t *testing.T) {
	svc, mock, capture := newTestService(t)
	svc.chartProvider = &fakeChartProvider{result: entity.ChartDataResult{
		Data: &query.KpiResponse{Value: 3000.5},
	}}
	svc.notifier = &fakeNotifier{}

	expectRuleScan(mock, nil)
	expectTx(mock)
	runEvaluateAll(svc)

	joined := strings.Join(capture.stmts, "\n")
	if !strings.Contains(joined, `INSERT INTO "bi_alert_trigger"`) {
		t.Errorf("trigger insert missing for KPI-shaped data, got: %s", joined)
	}
}

// KPI 形状未越线：跳过，不写库。
func TestEvaluateRule_KpiResponseBelowThresholdSkips(t *testing.T) {
	svc, mock, capture := newTestService(t)
	svc.chartProvider = &fakeChartProvider{result: entity.ChartDataResult{
		Data: &query.KpiResponse{Value: 50},
	}}
	svc.notifier = &fakeNotifier{}

	expectRuleScan(mock, nil)
	svc.evaluateAll(context.Background())

	if len(capture.stmts) != 1 {
		t.Fatalf("statements = %d, want 1 (no writes when KPI below threshold)", len(capture.stmts))
	}
}

// 同日去重：last_triggered_date 已是今天（UTC）→ 不取数、不写库。
func TestEvaluateRule_DedupSameUTCDay(t *testing.T) {
	svc, mock, capture := newTestService(t)
	provider := &fakeChartProvider{}
	svc.chartProvider = provider
	today := testNow
	expectRuleScan(mock, &today)
	runEvaluateAll(svc)

	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 (dedup must skip fetching)", provider.calls)
	}
	if len(capture.stmts) != 1 {
		t.Fatalf("statements = %d, want 1 (only the rule scan)", len(capture.stmts))
	}
}

// 未配置 notifier（nil）：触发照常落库。
func TestEvaluateRule_NilNotifierStillRecords(t *testing.T) {
	svc, mock, capture := newTestService(t)
	svc.chartProvider = &fakeChartProvider{result: entity.ChartDataResult{Data: []map[string]any{
		{"revenue": 200.0},
	}}}
	svc.notifier = nil

	expectRuleScan(mock, nil)
	expectTx(mock)
	runEvaluateAll(svc)

	if !strings.Contains(strings.Join(capture.stmts, "\n"), `INSERT INTO "bi_alert_trigger"`) {
		t.Errorf("trigger not recorded without notifier")
	}
}

// 通知失败：记录照常落库（notified=false 由 SQL 参数承载，语句层面验证不缺席）。
func TestEvaluateRule_NotifyErrorStillRecords(t *testing.T) {
	svc, mock, capture := newTestService(t)
	svc.chartProvider = &fakeChartProvider{result: entity.ChartDataResult{Data: []map[string]any{
		{"revenue": 200.0},
	}}}
	svc.notifier = &fakeNotifier{err: context.DeadlineExceeded}

	expectRuleScan(mock, nil)
	expectTx(mock)
	runEvaluateAll(svc)

	if !strings.Contains(strings.Join(capture.stmts, "\n"), `INSERT INTO "bi_alert_trigger"`) {
		t.Errorf("trigger not recorded despite notify error")
	}
}

// 整图无可用值：本周期跳过（不写库、不通知）。
func TestEvaluateRule_NoUsableValuesSkips(t *testing.T) {
	svc, mock, capture := newTestService(t)
	svc.chartProvider = &fakeChartProvider{result: entity.ChartDataResult{Data: []map[string]any{
		{"revenue": "not-a-number"},
	}}}
	svc.notifier = &fakeNotifier{}

	expectRuleScan(mock, nil)
	svc.evaluateAll(context.Background())

	if len(capture.stmts) != 1 {
		t.Fatalf("statements = %d, want 1 (no writes after skip)", len(capture.stmts))
	}
}

// --------------------------------------------------------------- Create 校验

func TestCreateRule_ValidationFailures(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		in      entity.AlertRuleCreateRequest
		wantMsg string
	}{
		{"empty name", entity.AlertRuleCreateRequest{Name: "", ChartID: 1, Metric: "v", Operator: entity.AlertOperatorGT}, "预警名称不能为空"},
		{"empty metric", entity.AlertRuleCreateRequest{Name: "n", ChartID: 1, Metric: "", Operator: entity.AlertOperatorGT}, "metric 不能为空"},
		{"bad operator", entity.AlertRuleCreateRequest{Name: "n", ChartID: 1, Metric: "v", Operator: "between"}, "operator 必须是 gt/lt/eq 之一"},
		{"zero chart", entity.AlertRuleCreateRequest{Name: "n", ChartID: 0, Metric: "v", Operator: entity.AlertOperatorGT}, "chart_id 必须为正整数"},
	}
	for _, tc := range cases {
		_, err := svc.CreateRule(ctx, tc.in)
		var biz router.BusinessError
		if !errors.As(err, &biz) || biz.Message != tc.wantMsg {
			t.Errorf("%s: err = %v, want business error %q", tc.name, err, tc.wantMsg)
		}
	}
}

// 图表不存在（软删同 20300）：COUNT 查询返回 0 时拒绝创建。
func TestCreateRule_ChartNotFound(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "bi_chart"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, err := svc.CreateRule(ctxBackground(), entity.AlertRuleCreateRequest{
		Name: "n", ChartID: 7, Metric: "v", Operator: entity.AlertOperatorGT,
	})
	var biz router.BusinessError
	if !errors.As(err, &biz) || biz.Code != response.CodeNotFound {
		t.Fatalf("err = %v, want CodeNotFound business error", err)
	}
}

func ctxBackground() context.Context { return context.Background() }
