package dashboard

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
	"data-insights/internal/response"
	"data-insights/internal/router"
)

const (
	// testID 是固定的 dashboard id（UUIDv7 形态），由 newTestService 注入。
	testID       = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5d"
	testNow      = "2026-09-25T10:00:00Z"
	testLayout   = `{"version":1,"widgets":[{"widgetId":"w-1","type":"chart","x":0,"y":0,"w":6,"h":4,"chartId":42}]}`
	testChartRef = 42
)

// sqlCapture 记录真实下发给驱动的 SQL。
//
// sqlmock 的宽松正则只保证「跑到了这条语句」，不保证语句内部写对；而本包最容易被
// 写错的两处恰好都在语句内部：软删 UPDATE 是否把 deleted_at 带进 SET（为零值复活
// 已删行），以及引用计数是否真的用了 jsonpath 存在性查询。所以把真实 SQL 抓下来
// 逐条断言（与 service/queryrecord 同款哨兵写法）。
type sqlCapture struct{ stmts []string }

type captureMatcher struct {
	inner   sqlmock.QueryMatcher
	capture *sqlCapture
}

func (m *captureMatcher) Match(expectedSQL, actualSQL string) error {
	m.capture.stmts = append(m.capture.stmts, actualSQL)
	return m.inner.Match(expectedSQL, actualSQL)
}

// newTestService 组装一个跑在 sqlmock 上的服务，并把时钟与 id 源固定下来。
func newTestService(t *testing.T) (*dashboardService, sqlmock.Sqlmock, *sqlCapture) {
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

	svc := &dashboardService{
		db:    bun.NewDB(sqldb, pgdialect.New()),
		now:   func() time.Time { return time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC) },
		newID: func() (uuid.UUID, error) { return uuid.MustParse(testID), nil },
	}
	return svc, mock, capture
}

func (c *sqlCapture) all() []string { return c.stmts }

// find 返回第一条包含 substr 的语句；没有则失败并打印全部捕获内容。
func (c *sqlCapture) find(t *testing.T, substr string) string {
	t.Helper()
	for _, stmt := range c.stmts {
		if strings.Contains(stmt, substr) {
			return stmt
		}
	}
	t.Fatalf("未捕获到包含 %q 的语句，已捕获: %v", substr, c.stmts)
	return ""
}

func dashboardColumns() []string {
	return []string{
		"id", "name", "description", "layout_json", "status",
		"owner_id", "tenant_id", "created_at", "updated_at", "deleted_at",
	}
}

// storedRow 是 List/Get/Update 读到的存量行。
func storedRow() *sqlmock.Rows {
	return sqlmock.NewRows(dashboardColumns()).
		AddRow(testID, "月度经营总览", "本盘口径：GMV 含退款", testLayout, "draft",
			nil, nil, time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), nil)
}

func bizCode(t *testing.T, err error) int {
	t.Helper()
	var bizErr router.BusinessError
	if !errors.As(err, &bizErr) {
		t.Fatalf("期望业务错误，实际为 %v", err)
	}
	return bizErr.Code
}

func ptr(s string) *string { return &s }

// setClause 截出 UPDATE 语句的 SET 子句，供 T-9 哨兵断言其中不含 deleted_at。
func setClause(t *testing.T, stmt string) string {
	t.Helper()
	_, rest, ok := strings.Cut(stmt, " SET ")
	if !ok {
		t.Fatalf("语句里没有 SET 子句: %s", stmt)
	}
	set, _, ok := strings.Cut(rest, " WHERE ")
	if !ok {
		t.Fatalf("语句里没有 WHERE 子句: %s", stmt)
	}
	return set
}

// ---------------------------------------------------------------- Create

