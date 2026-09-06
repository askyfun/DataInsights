package datasource

import "testing"

func TestSafeIdentifierRejectsInjection(t *testing.T) {
	cases := map[string]bool{
		"region":       true,
		"bi_orders.id": true,
		"a;b":          false,
		"a'b":          false,
		"a\"b":         false,
		"a-b":          false,
		"\"\")(; DROP": false,
		"":             false,
		"scoreavg":     true,
	}
	for in, want := range cases {
		if got := IsValidIdentifier(in); got != want {
			t.Errorf("IsValidIdentifier(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewDriverUnknownTypeReturnsError(t *testing.T) {
	d, err := NewDriver(DriverType("oracle"))
	if err == nil {
		t.Fatal("expected error for unknown driver type")
	}
	if d != nil {
		t.Fatalf("expected nil driver, got %v", d)
	}
}
