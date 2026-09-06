package entity

import (
	"encoding/json"
	"testing"
)

// TestShareMarshalJSONOmitsPassword verifies the share password (plaintext or
// bcrypt hash) never leaves the backend through any serialized entity.Share.
// The boolean has_password flag is the only password-related field exposed.
func TestShareMarshalJSONOmitsPassword(t *testing.T) {
	pw := "x"
	payload, err := json.Marshal(Share{Password: &pw, HasPassword: true})
	if err != nil {
		t.Fatalf("marshal share: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal share: %v", err)
	}
	if _, ok := decoded["password"]; ok {
		t.Fatalf("password leaked in json: %s", payload)
	}
	if decoded["has_password"] != true {
		t.Fatalf("expected has_password=true for protected share, got %s", payload)
	}
}

// TestShareMarshalJSONHasPasswordFalse verifies passwordless shares expose
// has_password=false so the frontend can render the public path.
func TestShareMarshalJSONHasPasswordFalse(t *testing.T) {
	payload, err := json.Marshal(Share{})
	if err != nil {
		t.Fatalf("marshal share: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal share: %v", err)
	}
	if _, ok := decoded["password"]; ok {
		t.Fatalf("password leaked in json: %s", payload)
	}
	if decoded["has_password"] != false {
		t.Fatalf("expected has_password=false for passwordless share, got %s", payload)
	}
}