// TestCreateGeneratesUUIDv7AndDefaults 钉住创建契约的三件事：id 是后端生成的
// 合法 UUIDv7（不是自增整数）、status 缺省 draft、layout_json 缺省空文档。
func TestCreateGeneratesUUIDv7AndDefaults(t *testing.T) {
	capture := &sqlCapture{}
	sqldb, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(&captureMatcher{
		inner:   sqlmock.QueryMatcherRegexp,
		capture: capture,
	}))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	// 走真实 NewService（不注入 id 源），验证 uuid.NewV7 确实接在默认路径上。
	svc := NewService(bun.NewDB(sqldb, pgdialect.New())).(*dashboardService)

	mock.ExpectExec(`INSERT INTO "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(1, 1))

	got, err := svc.Create(context.Background(), entity.DashboardCreateRequest{Name: "空盘"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	parsed, err := uuid.Parse(got.ID)
	if err != nil {
		t.Fatalf("id 不是合法 UUID: %q (%v)", got.ID, err)
	}
	if parsed.Version() != 7 {
		t.Errorf("id 版本 = %d, want 7 (UUIDv7)", parsed.Version())
	}
	if got.Status != entity.DashboardStatusDraft {
		t.Errorf("status = %q, want %q", got.Status, entity.DashboardStatusDraft)
	}
	if got.LayoutJSON != entity.DashboardDefaultLayoutJSON {
		t.Errorf("layout_json = %q, want %q", got.LayoutJSON, entity.DashboardDefaultLayoutJSON)
	}
	if got.Description != nil {
		t.Errorf("未提供的 description 应为 null，实际 %q", *got.Description)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("时间戳未打戳: %+v", got)
	}
}

// TestCreateUsesInjectedIDAndInput 钉住注入点与「原样透传」：给定 id 与显式字段时
// 不再走缺省分支，且 description/layout/status 逐字段生效。
func TestCreateUsesInjectedIDAndInput(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectExec(`INSERT INTO "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(1, 1))

	got, err := svc.Create(context.Background(), entity.DashboardCreateRequest{
		Name:        "月度经营总览",
		Description: ptr("口径说明"),
		LayoutJSON:  testLayout,
		Status:      "published",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if got.ID != testID {
		t.Errorf("id = %q, want %q", got.ID, testID)
	}
	if got.Description == nil || *got.Description != "口径说明" {
		t.Errorf("description 未透传: %+v", got.Description)
	}
	if got.LayoutJSON != testLayout {
		t.Errorf("layout_json 未透传: %q", got.LayoutJSON)
	}
	if got.Status != "published" {
		t.Errorf("status 未透传: %q", got.Status)
	}
	if got.CreatedAt != testNow || got.UpdatedAt != testNow {
		t.Errorf("时间戳 = %q/%q, want %q", got.CreatedAt, got.UpdatedAt, testNow)
	}
}

// TestCreateRejectsMalformedLayoutJSON 畸形 JSON 必须在落库前挡住：列是 JSONB，
// 否则会以 500 的形式从 PostgreSQL 冒出来。
func TestCreateRejectsMalformedLayoutJSON(t *testing.T) {
	svc, mock, _ := newTestService(t)

	_, err := svc.Create(context.Background(), entity.DashboardCreateRequest{
		Name:       "坏布局",
		LayoutJSON: "{bad",
	})
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Errorf("code = %d, want %d", code, response.CodeBadRequest)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("畸形布局不应发出任何 SQL: %v", err)
	}
}

// ---------------------------------------------------------------- List

func TestListFiltersDeletedAtAndOrdersNewestFirst(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).WillReturnRows(storedRow())

	got, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	sel := capture.find(t, `FROM "bi_dashboard"`)
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Errorf("List 必须过滤软删行: %s", sel)
	}
	if !strings.Contains(sel, "ORDER BY created_at DESC, id DESC") {
		t.Errorf("排序口径必须与 idx_dashboard_not_deleted 一致: %s", sel)
	}
	if !strings.Contains(sel, "LIMIT 100") {
		t.Errorf("limit<=0 时应回落缺省 100: %s", sel)
	}

	if len(got) != 1 {
		t.Fatalf("期望 1 行，实际 %d", len(got))
	}
	if got[0].ID != testID || got[0].Name != "月度经营总览" {
		t.Errorf("实体映射错误: %+v", got[0])
	}
	if got[0].Description == nil || *got[0].Description != "本盘口径：GMV 含退款" {
		t.Errorf("description 映射错误: %+v", got[0].Description)
	}
	if got[0].Status != "draft" || got[0].LayoutJSON != testLayout {
		t.Errorf("layout/status 映射错误: %+v", got[0])
	}
}

// ---------------------------------------------------------------- Get

func TestGetNotFound(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows(dashboardColumns()))

	_, err := svc.Get(context.Background(), testID)
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeNotFound, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if sel := capture.find(t, `FROM "bi_dashboard"`); !strings.Contains(sel, "deleted_at IS NULL") {
		t.Errorf("Get 必须过滤软删行: %s", sel)
	}
}

// ---------------------------------------------------------------- Update

