package datasource

import "testing"

func TestNewDriverUnknownTypeReturnsError(t *testing.T) {
	d, err := NewDriver(DriverType("oracle"))
	if err == nil {
		t.Fatal("expected error for unknown driver type")
	}
	if d != nil {
		t.Fatalf("expected nil driver, got %v", d)
	}
}
