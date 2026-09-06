package datasource

import (
	"context"
	"strings"
	"testing"
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
