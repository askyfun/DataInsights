package datasource

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"data-insights/internal/domain/entity"
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
