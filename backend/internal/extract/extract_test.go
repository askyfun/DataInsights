package extract

import (
	"errors"
	"testing"
)

func TestTableName(t *testing.T) {
	if got := TableName(42); got != "di_extract_42" {
		t.Fatalf("TableName(42) = %q, want di_extract_42", got)
	}
}

func TestGuardCheckDatasource(t *testing.T) {
	tests := []struct {
		name       string
		guard      Guard
		datasource int
		wantErr    bool
	}{
		{"disabled guard never fires", Guard{}, 0, false},
		{"disabled guard passes the would-be id", Guard{DatasourceID: 0}, 7, false},
		{"enabled guard rejects the extract id", Guard{DatasourceID: 7}, 7, true},
		{"enabled guard passes other ids", Guard{DatasourceID: 7}, 8, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.guard.CheckDatasource(tc.datasource)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CheckDatasource(%d) = nil, want error", tc.datasource)
				}
				if !errors.Is(err, ErrExtractDatasource) {
					t.Fatalf("error %v must wrap ErrExtractDatasource", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckDatasource(%d) = %v, want nil", tc.datasource, err)
			}
		})
	}
}

func TestGuardEnabled(t *testing.T) {
	if (Guard{}).Enabled() || (Guard{DatasourceID: -1}).Enabled() {
		t.Fatal("Enabled must be false for 0/negative ids")
	}
	if !(Guard{DatasourceID: 1}).Enabled() {
		t.Fatal("Enabled must be true for positive ids")
	}
}
