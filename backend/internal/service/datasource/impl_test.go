package datasource

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"dataray/internal/crypto"
	"dataray/internal/domain/entity"
	"dataray/internal/model"
)

// TestBuildDistributionSQLsSQLDatasetUsesQuerySQL verifies that for SQL-type
// datasets (front end sends table_name="" and query_sql=<user SQL>) both the
// distribution and total SQL wrap the user querySQL, not the empty tableName
// (which used to produce "FROM () as _subquery").
func TestBuildDistributionSQLsSQLDatasetUsesQuerySQL(t *testing.T) {
	distSQL, totalSQL, err := buildDistributionSQLs("region", "", "SELECT region FROM sales", "sql", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantSub := "FROM (SELECT region FROM sales) as _subquery"
	if !strings.Contains(distSQL, wantSub) {
		t.Fatalf("dist SQL missing user subquery: %s", distSQL)
	}
	if !strings.Contains(totalSQL, wantSub) {
		t.Fatalf("total SQL missing user subquery: %s", totalSQL)
	}
}

// TestBuildDistributionSQLsTableBranch verifies the table branch keeps using
// the (validated) tableName for both SQLs and rejects invalid identifiers.
func TestBuildDistributionSQLsTableBranch(t *testing.T) {
	distSQL, totalSQL, err := buildDistributionSQLs("status", "public.orders", "", "table", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(distSQL, "FROM public.orders GROUP BY status") {
		t.Fatalf("unexpected dist SQL: %s", distSQL)
	}
	if !strings.Contains(totalSQL, "FROM public.orders") {
		t.Fatalf("unexpected total SQL: %s", totalSQL)
	}

	if _, _, err := buildDistributionSQLs("status", "orders; DROP TABLE orders", "", "table", 20); err == nil {
		t.Fatal("expected error for invalid table name")
	}
	if _, _, err := buildDistributionSQLs("a;b", "orders", "", "table", 20); err == nil {
		t.Fatal("expected error for invalid field name")
	}
}

// TestGetFieldDistributionRejectsInvalidFieldName verifies the service-level
// contract: an invalid fieldName is rejected before any SQL is built or any
// DB access happens (validation runs first in GetFieldDistribution).
func TestGetFieldDistributionRejectsInvalidFieldName(t *testing.T) {
	s := &datasourceService{} // nil db: validation must fail before getDatasourceModel is reached
	_, err := s.GetFieldDistribution(context.Background(), 1, "orders", "", "table", "1; DROP TABLE x", 10)
	if err == nil || !strings.Contains(err.Error(), "invalid field name") {
		t.Fatalf("expected invalid field name error, got %v", err)
	}
}

// TestPreviewRejectsInvalidTableName verifies the service-level contract for
// Preview: an invalid tableName in the table branch is rejected before any
// DB access happens.
func TestPreviewRejectsInvalidTableName(t *testing.T) {
	s := &datasourceService{} // nil db: validation must fail before getDatasourceModel is reached
	_, err := s.Preview(context.Background(), 1, "orders; DROP TABLE orders", "", "table")
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("expected invalid table name error, got %v", err)
	}
}

// TestGetTableDataRejectsInvalidTableName verifies the service-level contract
// for GetTableData: an invalid tableName is rejected before any DB access.
func TestGetTableDataRejectsInvalidTableName(t *testing.T) {
	s := &datasourceService{} // nil db: validation must fail before getDatasourceModel is reached
	_, err := s.GetTableData(context.Background(), 1, "orders; DROP TABLE orders", 1, 20, "", "")
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("expected invalid table name error, got %v", err)
	}
}

// --- security integration tests (Task 8b) ---
// --- security integration tests (Task 8b) ---

func testSecurityKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// captureMatcherFunc 记录实际执行的 SQL 并保留默认的 regexp 匹配语义，
// 便于断言写库语句中的密码形态（密文每次随机，无法提前断言精确值）。
func captureMatcherFunc(record *[]string) sqlmock.QueryMatcherFunc {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*record = append(*record, actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

// TestCreateEncryptsPassword 验证有 key 时 Create 写库的 password 是 v1: 密文，
// 且无 key 时保持明文透传（兼容本地无 key 开发）。
func TestCreateEncryptsPassword(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())

	returnRows := sqlmock.NewRows([]string{"id", "name", "type", "host", "port", "database_name", "username", "password", "created_at", "updated_at"}).
		AddRow(1, "ds", "postgresql", "h", 5432, "db", "u", "", nil, nil)

	ds := &entity.Datasource{
		Name: "ds", Type: "postgresql", Host: "h", Port: 5432,
		DatabaseName: "db", Username: "u", Password: "s3cret",
	}

	// 有 key：INSERT 语句中的 password 是 v1: 密文
	s := &datasourceService{db: db, key: testSecurityKey()}
	mock.ExpectQuery(`INSERT INTO "bi_datasource"`).WillReturnRows(returnRows)
	_, err = s.Create(context.Background(), ds)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if len(executed) != 1 || !strings.Contains(executed[0], "'v1:") {
		t.Fatalf("expected encrypted password in INSERT, got: %q", executed)
	}

	// 无 key：密码原样明文写库
	executed = nil
	s2 := &datasourceService{db: db}
	mock.ExpectQuery(`INSERT INTO "bi_datasource"`).WillReturnRows(returnRows)
	if _, err := s2.Create(context.Background(), ds); err != nil {
		t.Fatalf("Create (plaintext mode): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if len(executed) != 1 || !strings.Contains(executed[0], "'s3cret'") || strings.Contains(executed[0], "'v1:") {
		t.Fatalf("plaintext mode must keep password as-is, got: %q", executed)
	}
}

// TestConnectDecryptsAndAutoUpgrades 验证 connect 的密码解析路径：
// 存量明文密码原样用于连接且回写 v1: 密文；已加密密码解密且不重复写库。
func TestConnectDecryptsAndAutoUpgrades(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureMatcherFunc(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())

	key := testSecurityKey()
	s := &datasourceService{db: db, key: key}

	// 存量明文：返回原文用于连接，同时回写 v1: 密文
	mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))
	got, err := s.resolvePassword(context.Background(), &model.Datasource{ID: 2, Password: "legacy-pass"})
	if err != nil {
		t.Fatalf("resolvePassword (legacy): %v", err)
	}
	if got != "legacy-pass" {
		t.Fatalf("connection should use original plaintext, got %q", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if len(executed) != 1 || !strings.Contains(executed[0], `SET "password" = 'v1:`) {
		t.Fatalf("legacy password was not upgraded to ciphertext, got: %q", executed)
	}

	// 已加密：解密为明文，无额外写库
	executed = nil
	ct, err := crypto.Encrypt(key, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.resolvePassword(context.Background(), &model.Datasource{ID: 2, Password: ct})
	if err != nil {
		t.Fatalf("resolvePassword (encrypted): %v", err)
	}
	if got != "s3cret" {
		t.Fatalf("expected decrypted password, got %q", got)
	}
	if len(executed) != 0 {
		t.Fatalf("encrypted password must not trigger write-back, got: %q", executed)
	}
}

// TestConnectFailsOnGarbageCiphertext 验证带 v1: 前缀但内容损坏的密文
// 导致连接报错，而不是静默降级为明文。
func TestConnectFailsOnGarbageCiphertext(t *testing.T) {
	key := testSecurityKey()
	s := &datasourceService{
		key: key,
		getDatasourceModelFn: func(ctx context.Context, id int) (*model.Datasource, error) {
			return &model.Datasource{ID: 2, Type: "postgresql", Password: "v1:not-valid-base64!!!"}, nil
		},
	}

	_, err := s.GetTables(context.Background(), 2)
	if err == nil {
		t.Fatal("expected error for garbage ciphertext")
	}
	if strings.Contains(err.Error(), "not encrypted") {
		t.Fatalf("garbage ciphertext must not be treated as legacy plaintext: %v", err)
	}
}
