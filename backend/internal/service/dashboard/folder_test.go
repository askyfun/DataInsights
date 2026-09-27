package dashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
)

// 夹侧测试用的固定 id：kid 是 root 的子夹，move 三态与环检测都围绕这一对展开。
const (
	folderRootID = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5e"
	folderKidID  = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5f"
	folderOther  = "0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c60"
	folderNow    = "2026-09-25T10:00:00Z"
)

// bun 在这个包生成的 SELECT 只有两种形状会落到同一张文件夹表上，而 sqlmock 的正则
// 匹配器是**非锚定**的子串匹配（`^` 有效、`$` 无效），所以「全行投影」的正则是
// 「祖先步」正则的超集 —— 必须显式写出 WHERE 的差异，否则先注册的宽正则会把
// 后面的窄正则吃掉，报出一条完全不相干语句的匹配失败。
//
//	sqlFolderRow     全行：… WHERE (deleted_at IS NULL) AND ("dashboard_folder"."id" = …)
//	sqlFolderAncestry 单列：SELECT "dashboard_folder"."parent_id" … WHERE (id = …)
//	（bun 把 WherePK 之后的手写条件排在软删过滤之前，WherePK 那条则排在之后。）
const (
	sqlFolderRow      = `SELECT "dashboard_folder"."id", "dashboard_folder"."name".* FROM "bi_dashboard_folder" AS "dashboard_folder" WHERE \(deleted_at IS NULL\) AND \("dashboard_folder"\."id"`
	sqlFolderRowAny   = `SELECT "dashboard_folder"."id", "dashboard_folder"."name".* FROM "bi_dashboard_folder"`
	sqlFolderAncestry = `SELECT "dashboard_folder"\."parent_id" FROM "bi_dashboard_folder" AS "dashboard_folder" WHERE \(id = `
)

// sqlFolderRowFor 把主键值一起写进正则。bun 对字符串主键**内联字面量**（不走占位符），
// 于是同一条 WherePK 语句在一张表上出现两次时只有 id 不同；不带 id 的正则会被第一条
// 语句整体吃掉（子串匹配、无顺序感知之外的区分手段），第二次就匹配不上了。
func sqlFolderRowFor(id string) string {
	return sqlFolderRow + ` = '` + id + `'`
}

// newFolderTestService 与 newTestService 同款：sqlmock + 固定时钟 + 固定 id 源，
// 但装的是 folderService（同一张 mock 连接，两个类型各跑各的语句）。
func newFolderTestService(t *testing.T) (*folderService, sqlmock.Sqlmock, *sqlCapture) {
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

	svc := &folderService{
		db:    bun.NewDB(sqldb, pgdialect.New()),
		now:   func() time.Time { return time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC) },
		newID: func() (uuid.UUID, error) { return uuid.MustParse(folderRootID), nil },
	}
	return svc, mock, capture
}

func folderColumns() []string {
	return []string{"id", "name", "parent_id", "owner_id", "tenant_id", "created_at", "updated_at", "deleted_at"}
}

// folderRow 造一行夹。parentID 传 nil 表示根级（列值 NULL）。
func folderRow(id, name string, parentID *string) *sqlmock.Rows {
	var parent any
	if parentID != nil {
		parent = *parentID
	}
	return sqlmock.NewRows(folderColumns()).AddRow(
		id, name, parent, nil, nil,
		time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), nil)
}

func sptr(s string) *string { return &s }

// ---------------------------------------------------------------- List

// TestFolderListIsFlatAndUnpaginated 钉住「读出口是扁平数组」这一条契约：
// 组树归前端，所以 SQL 里既没有递归 CTE 也没有 LIMIT/OFFSET。
func TestFolderListIsFlatAndUnpaginated(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRowAny).
		WillReturnRows(sqlmock.NewRows(folderColumns()).
			AddRow(folderRootID, "经营分析", nil, nil, nil, time.Now(), time.Now(), nil).
			AddRow(folderKidID, "周报", folderRootID, nil, nil, time.Now(), time.Now(), nil))

	got, err := svc.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 条扁平记录, 实际 %d", len(got))
	}

	sel := capture.find(t, `FROM "bi_dashboard_folder"`)
	if strings.Contains(sel, "LIMIT") || strings.Contains(sel, "OFFSET") {
		t.Errorf("列表不该分页: %s", sel)
	}
	if strings.Contains(sel, "WITH RECURSIVE") {
		t.Errorf("后端不该组树（不该出现递归 CTE）: %s", sel)
	}
	if !strings.Contains(sel, "deleted_at IS NULL") {
		t.Errorf("列表必须过滤软删行: %s", sel)
	}
	if !strings.Contains(sel, "created_at ASC") {
		t.Errorf("列表应按创建时间升序（先序稳定）: %s", sel)
	}
	// parent_id 的两种形态：根级 null、子级字符串。
	if got[0].ParentID != nil {
		t.Errorf("根级夹的 parent_id 必须是 null, 实际 %q", *got[0].ParentID)
	}
	if got[1].ParentID == nil || *got[1].ParentID != folderRootID {
		t.Errorf("子夹的 parent_id 未回显: %+v", got[1].ParentID)
	}
}

