package dataset

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"dataray/internal/crypto"
	"dataray/internal/datasource"
	"dataray/internal/domain/entity"
	"dataray/internal/model"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// stubConnection 实现 datasource.Connection，用于替换真实驱动。
type stubConnection struct {
	executeSQL    string
	executeErr    error
	resultColumns []string
	resultRows    []map[string]any

	getColumnsCalledWith string
	getColumnsResult     []datasource.ColumnInfo
	getColumnsErr        error
}

func (s *stubConnection) Close() error { return nil }
func (s *stubConnection) Ping(ctx context.Context) error { return nil }
func (s *stubConnection) GetTables(ctx context.Context) ([]datasource.TableInfo, error) {
	return nil, errors.New("not implemented")
}

func (s *stubConnection) GetColumns(ctx context.Context, tableName string) ([]datasource.ColumnInfo, error) {
	s.getColumnsCalledWith = tableName
	if s.getColumnsErr != nil {
		return nil, s.getColumnsErr
	}
	return s.getColumnsResult, nil
}

func (s *stubConnection) GetPrimaryKeys(ctx context.Context, tableName string) ([]string, error) {
	return nil, nil
}

func (s *stubConnection) Execute(ctx context.Context, query string, args ...any) (*datasource.QueryResult, error) {
	s.executeSQL = query
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	return &datasource.QueryResult{Columns: s.resultColumns, Rows: s.resultRows}, nil
}

// TestToDatasetModelNormalizesEmptyJSONFields 先红：空字符串写入 JSONB 列
// 会让 PG 报 invalid input syntax for type json；toDatasetModel 必须把
// 空 JSON 字段归一为合法 JSON（与空集合序列化为 [] 的响应契约一致）。
func TestToDatasetModelNormalizesEmptyJSONFields(t *testing.T) {
	empty := ""
	m := toDatasetModel(&entity.Dataset{
		Name:             "fixture",
		DatasourceID:     1,
		QueryType:        "table",
		AccelerateConfig: &empty,
		RefreshStrategy:  &empty,
		PreviewData:      &empty,
	})
	if m.Tags != "[]" {
		t.Fatalf("empty tags must normalize to [], got %q", m.Tags)
	}
	if m.QualityRules != "[]" {
		t.Fatalf("empty quality_rules must normalize to [], got %q", m.QualityRules)
	}
	if m.Columns != "[]" {
		t.Fatalf("empty columns must normalize to [], got %q", m.Columns)
	}
	if m.ShardKeys != "[]" {
		t.Fatalf("empty shard_keys must normalize to [], got %q", m.ShardKeys)
	}
	// 空 JSON 指针字段置 NULL，让列默认值生效，而不是把 "" 写进 JSONB
	if m.AccelerateConfig.Valid || m.RefreshStrategy.Valid || m.PreviewData.Valid {
		t.Fatalf("empty json pointer fields must be stored as NULL, got %+v", m)
	}
}

// TestGetColumnsSQLDatasetDerivesFromQuery 验证 SQL 型数据集在无已存列时，
// 通过 query 包单一通道执行包装后的查询并从结果列名推导列，
// 不再把 querySQL 拼成表名传给 GetColumns（该路径已被驱动校验拒绝）。
func TestGetColumnsSQLDatasetDerivesFromQuery(t *testing.T) {
	stub := &stubConnection{resultColumns: []string{"id", "name"}}
	s := &datasetService{
		getDatasetModelFn: func(ctx context.Context, id int) (*model.Dataset, error) {
			return &model.Dataset{
				ID:        1,
				QueryType: "sql",
				QuerySQL:  sql.NullString{String: "SELECT id, name FROM users", Valid: true},
				Columns:   "",
			}, nil
		},
		getDatasourceModelFn: func(ctx context.Context, id int) (*model.Datasource, error) {
			return &model.Datasource{ID: 2, Type: "mysql"}, nil
		},
		connectFn: func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
			return stub, nil
		},
	}

	cols, err := s.GetColumns(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 必须"执行查询"而不是调用 GetColumns
	if stub.getColumnsCalledWith != "" {
		t.Fatalf("GetColumns should not be called for sql datasets, got tableName %q", stub.getColumnsCalledWith)
	}
	wantSQL := "SELECT * FROM (SELECT id, name FROM users) AS _preview LIMIT 1"
	if stub.executeSQL != wantSQL {
		t.Fatalf("executed sql = %q, want %q", stub.executeSQL, wantSQL)
	}

	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d (%v)", len(cols), cols)
	}
	if cols[0].Name != "id" || cols[1].Name != "name" {
		t.Fatalf("unexpected column names: %v", cols)
	}
	for _, col := range cols {
		if col.Type != "unknown" {
			t.Errorf("column %q type = %q, want %q", col.Name, col.Type, "unknown")
		}
		if col.Role != "dimension" {
			t.Errorf("column %q role = %q, want dimension", col.Name, col.Role)
		}
	}
}

