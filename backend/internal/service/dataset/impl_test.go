package dataset

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"dataray/internal/datasource"
	"dataray/internal/model"
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
