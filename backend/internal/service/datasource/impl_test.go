package datasource

import (
	"context"
	"strings"
	"testing"
)

func TestBuildFieldDistributionSQL(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		source    string
		queryType string
		limit     int
		want      string
		wantErr   string
	}{
		{
			name:      "table branch valid",
			fieldName: "status",
			source:    "public.orders",
			queryType: "table",
			limit:     20,
			want:      "SELECT status, COUNT(*) as _count FROM public.orders GROUP BY status ORDER BY _count DESC LIMIT 20",
		},
		{
			name:      "sql branch valid",
			fieldName: "region",
			source:    "SELECT region, amount FROM sales",
			queryType: "sql",
			limit:     10,
			want:      "SELECT region, COUNT(*) as _count FROM (SELECT region, amount FROM sales) as _subquery GROUP BY region ORDER BY _count DESC LIMIT 10",
		},
		{
			name:      "invalid field name rejected",
			fieldName: "1; DROP TABLE x",
			source:    "orders",
			queryType: "table",
			limit:     20,
			wantErr:   "invalid field name",
		},
		{
			name:      "invalid field name rejected in sql branch too",
			fieldName: "status; DELETE FROM t",
			source:    "SELECT * FROM t",
			queryType: "sql",
			limit:     20,
			wantErr:   "invalid field name",
		},
		{
			name:      "empty field name rejected",
			fieldName: "",
			source:    "orders",
			queryType: "table",
			limit:     20,
			wantErr:   "invalid field name",
		},
		{
			name:      "table branch invalid table name rejected",
			fieldName: "status",
			source:    "orders; DROP TABLE orders",
			queryType: "table",
			limit:     20,
			wantErr:   "invalid table name",
		},
		{
			name:      "sql branch allows arbitrary user SQL as source",
			fieldName: "status",
			source:    "SELECT status; DROP TABLE x",
			queryType: "sql",
			limit:     20,
			want:      "SELECT status, COUNT(*) as _count FROM (SELECT status; DROP TABLE x) as _subquery GROUP BY status ORDER BY _count DESC LIMIT 20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildFieldDistributionSQL(tt.fieldName, tt.source, tt.queryType, tt.limit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v (sql=%q)", tt.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildFieldDistributionSQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPreviewSQL(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		querySQL  string
		queryType string
		want      string
		wantErr   string
	}{
		{
			name:      "table branch valid",
			tableName: "public.orders",
			queryType: "table",
			want:      "SELECT * FROM public.orders LIMIT 10",
		},
		{
			name:      "sql branch wraps user SQL in subquery",
			tableName: "",
			querySQL:  "SELECT id, name FROM users WHERE active = 1; -- done",
			queryType: "sql",
			want:      "SELECT * FROM (SELECT id, name FROM users WHERE active = 1; -- done) AS _preview LIMIT 10",
		},
		{
			name:      "table branch invalid table name rejected",
			tableName: "orders; DROP TABLE orders",
			queryType: "table",
			wantErr:   "invalid table name",
		},
		{
			name:      "table branch empty table name rejected",
			tableName: "",
			queryType: "table",
			wantErr:   "invalid table name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPreviewSQL(tt.tableName, tt.querySQL, tt.queryType)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v (sql=%q)", tt.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildPreviewSQL() = %q, want %q", got, tt.want)
			}
		})
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
