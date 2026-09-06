package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPreviewResultMarshalJSONUsesEmptyArrays verifies datasource preview responses never
// serialize nil slice fields as JSON null, so frontend callers can always treat them as arrays.
func TestPreviewResultMarshalJSONUsesEmptyArrays(t *testing.T) {
	result := PreviewResult{}

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal preview result: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal preview result: %v", err)
	}

	if _, ok := decoded["columns"].([]any); !ok {
		t.Fatalf("expected columns to marshal as JSON array, got %T (%s)", decoded["columns"], payload)
	}
	if _, ok := decoded["data"].([]any); !ok {
		t.Fatalf("expected data to marshal as JSON array, got %T (%s)", decoded["data"], payload)
	}
	if len(decoded["columns"].([]any)) != 0 || len(decoded["data"].([]any)) != 0 {
		t.Fatalf("expected preview arrays to be empty, got %s", payload)
	}
}

// TestTableDataResultMarshalJSONUsesEmptyArrays verifies paginated table data responses keep
// array fields stable even when the backend currently holds nil slices.
func TestTableDataResultMarshalJSONUsesEmptyArrays(t *testing.T) {
	result := TableDataResult{}

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal table data result: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal table data result: %v", err)
	}

	if _, ok := decoded["columns"].([]any); !ok {
		t.Fatalf("expected columns to marshal as JSON array, got %T (%s)", decoded["columns"], payload)
	}
	if _, ok := decoded["data"].([]any); !ok {
		t.Fatalf("expected data to marshal as JSON array, got %T (%s)", decoded["data"], payload)
	}
	if _, ok := decoded["primary_keys"].([]any); !ok {
		t.Fatalf("expected primary_keys to marshal as JSON array, got %T (%s)", decoded["primary_keys"], payload)
	}
	if len(decoded["columns"].([]any)) != 0 || len(decoded["data"].([]any)) != 0 || len(decoded["primary_keys"].([]any)) != 0 {
		t.Fatalf("expected table data arrays to be empty, got %s", payload)
	}
}

// TestFieldDistributionMarshalJSONUsesEmptyArrays verifies distribution responses also keep
// empty collections as JSON arrays instead of null to satisfy the shared API contract.
func TestFieldDistributionMarshalJSONUsesEmptyArrays(t *testing.T) {
	result := FieldDistribution{}

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal field distribution: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal field distribution: %v", err)
	}

	if _, ok := decoded["distribution"].([]any); !ok {
		t.Fatalf("expected distribution to marshal as JSON array, got %T (%s)", decoded["distribution"], payload)
	}
	if len(decoded["distribution"].([]any)) != 0 {
		t.Fatalf("expected distribution to be empty, got %s", payload)
	}
}

// TestDatasourceMarshalJSONOmitsPassword verifies the stored datasource
// password (plaintext or encrypted) never leaves the backend through any
// serialized entity.Datasource.
func TestDatasourceMarshalJSONOmitsPassword(t *testing.T) {
	payload, err := json.Marshal(Datasource{Password: "x"})
	if err != nil {
		t.Fatalf("marshal datasource: %v", err)
	}
	if strings.Contains(string(payload), "password") {
		t.Fatalf("password leaked in json: %s", payload)
	}
}
