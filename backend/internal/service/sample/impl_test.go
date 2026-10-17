package sample

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func newMockService(t *testing.T) (*service, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &service{db: bun.NewDB(sqlDB, pgdialect.New())}, mock
}

func TestParseDatabaseURL(t *testing.T) {
	info, err := parseDatabaseURL("postgres://alice:s3cret@db.internal:6543/insight?sslmode=disable")
	if err != nil {
		t.Fatalf("parseDatabaseURL: %v", err)
	}
	if info.host != "db.internal" || info.port != 6543 || info.database != "insight" ||
		info.username != "alice" || info.password != "s3cret" {
		t.Fatalf("unexpected info: %+v", info)
	}

	info, err = parseDatabaseURL("postgresql://bob@localhost/app")
	if err != nil {
		t.Fatalf("parseDatabaseURL without port: %v", err)
	}
	if info.port != 5432 || info.password != "" || info.database != "app" {
		t.Fatalf("unexpected defaults: %+v", info)
	}

	if _, err := parseDatabaseURL("mysql://u:p@h/db"); err == nil {
		t.Fatal("non-postgres scheme must be rejected")
	}
	if _, err := parseDatabaseURL("postgres:///db"); err == nil {
		t.Fatal("missing host must be rejected")
	}
	if _, err := parseDatabaseURL(""); err == nil {
		t.Fatal("empty URL must be rejected")
	}
}

// TestStatusReadsLoadedAndDismissed 钉死引导卡片的两态读取：加载态看示例
// 数据源是否存在，忽略态看 bi_setting 的持久化值。
func TestStatusReadsLoadedAndDismissed(t *testing.T) {
	s, mock := newMockService(t)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM bi_datasource WHERE is_sample = TRUE`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'sample_dataset_dismissed'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Loaded || !status.Dismissed {
		t.Fatalf("expected loaded+dismissed, got %+v", status)
	}
}

func TestStatusDefaultsToUndismissed(t *testing.T) {
	s, mock := newMockService(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'sample_dataset_dismissed'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Loaded || status.Dismissed {
		t.Fatalf("expected default false/false, got %+v", status)
	}
}

// TestDismissPersistsChoice 验证「忽略」写 bi_setting 的 upsert，随后状态里
// dismissed=true（忽略选择持久化，刷新不再出现卡片）。
func TestDismissPersistsChoice(t *testing.T) {
	s, mock := newMockService(t)
	mock.ExpectExec(`INSERT INTO bi_setting \(key, value\) VALUES \('sample_dataset_dismissed', 'true'\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'sample_dataset_dismissed'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))

	status, err := s.Dismiss(context.Background())
	if err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if !status.Dismissed || status.Loaded {
		t.Fatalf("unexpected status after dismiss: %+v", status)
	}
}

// TestLoadCreatesHiddenDatasourceAndDatasets 验证加载态：一个 is_sample 数据源
// + 两个预置列的数据集，全部落在同一事务里，样例列带稳定 id / 角色。
func TestLoadCreatesHiddenDatasourceAndDatasets(t *testing.T) {
	s, mock := newMockService(t)
	s.databaseURL = "postgres://alice:secret@localhost:5432/insight"

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO "bi_datasource"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(21, nil))
	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(22, nil))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT value FROM bi_setting WHERE key = 'sample_dataset_dismissed'`).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	status, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !status.Loaded {
		t.Fatalf("expected loaded status, got %+v", status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSampleColumnsCarryIDsAndRoles(t *testing.T) {
	columns := sampleColumns(ordersDatasetName)
	if len(columns) == 0 {
		t.Fatal("orders sample must define columns")
	}
	seen := map[string]bool{}
	for _, col := range columns {
		if col.ID == "" {
			t.Fatalf("column %q missing stable id", col.Name)
		}
		if seen[col.ID] {
			t.Fatalf("duplicate column id %q", col.ID)
		}
		seen[col.ID] = true
		if col.Role != "dimension" && col.Role != "metric" {
			t.Fatalf("column %q has invalid role %q", col.Name, col.Role)
		}
	}
	// amount 是数值列，必须预置为 metric，否则拖拽后无法直接聚合。
	for _, col := range columns {
		if col.Name == "amount" && col.Role != "metric" {
			t.Fatalf("amount must be metric, got %q", col.Role)
		}
	}
	if !strings.Contains(mustColumnsJSON(columns), `"amount"`) {
		t.Fatal("columns JSON must include column names as expr")
	}
}

// TestRemoveSoftDeletesSampleRows 验证一键移除：图表随数据集级联软删，随后
// 数据集与数据源都打 deleted_at。
func TestRemoveSoftDeletesSampleRows(t *testing.T) {
	s, mock := newMockService(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM bi_datasource WHERE is_sample = TRUE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectExec(`UPDATE "bi_chart" AS "chart" SET deleted_at = now\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE "bi_dataset" AS "dataset" SET deleted_at = now\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE "bi_datasource" AS "datasource" SET deleted_at = now\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := s.Remove(context.Background()); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRemoveIsNoopWhenNotLoaded(t *testing.T) {
	s, mock := newMockService(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM bi_datasource WHERE is_sample = TRUE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	if err := s.Remove(context.Background()); err != nil {
		t.Fatalf("Remove on empty state: %v", err)
	}
}
