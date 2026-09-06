package dataset

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"dataray/internal/crypto"
	"dataray/internal/datasource"
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
		ct, err := crypto.Encrypt(key, "s3cret")
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
		if got != "s3cret" {
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

		if _, err := s.connect(context.Background(), &model.Datasource{ID: 2, Type: "postgresql", Password: "legacy-pass"}); err != nil {
			t.Fatal(err)
		}
		if got != "legacy-pass" {
			t.Fatalf("connection should use original plaintext, got %q", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("legacy password was not upgraded: %v", err)
		}
	})
}
