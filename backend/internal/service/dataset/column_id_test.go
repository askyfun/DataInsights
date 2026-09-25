package dataset

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

// newDatasetServiceWithMockDB 构造一个只带 sqlmock 库的 service，返回记录语句的切片。
func newDatasetServiceWithMockDB(t *testing.T) (*datasetService, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	executed := &[]string{}
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}, mock, executed
}

// TestAssignColumnIDsAssignsPreservesAndDedupes 钉住列 ID 分配的三条不变量：
// 缺 ID 的补发、已有 ID 原样保留（换 ID 等于切断图表引用）、重复 ID 只保留首次出现者。
func TestAssignColumnIDsAssignsPreservesAndDedupes(t *testing.T) {
	columns := []entity.DatasetColumn{
		{Name: "a", Expr: "a"},
		{ID: "keep0001", Name: "b", Expr: "b"},
		{ID: "dup00001", Name: "c", Expr: "c"},
		{ID: "dup00001", Name: "d", Expr: "d"},
	}

	if !assignColumnIDs(columns) {
		t.Fatal("assignColumnIDs must report a change")
	}
	if columns[0].ID == "" {
		t.Error("missing id must be minted")
	}
	if columns[1].ID != "keep0001" {
		t.Errorf("existing ids must be preserved, got %q", columns[1].ID)
	}
	if columns[2].ID != "dup00001" {
		t.Errorf("first occurrence of a duplicated id must win, got %q", columns[2].ID)
	}
	if columns[3].ID == "dup00001" || columns[3].ID == "" {
		t.Errorf("duplicated id must be re-minted, got %q", columns[3].ID)
	}

	seen := make(map[string]struct{}, len(columns))
	for _, col := range columns {
		if _, dup := seen[col.ID]; dup {
			t.Fatalf("duplicate id %q after assignment: %+v", col.ID, columns)
		}
		seen[col.ID] = struct{}{}
	}

	// 幂等：全列已有唯一 ID 时二次调用必须无改动（GET 路径每次都会跑它）
	if assignColumnIDs(columns) {
		t.Fatal("assignColumnIDs must be a no-op once every column carries a unique id")
	}
}

// TestGetColumnsBackfillsIDsOnLegacySavedColumns 验证本特性之前保存的列
// （JSON 里没有 id 键）在读取时被补上 ID 并回写——否则图表配置与 shard_keys
// 没有可引用的稳定标识。
func TestGetColumnsBackfillsIDsOnLegacySavedColumns(t *testing.T) {
	s, mock, executed := newDatasetServiceWithMockDB(t)
	s.getDatasetModelFn = func(_ context.Context, id int) (*model.Dataset, error) {
		return &model.Dataset{
			ID: id,
			// 未访问数据源：getDatasourceModelFn 保持 nil，一旦被调用会 panic
			Columns: `[{"name":"id","expr":"id","type":"string","comment":"","role":"dimension"},` +
				`{"name":"amount","expr":"SUM(amount)","type":"float","comment":"","role":"metric"}]`,
		}, nil
	}
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))

	cols, err := s.GetColumns(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetColumns: %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	for _, col := range cols {
		if col.ID == "" {
			t.Errorf("legacy column %q must get a backfilled id", col.Name)
		}
	}
	if cols[0].ID == cols[1].ID {
		t.Fatalf("backfilled ids must be unique, both %q", cols[0].ID)
	}
	// 表达式与角色必须原样保留：补 ID 绝不能改列语义
	if cols[1].Expr != "SUM(amount)" || cols[1].Role != "metric" {
		t.Fatalf("backfill must not touch expr/role, got %+v", cols[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("backfilled ids must be persisted: %v", err)
	}
	if !strings.Contains(findStmt(*executed, `UPDATE "bi_dataset"`), `"columns"`) {
		t.Fatalf("backfill must write only the columns column, got: %q", *executed)
	}
}

// TestUpdateColumnsPreservesAndMintsIDs 验证保存列定义时：已带 ID 的列原样保留，
// 新建列（前端不传 ID）由后端补发。
func TestUpdateColumnsPreservesAndMintsIDs(t *testing.T) {
	s, mock, executed := newDatasetServiceWithMockDB(t)

	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
			AddRow(1, "n", 1, "table", nil, nil))
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))

	columns := []entity.DatasetColumn{
		{ID: "keep0001", Name: "renamed", Expr: "raw_col", Type: "string", Role: "dimension"},
		{Name: "virtual", Expr: "a + b", Type: "float", Role: "metric"},
	}

	if _, err := s.UpdateColumns(context.Background(), 1, columns); err != nil {
		t.Fatalf("UpdateColumns: %v", err)
	}
	if columns[0].ID != "keep0001" {
		t.Fatalf("existing id must survive a rename, got %q", columns[0].ID)
	}
	if columns[1].ID == "" {
		t.Fatal("new column must be minted an id")
	}

	upd := findStmt(*executed, `UPDATE "bi_dataset"`)
	for _, id := range []string{columns[0].ID, columns[1].ID} {
		if !strings.Contains(upd, id) {
			t.Fatalf("persisted columns must carry id %q, got: %s", id, upd)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
