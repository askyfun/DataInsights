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

func TestDatasetCreateWritesOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	insertRows := sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at", "owner_id"}).
		AddRow(1, "n", 1, "table", nil, nil, int32(model.SystemOwnerID))
	mock.ExpectQuery(`INSERT INTO "bi_dataset"`).WillReturnRows(insertRows)

	if _, err := s.Create(context.Background(), &entity.Dataset{Name: "n", DatasourceID: 1, QueryType: "table"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ins := findStmt(executed, `INSERT INTO "bi_dataset"`)
	if !strings.Contains(ins, `"owner_id"`) {
		t.Fatalf("Create INSERT must carry owner_id, got: %s", ins)
	}
}

func TestDatasetUpdateDoesNotWriteOwnerID(t *testing.T) {
	var executed []string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(datasetCaptureMatcher(&executed)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	s := &datasetService{db: bun.NewDB(sqlDB, pgdialect.New())}

	reselect := sqlmock.NewRows([]string{"id", "name", "datasource_id", "query_type", "created_at", "updated_at"}).
		AddRow(1, "n", 1, "table", nil, nil)
	mock.ExpectExec(`UPDATE "bi_dataset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "bi_dataset"`).WillReturnRows(reselect)

	if _, err := s.Update(context.Background(), &entity.Dataset{ID: 1, Name: "n", DatasourceID: 1, QueryType: "table"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	upd := findStmt(executed, `UPDATE "bi_dataset"`)
	if strings.Contains(upd, `"owner_id" =`) {
		t.Fatalf("Update must exclude owner_id, got: %s", upd)
	}
}