// TestGetColumnsTableDatasetUsesGetColumns 验证 table 型数据集路径保持现状：
// 校验后的 tableName 直连 GetColumns。
func TestGetColumnsTableDatasetUsesGetColumns(t *testing.T) {
	stub := &stubConnection{
		getColumnsResult: []datasource.ColumnInfo{{Name: "id", Type: "int"}},
	}
	s := &datasetService{
		getDatasetModelFn: func(ctx context.Context, id int) (*model.Dataset, error) {
			return &model.Dataset{
				ID:        1,
				QueryType: "table",
				TableName: sql.NullString{String: "users", Valid: true},
				Columns:   "",
			}, nil
		},
		getDatasourceModelFn: func(ctx context.Context, id int) (*model.Datasource, error) {
			return &model.Datasource{ID: 2, Type: "mysql"}, nil
		},
		connectFn: func(ctx context.Context, ds *model.Datasource) (datasource.Connection, error) {
			return stub, nil
		},
	}

	cols, err := s.GetColumns(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.getColumnsCalledWith != "users" {
		t.Fatalf("GetColumns called with %q, want %q", stub.getColumnsCalledWith, "users")
	}
	if stub.executeSQL != "" {
		t.Fatalf("Execute should not be called for table datasets, got %q", stub.executeSQL)
	}
	if len(cols) != 1 || cols[0].Name != "id" {
		t.Fatalf("unexpected columns: %v", cols)
	}
	if !strings.Contains(cols[0].Expr, "id") {
		t.Fatalf("unexpected expr: %q", cols[0].Expr)
	}
}

// TestMapDatasetColumnsBareIdentifier 先红：列表达式不能带 MySQL 反引号——
// 反引号会被查询层原样渲染，PostgreSQL 报 syntax error at or near "`"。
// 方言相关的引号是查询层（safeIdentifier/方言 builder）的职责。
func TestMapDatasetColumnsBareIdentifier(t *testing.T) {
	cols := mapDatasetColumns([]datasource.ColumnInfo{
		{Name: "region", Type: "varchar"},
		{Name: "amount", Type: "numeric"},
	})
	for _, col := range cols {
		if strings.ContainsAny(col.Expr, "`\"") {
			t.Fatalf("column expr must be a bare identifier, got %q", col.Expr)
		}
		if col.Expr != col.Name {
			t.Fatalf("expr should equal column name, got expr %q for %q", col.Expr, col.Name)
		}
	}
}

// --- password resolution tests (C1 fix) ---

func testAESKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// TestConnectResolvesPassword 验证 dataset 服务启用 key 后 connect 拿到的是
// 解密后的密码（dialFn 收到的值即写入 ConnectionConfig.Password 的值），
// 且存量明文密码被回写为 v1: 密文。
func TestConnectResolvesPassword(t *testing.T) {
	key := testAESKey()

	t.Run("encrypted password decrypted for dial", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, "fixture-credential-value")
		if err != nil {
			t.Fatal(err)
		}
		var got string
		s := &datasetService{}
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: ct}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-credential-value" {
			t.Fatalf("connect should dial with decrypted password, got %q", got)
		}
	})

	t.Run("legacy plaintext upgraded and used", func(t *testing.T) {
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer sqlDB.Close()

		var got string
		s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}
		s.SetSecurityKey(key)
		s.dialFn = func(ctx context.Context, ds *model.Datasource, password string) (datasource.Connection, error) {
			got = password
			return &stubConnection{}, nil
		}
		mock.ExpectExec(`UPDATE "bi_datasource"`).WillReturnResult(sqlmock.NewResult(0, 1))

		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: "fixture-plaintext-credential"}); err != nil {
			t.Fatal(err)
		}
		if got != "fixture-plaintext-credential" {
			t.Fatalf("connection should use original plaintext, got %q", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("legacy password was not upgraded: %v", err)
		}
	})
}