// TestUpdatePreservesUnprovidedFields 钉住「未提供则保留」：只传 name 时，
// description / layout_json / status 必须原样留存，而不是被零值覆盖。
func TestUpdatePreservesUnprovidedFields(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).WillReturnRows(storedRow())
	mock.ExpectExec(`UPDATE "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := svc.Update(context.Background(), testID, entity.DashboardUpdateRequest{Name: ptr("改名后的盘")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	if got.Name != "改名后的盘" {
		t.Errorf("name 未更新: %q", got.Name)
	}
	if got.Description == nil || *got.Description != "本盘口径：GMV 含退款" {
		t.Errorf("未提供的 description 必须保留存量: %+v", got.Description)
	}
	if got.LayoutJSON != testLayout {
		t.Errorf("未提供的 layout_json 必须保留存量: %q", got.LayoutJSON)
	}
	if got.Status != "draft" {
		t.Errorf("未提供的 status 必须保留存量: %q", got.Status)
	}
	if got.CreatedAt != "2026-09-24T08:00:00Z" {
		t.Errorf("created_at 必须保留: %q", got.CreatedAt)
	}
	if got.UpdatedAt != testNow {
		t.Errorf("updated_at 必须前进到当前时间: %q", got.UpdatedAt)
	}

	// ⚠️ T-9 哨兵：整行 UPDATE 的 SET 子句里绝不能出现 deleted_at，否则零值会把
	// 已软删的行复活。
	upd := capture.find(t, `UPDATE "bi_dashboard"`)
	set := setClause(t, upd)
	if strings.Contains(set, "deleted_at") {
		t.Errorf("UPDATE 的 SET 子句不得包含 deleted_at（T-9 复活风险）: %s", set)
	}
	if !strings.Contains(upd, "deleted_at IS NULL") {
		t.Errorf("UPDATE 必须带 deleted_at IS NULL 兜住已删行: %s", upd)
	}
}

// TestUpdateClearsDescriptionWithEmptyString 空串是唯一可达的「清空」形态
// （JSON 里缺省与 null 在 *string 上都解成 nil，只能表示保留）。
func TestUpdateClearsDescriptionWithEmptyString(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).WillReturnRows(storedRow())
	mock.ExpectExec(`UPDATE "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := svc.Update(context.Background(), testID, entity.DashboardUpdateRequest{Description: ptr("")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Description != nil {
		t.Errorf("空串应清空 description，实际 %q", *got.Description)
	}
}

func TestUpdateRejectsMalformedLayoutJSON(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).WillReturnRows(storedRow())

	_, err := svc.Update(context.Background(), testID, entity.DashboardUpdateRequest{LayoutJSON: ptr("[1,2")})
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeBadRequest, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	for _, stmt := range capture.all() {
		if strings.HasPrefix(stmt, "UPDATE") {
			t.Fatalf("畸形布局不应发出 UPDATE: %s", stmt)
		}
	}
}

func TestUpdateOnMissingRowIsNotFound(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows(dashboardColumns()))

	_, err := svc.Update(context.Background(), testID, entity.DashboardUpdateRequest{Name: ptr("x")})
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeNotFound, err)
	}
	for _, stmt := range capture.all() {
		if strings.HasPrefix(stmt, "UPDATE") {
			t.Fatalf("行不存在时不应发出 UPDATE: %s", stmt)
		}
	}
}

// ---------------------------------------------------------------- Delete

// TestDeleteIsSoft 钉住软删：只打 deleted_at、绝不发 DELETE，且用按需 SET 子句
// 而不是整行更新（T-9 的另一半）。
func TestDeleteIsSoft(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectExec(`UPDATE "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.Delete(context.Background(), testID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	upd := capture.find(t, `UPDATE "bi_dashboard"`)
	if !strings.Contains(upd, "SET deleted_at = now()") {
		t.Errorf("软删必须打 deleted_at: %s", upd)
	}
	if !strings.Contains(upd, "deleted_at IS NULL") {
		t.Errorf("软删必须幂等限定未删行: %s", upd)
	}
	for _, stmt := range capture.all() {
		if strings.HasPrefix(strings.ToUpper(stmt), "DELETE") {
			t.Fatalf("软删不得发出 DELETE: %s", stmt)
		}
	}
}

// TestDeleteIsIdempotent 重复删除（影响 0 行）不报错。
func TestDeleteIsIdempotent(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectExec(`UPDATE "bi_dashboard"`).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := svc.Delete(context.Background(), testID); err != nil {
		t.Fatalf("重复软删必须幂等，实际: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ---------------------------------------------------------------- CountChartReferences

// TestCountChartReferencesUsesJSONPath 是引用计数的哨兵：必须走 jsonb 存在性查询
// （@? + jsonpath），绝不能退化成 layout_json::text LIKE 的子串扫描。
func TestCountChartReferencesUsesJSONPath(t *testing.T) {
	svc, mock, capture := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows(dashboardColumns()).
			AddRow(testID, "月度经营总览", nil, testLayout, "draft",
				nil, nil, time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), nil))

	got, err := svc.CountChartReferences(context.Background(), testChartRef)
	if err != nil {
		t.Fatalf("CountChartReferences: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	sel := capture.find(t, `FROM "bi_dashboard"`)
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Errorf("引用计数只应返回未软删的盘: %s", sel)
	}
	if !strings.Contains(sel, `layout_json @? `) {
		t.Errorf("必须用 jsonb 存在性运算符 @?: %s", sel)
	}
	if !strings.Contains(sel, `$.widgets[*] ? (@.chartId == 42)`) {
		t.Errorf("jsonpath 未按 widgets[*] 逐块匹配 chartId: %s", sel)
	}
	if strings.Contains(strings.ToUpper(sel), "LIKE") {
		t.Errorf("禁止用 LIKE 子串匹配实现引用计数: %s", sel)
	}

	if len(got) != 1 || got[0].ID != testID || got[0].Name != "月度经营总览" {
		t.Fatalf("实体映射错误: %+v", got)
	}
}