// ---------------------------------------------------------------- Create

func TestFolderCreateRootLevel(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	// 根级：不该有任何「父级存在性」查询，直接 INSERT。
	mock.ExpectExec(`INSERT INTO "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(1, 1))

	got, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{Name: "经营分析"})
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if got.ID != folderRootID {
		t.Errorf("id = %q, want 注入的 %q", got.ID, folderRootID)
	}
	if got.ParentID != nil {
		t.Errorf("根级的 parent_id 必须是 null, 实际 %q", *got.ParentID)
	}
	if got.CreatedAt != folderNow || got.UpdatedAt != folderNow {
		t.Errorf("时间戳 = %q/%q, want %q", got.CreatedAt, got.UpdatedAt, folderNow)
	}
	ins := capture.find(t, `INSERT INTO "bi_dashboard_folder"`)
	if !strings.Contains(ins, "parent_id") {
		t.Errorf("INSERT 必须带上 parent_id 列（否则根级写不出 NULL）: %s", ins)
	}
}

func TestFolderCreateUnderParentChecksExistence(t *testing.T) {
	svc, mock, _ := newFolderTestService(t)
	// 父级存在性 → 再 INSERT。顺序是关键：先查后插，悬空父级不落库。
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))
	mock.ExpectExec(`INSERT INTO "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(1, 1))

	got, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{
		Name: "周报", ParentID: folderRootID,
	})
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != folderRootID {
		t.Errorf("parent_id 未透传: %+v", got.ParentID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderCreateMissingParentIsNotFound(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(sqlmock.NewRows(folderColumns()))

	_, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{
		Name: "周报", ParentID: folderOther,
	})
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeNotFound, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	// 拒绝之后绝不能已经插了行（ExpectationsWereMet 只保证「该跑的跑了」，
	// 这条反向断言保证「不该跑的没跑」）。
	if len(capture.all()) != 1 {
		t.Errorf("父级不存在时不该执行 INSERT, 实际语句: %v", capture.all())
	}
}

func TestFolderCreateEmptyNameRejected(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	_, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{Name: ""})
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Fatalf("code = %d, want %d", code, response.CodeBadRequest)
	}
	if len(capture.all()) != 0 {
		t.Errorf("名称为空不该产生任何 SQL: %v", capture.all())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderCreateMalformedParentRejectedBeforeQuery(t *testing.T) {
	svc, _, capture := newFolderTestService(t)
	_, err := svc.CreateFolder(context.Background(), entity.DashboardFolderCreateRequest{
		Name: "周报", ParentID: "not-a-uuid",
	})
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Fatalf("code = %d, want %d", code, response.CodeBadRequest)
	}
	// 形态非法当场拒绝：把畸形串交给 uuid 列只会得到一个 PostgreSQL 转换 500。
	if len(capture.all()) != 0 {
		t.Errorf("非法 parent_id 不该下发任何 SQL: %v", capture.all())
	}
}

// ---------------------------------------------------------------- Update

func TestFolderUpdateRenameKeepsParent(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderKidID, "周报", sptr(folderRootID)))
	mock.ExpectExec(`UPDATE "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := svc.UpdateFolder(context.Background(), folderKidID, entity.DashboardFolderUpdateRequest{
		Name: sptr("周报归档"),
	})
	if err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != folderRootID {
		t.Errorf("只改名时 parent_id 必须保留存量: %+v", got.ParentID)
	}
	set := setClause(t, capture.find(t, `UPDATE "bi_dashboard_folder"`))
	// T-9 复活哨兵（同 dashboard：零值 DeletedAt 进 SET 会把软删行当场复活）。
	if strings.Contains(set, "deleted_at") {
		t.Errorf("SET 不得包含 deleted_at（会复活已软删的行）: %s", set)
	}
	// created_at 是本次新写出来的整行更新，漂成 now() 的后果是「建夹时间变了」，
	// 而列表排序正是按它 —— 这条哨兵专门盯它。
	if strings.Contains(set, "created_at") {
		t.Errorf("SET 不得包含 created_at: %s", set)
	}
	if !strings.Contains(set, "updated_at") {
		t.Errorf("SET 必须前进 updated_at（表无触发器）: %s", set)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderUpdateMoveToRootWritesNull(t *testing.T) {
	svc, mock, _ := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderKidID, "周报", sptr(folderRootID)))
	mock.ExpectExec(`UPDATE "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := svc.UpdateFolder(context.Background(), folderKidID, entity.DashboardFolderUpdateRequest{
		ParentID: sptr(""),
	})
	if err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	// ""（移到根级）与 nil（保留存量）的分野就在这里：必须落回 null。
	if got.ParentID != nil {
		t.Errorf("空串哨兵必须把 parent 清成 null, 实际 %q", *got.ParentID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderUpdateMoveUnderSelfRejected(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))

	_, err := svc.UpdateFolder(context.Background(), folderRootID, entity.DashboardFolderUpdateRequest{
		ParentID: sptr(folderRootID),
	})
	if code := bizCode(t, err); code != response.CodeBusinessError {
		t.Fatalf("code = %d, want %d", code, response.CodeBusinessError)
	}
	if len(capture.all()) != 1 {
		t.Errorf("移动到自身不该再有后续 SQL（既不查祖先也不写库）: %v", capture.all())
	}
}

