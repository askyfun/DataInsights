package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestShareMarshalJSONOmitsPassword verifies the share password (plaintext or
// bcrypt hash) never leaves the backend through any serialized entity.Share.
func TestShareMarshalJSONOmitsPassword(t *testing.T) {
	pw := "x"
	payload, err := json.Marshal(Share{Password: &pw})
	if err != nil {
		t.Fatalf("marshal share: %v", err)
	}
	if strings.Contains(string(payload), "password") {
		t.Fatalf("password leaked in json: %s", payload)
	}
}
