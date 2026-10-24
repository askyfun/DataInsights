package datasource

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
	"data-insights/internal/listpage"
	"data-insights/internal/model"
)

// R-22 归属（#171）：Create 必须把 owner_id 写进 INSERT（打占位 id）；Update 走整行
// WherePK 更新，必须把 owner_id 排除出 SET，否则 toModel 带进来的零值会把已归属行重置。
func TestDatasourceCreateWritesOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasourceService{db: bun.NewDB(sqlDB, pgdialect.New())}

	rows := sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at", "owner_id"}).
		AddRow(1, "ds", "postgresql", "h", 5432, "db", "u", "", nil, nil, int32(model.SystemOwnerID))
	mock.ExpectQuery(`INSERT INTO "bi_datasource"`).WillReturnRows(rows)

	if _, err := s.Create(context.Background(), &entity.Datasource{
		Name: "ds", Type: "postgresql", Host: "h", Port: 5432,
		DatabaseName: "db", Username: "u", Password: "fixture-credential-value",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ins := firstStmtWithPrefix(executed, `INSERT INTO "bi_datasource"`)
	if ins == "" {
		t.Fatalf("no INSERT captured: %q", executed)
	}
	if !strings.Contains(ins, `"owner_id"`) {
		t.Fatalf("Create INSERT must carry owner_id, got: %s", ins)
	}
}

func TestDatasourceUpdateDoesNotWriteOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasourceService{db: bun.NewDB(sqlDB, pgdialect.New())}

	reselect := sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at"}).
		AddRow(3, "ds", "postgresql", "h", 5432, "db", "u", "", nil, nil)
	mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_datasource"`).WillReturnRows(reselect)

	// 非空 password 走 encrypt 分支（nil key → no-op），不触发保留旧密码的回读分支。
	if _, err := s.Update(context.Background(), &entity.Datasource{
		ID: 3, Name: "ds", Type: "postgresql", Host: "h", Port: 5432,
		DatabaseName: "db", Username: "u", Password: "fixture-credential-value",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	upd := firstStmtWithPrefix(executed, `UPDATE "bi_datasource"`)
	if upd == "" {
		t.Fatalf("no UPDATE captured: %q", executed)
	}
	if strings.Contains(upd, `"owner_id" =`) {
		t.Fatalf("Update must exclude owner_id (归属不随更新改写), got: %s", upd)
	}
}

// argsHook 通过 bun 的 QueryHook 捕获每条语句：bun 对 INSERT 不在 QueryEvent
// 里回传绑定参数（query_base.go 的 beforeQuery 传 nil），所以「登录 → 当前用户 /
// 匿名 → 0」的**参数值**级断言由真库往返用例 TestZZRealPGDatasourceOwnerIDRoundTrip
// 承担（zz_ 前缀探针约定，TEST_DATABASE_URL 未设置时跳过）；这里只断言 INSERT
// 语句文本显式携带 owner_id 列。
type argsHook struct{ events []bun.QueryEvent }

func (h *argsHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context { return ctx }

func (h *argsHook) AfterQuery(_ context.Context, e *bun.QueryEvent) {
	h.events = append(h.events, *e)
}

// insertHasOwnerColumn 断言捕获到的 bi_datasource INSERT 语句携带 owner_id 列
// （纯文本包含判断，不拼任何 SQL）。
func insertHasOwnerColumn(t *testing.T, h *argsHook) {
	t.Helper()
	for i := range h.events {
		q := h.events[i].Query
		if !strings.HasPrefix(q, `INSERT INTO "bi_datasource"`) {
			continue
		}
		if strings.Contains(q, `"owner_id"`) {
			return
		}
		t.Fatalf("INSERT carries no owner_id column: %s", q)
	}
	t.Fatalf("no bi_datasource INSERT captured in %d events", len(h.events))
}

// TestDatasourceCreateOwnerIDFollowsPrincipal 钉住写入口径：归属注入位
// （entity.OwnerID）一路进 INSERT 的 owner_id 列。
func TestDatasourceCreateOwnerIDFollowsPrincipal(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	hook := &argsHook{}
	db := bun.NewDB(sqlDB, pgdialect.New())
	db.AddQueryHook(hook)
	s := &datasourceService{db: db}

	rows := sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at", "owner_id"}).
		AddRow(1, "ds", "postgresql", "h", 5432, "db", "u", "", nil, nil, model.SystemOwnerID)
	mock.ExpectQuery(`INSERT INTO "bi_datasource"`).WillReturnRows(rows)

	if _, err := s.Create(context.Background(), &entity.Datasource{
		Name: "ds", Type: "postgresql", Host: "h", Port: 5432,
		DatabaseName: "db", Username: "u", Password: "fixture-credential-value",
		OwnerID: 7,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	insertHasOwnerColumn(t, hook)
}

// TestDatasourceListFillsOwnerName 是 R-95 验收 2 的读出口用例：列表按行回填
// 创建人用户名，owner_id=0 的行保持 null；解析用户名的查询只按 id IN 过滤
// （禁用用户的名字照常返回）。
func TestDatasourceListFillsOwnerName(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasourceService{db: bun.NewDB(sqlDB, pgdialect.New())}
	ctx := context.Background()

	mock.ExpectQuery(`SELECT count\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT .* FROM "bi_datasource"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "is_sample", "created_at", "updated_at", "owner_id"}).
			AddRow(1, "a", "postgresql", "h", 5432, "d", "u", "", false, nil, nil, 1).
			AddRow(2, "b", "mysql", "h", 3306, "d", "u", "", false, nil, nil, 0))
	mock.ExpectQuery(`FROM "bi_user" AS "user" WHERE \(id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(1, "alice"))

	out, err := s.List(ctx, listpage.Params{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(out.Items))
	}
	if out.Items[0].OwnerName == nil || *out.Items[0].OwnerName != "alice" {
		t.Fatalf("items[0].owner_name = %v, want alice", out.Items[0].OwnerName)
	}
	if out.Items[1].OwnerName != nil {
		t.Fatalf("items[1].owner_name = %v, want null (owner_id=0)", *out.Items[1].OwnerName)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