// TestFolderUpdateCycleRejectedByWalkingAncestors 钉住环检测的**方向**：
// 从 target 往上爬祖先链，而不是把 id 的整棵子树拉下来。断言的是语句形状——
// 每一步都只取 parent_id 这一列，所以爬链代价只随深度增长。
func TestFolderUpdateCycleRejectedByWalkingAncestors(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	// 存量 root（根级）+ 父级存在性 kid，两条 WherePK 全行查询只差 id：
	// bun 内联主键字面量，正则又是不锚定的，所以必须各写各的 id（见 sqlFolderRowFor），
	// 否则第一条语句会把宽正则整个吃掉。
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))
	mock.ExpectQuery(sqlFolderRowFor(folderKidID)).
		WillReturnRows(folderRow(folderKidID, "周报", sptr(folderRootID)))
	// target=kid 的祖先步（只取 parent_id 一列）→ 返回其 parent_id=root。
	// 正则靠 WHERE 的书写顺序与全行投影区分（bun 把 WherePK 的条件排在软删过滤之后），
	// 单靠 SELECT 列无法区分：全行投影也含 parent_id 子串。
	mock.ExpectQuery(sqlFolderAncestry).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(folderRootID))

	_, err := svc.UpdateFolder(context.Background(), folderRootID, entity.DashboardFolderUpdateRequest{
		ParentID: sptr(folderKidID),
	})
	if code := bizCode(t, err); code != response.CodeBusinessError {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeBusinessError, err)
	}
	for _, stmt := range capture.all() {
		if strings.Contains(stmt, "UPDATE") {
			t.Errorf("环检测失败后不该写库: %s", stmt)
		}
		if strings.Contains(stmt, "WITH RECURSIVE") {
			t.Errorf("不该用递归拉整棵子树: %s", stmt)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestFolderUpdateAncestorsExhaustedAllowsMove 覆盖爬链的正常终点：爬到根级
// （parent_id IS NULL）说明 target 不是后代，移动应当放行。
func TestFolderUpdateAncestorsExhaustedAllowsMove(t *testing.T) {
	svc, mock, _ := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderKidID, "周报", sptr(folderRootID)))
	// target 存在性检查（另一条 WherePK，必须带自己的 id）
	mock.ExpectQuery(sqlFolderRowFor(folderOther)).
		WillReturnRows(folderRow(folderOther, "另一支", nil))
	// 爬第一级：target 的 parent 是 NULL → 链到头。
	mock.ExpectQuery(sqlFolderAncestry).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
	mock.ExpectExec(`UPDATE "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := svc.UpdateFolder(context.Background(), folderKidID, entity.DashboardFolderUpdateRequest{
		ParentID: sptr(folderOther),
	})
	if err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != folderOther {
		t.Errorf("parent_id 未更新: %+v", got.ParentID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderUpdateEmptyNameRejected(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))

	_, err := svc.UpdateFolder(context.Background(), folderRootID, entity.DashboardFolderUpdateRequest{
		Name: sptr(""),
	})
	if code := bizCode(t, err); code != response.CodeBadRequest {
		t.Fatalf("code = %d, want %d", code, response.CodeBadRequest)
	}
	for _, stmt := range capture.all() {
		if strings.Contains(stmt, "UPDATE") {
			t.Errorf("空名不该写库: %s", stmt)
		}
	}
}

// ---------------------------------------------------------------- Delete

func TestFolderDeleteEmptyStampsDeletedAt(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))
	// 子夹计数
	mock.ExpectQuery(`SELECT count\(.*\) FROM "bi_dashboard_folder"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 夹内仪表盘计数
	mock.ExpectQuery(`SELECT count\(.*\) FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`UPDATE "bi_dashboard_folder"`).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.DeleteFolder(context.Background(), folderRootID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	// 软删必须是「只打 deleted_at」的定向 UPDATE，而不是整行写回：整行写回会用
	// 零值把别列刷掉（troubleshooting T-9 的同一族问题）。
	upd := capture.find(t, `UPDATE "bi_dashboard_folder"`)
	set := setClause(t, upd)
	if set != `deleted_at = now()` {
		t.Errorf("软删 SET 应恰为 deleted_at = now(), 实际 %q", set)
	}
	if !strings.Contains(upd, "deleted_at IS NULL") {
		t.Errorf("软删 WHERE 必须挡住已删行（幂等的来源）: %s", upd)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFolderDeleteRefusedByChildFolders(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))
	mock.ExpectQuery(`SELECT count\(.*\) FROM "bi_dashboard_folder"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	err := svc.DeleteFolder(context.Background(), folderRootID)
	if code := bizCode(t, err); code != response.CodeBusinessError {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeBusinessError, err)
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("拒绝消息要带上子夹数量, 实际 %q", err.Error())
	}
	// 第一条守卫命中就够：不该再数仪表盘（省一条 SQL），更不该写库。
	for _, stmt := range capture.all() {
		if strings.Contains(stmt, `FROM "bi_dashboard" AS "dashboard"`) || strings.Contains(stmt, "UPDATE") {
			t.Errorf("已有子夹时不该继续（数仪表盘或写库）: %s", stmt)
		}
	}
}

func TestFolderDeleteRefusedByDashboards(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(folderRow(folderRootID, "经营分析", nil))
	mock.ExpectQuery(`SELECT count\(.*\) FROM "bi_dashboard_folder"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT count\(.*\) FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	err := svc.DeleteFolder(context.Background(), folderRootID)
	if code := bizCode(t, err); code != response.CodeBusinessError {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeBusinessError, err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("拒绝消息要带上仪表盘数量, 实际 %q", err.Error())
	}
	// 计数条件必须走 folder_id 归属 + 软删过滤，和迁移 00007 的索引同口径。
	cnt := capture.find(t, `FROM "bi_dashboard" AS "dashboard"`)
	if !strings.Contains(cnt, "folder_id") || !strings.Contains(cnt, "deleted_at IS NULL") {
		t.Errorf("仪表盘计数条件不对: %s", cnt)
	}
	for _, stmt := range capture.all() {
		if strings.Contains(stmt, "UPDATE") {
			t.Errorf("夹非空时不该写库: %s", stmt)
		}
	}
}

func TestFolderDeleteMissingIsNotFound(t *testing.T) {
	svc, mock, capture := newFolderTestService(t)
	mock.ExpectQuery(sqlFolderRow).
		WillReturnRows(sqlmock.NewRows(folderColumns()))

	err := svc.DeleteFolder(context.Background(), folderRootID)
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("code = %d, want %d", code, response.CodeNotFound)
	}
	if len(capture.all()) != 1 {
		t.Errorf("夹不存在不该继续探测/写库: %v", capture.all())
	}
}

// ---------------------------------------------------------------- 表名钉

// TestFolderTableName 钉住 bun 模型与迁移 00007 的表名对齐：改名会让所有语句
// 指向一张不存在的表，而 sqlmock 的正则匹配器照样能让用例绿。
func TestFolderTableName(t *testing.T) {
	if got := (&model.DashboardFolder{}).TableName(); got != "bi_dashboard_folder" {
		t.Errorf("TableName() = %q, want bi_dashboard_folder", got)
	}
	if got := (&model.Dashboard{}).TableName(); got != "bi_dashboard" {
		t.Errorf("dashboard TableName() = %q", got)
	}
}
