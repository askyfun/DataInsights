package datasource

import (
	"context"
	"strings"
	"testing"
)

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
