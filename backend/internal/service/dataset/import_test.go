package dataset

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/extract"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// fakeConn records the statements and args an extract ingest sends, so the
// generated DDL/DML can be asserted without a live database.
type fakeConn struct {
	executed []string
	args     [][]any
	failOn   string
}

func (f *fakeConn) Close() error                                              { return nil }
func (f *fakeConn) Ping(context.Context) error                                { return nil }
func (f *fakeConn) GetTables(context.Context) ([]datasource.TableInfo, error) { return nil, nil }
func (f *fakeConn) GetColumns(context.Context, string) ([]datasource.ColumnInfo, error) {
	return nil, nil
}
func (f *fakeConn) GetPrimaryKeys(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeConn) Capabilities(context.Context) (*datasource.DialectCapabilities, error) {
	return &datasource.DialectCapabilities{}, nil
}
func (f *fakeConn) Execute(_ context.Context, query string, args ...any) (*datasource.QueryResult, error) {
	if f.failOn != "" && strings.Contains(query, f.failOn) {
		return nil, errors.New("fake exec failure")
	}
	f.executed = append(f.executed, query)
	f.args = append(f.args, args)
	return &datasource.QueryResult{}, nil
}

// newImportService wires a datasetService whose extract storage is datasource 9
// (StarRocks) backed by the given fake connection.
func newImportService(sqlDB *sql.DB, fake *fakeConn) *datasetService {
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}
	s.extract = extract.Guard{DatasourceID: 9}
	s.getDatasourceModelFn = func(context.Context, int) (*model.Datasource, error) {
		return &model.Datasource{ID: 9, Type: "starrocks"}, nil
	}
	s.dialFn = func(context.Context, *model.Datasource, string) (datasource.Connection, error) {
		return fake, nil
	}
	return s
}

func newMock(t *testing.T, executed *[]string) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return sqlDB, mock
}

func TestImportFileRequiresExtractStorage(t *testing.T) {
	var executed []string
	sqlDB, _ := newMock(t, &executed)
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	_, err := s.ImportFile(context.Background(), ImportRequest{Filename: "a.csv", Data: []byte("id\n1\n")})
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("expected %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
	if !strings.Contains(bizErr.Message, "EXTRACT_DATASOURCE_ID") {
		t.Fatalf("error must point at the missing config, got %q", bizErr.Message)
	}
	if len(executed) != 0 {
		t.Fatalf("no database work should happen when extract storage is unset, got %q", executed)
	}
}

func TestImportFileCreatesTableAndInserts(t *testing.T) {
	var executed []string
	sqlDB, mock := newMock(t, &executed)
	defer sqlDB.Close()
	fake := &fakeConn{}
	s := newImportService(sqlDB, fake)

	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "created_at", "updated_at"}).
			AddRow(42, "people", 9, "table", "extract", nil, nil))
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "created_at", "updated_at"}).
			AddRow(42, "people", 9, "table", "extract", nil, nil))

	ds, err := s.ImportFile(context.Background(), ImportRequest{
		Name:     "people",
		Filename: "people.csv",
		Data:     []byte("id,name\n1,alice\n2,bob\n"),
	})
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if ds.ID != 42 {
		t.Fatalf("dataset id = %d, want 42", ds.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	if len(fake.executed) != 2 {
		t.Fatalf("expected CREATE + INSERT, got %q", fake.executed)
	}
	if !strings.Contains(fake.executed[0], "CREATE TABLE `di_extract_42`") {
		t.Fatalf("unexpected DDL: %s", fake.executed[0])
	}
	if !strings.Contains(fake.executed[0], "DUPLICATE KEY(`id`)") {
		t.Fatalf("starrocks key missing: %s", fake.executed[0])
	}
	if !strings.Contains(fake.executed[1], "INSERT INTO `di_extract_42`") {
		t.Fatalf("unexpected DML: %s", fake.executed[1])
	}
	got := fake.args[1]
	want := []any{int64(1), "alice", int64(2), "bob"}
	if len(got) != len(want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
	if upd := findStmt(executed, `UPDATE "bi_dataset"`); !strings.Contains(upd, "di_extract_42") {
		t.Fatalf("table_name not recorded: %s", upd)
	}
}

func TestImportFileRollsBackOnIngestFailure(t *testing.T) {
	var executed []string
	sqlDB, mock := newMock(t, &executed)
	defer sqlDB.Close()
	fake := &fakeConn{failOn: "CREATE TABLE"}
	s := newImportService(sqlDB, fake)

	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "created_at", "updated_at"}).
			AddRow(7, "people", 9, "table", "extract", nil, nil))
	mock.ExpectExec(`DELETE FROM "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := s.ImportFile(context.Background(), ImportRequest{Filename: "people.csv", Data: []byte("id\n1\n")})
	if err == nil {
		t.Fatal("ingest failure must surface as an error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if sel := findStmt(executed, "DELETE"); !strings.Contains(sel, "bi_dataset") {
		t.Fatalf("failed ingest must roll the dataset row back, got %q", executed)
	}
}

func TestReplaceFileRejectsNonExtractDataset(t *testing.T) {
	var executed []string
	sqlDB, _ := newMock(t, &executed)
	defer sqlDB.Close()
	fake := &fakeConn{}
	s := newImportService(sqlDB, fake)
	s.getDatasetModelFn = func(context.Context, int) (*model.Dataset, error) {
		return &model.Dataset{ID: 1, Mode: "direct", DatasourceID: 3}, nil
	}

	_, err := s.ReplaceFile(context.Background(), 1, ImportRequest{Filename: "x.csv", Data: []byte("id\n1\n")})
	bizErr, ok := err.(router.BusinessError)
	if !ok || bizErr.Code != response.CodeBadRequest {
		t.Fatalf("expected %d BusinessError, got %#v", response.CodeBadRequest, err)
	}
	if len(fake.executed) != 0 {
		t.Fatalf("no ingestion should happen for a non-extract dataset, got %q", fake.executed)
	}
}

func TestDeleteDropsExtractTableWhenEnabled(t *testing.T) {
	var executed []string
	sqlDB, mock := newMock(t, &executed)
	defer sqlDB.Close()
	fake := &fakeConn{}
	s := newImportService(sqlDB, fake)

	// The pre-check lookup (getDatasetModel) returns an extract dataset.
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "mode", "created_at", "updated_at"}).
			AddRow(42, "people", 9, "table", "extract", nil, nil))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "bi_chart"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := s.Delete(context.Background(), 42); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
	if len(fake.executed) != 1 || !strings.Contains(fake.executed[0], "DROP TABLE IF EXISTS `di_extract_42`") {
		t.Fatalf("extract table must be dropped, got %q", fake.executed)
	}
}
