package diag

import (
	"strings"
	"testing"
)

func TestAddRedacts(t *testing.T) {
	Add(Entry{Message: "password=hunter2 di_pat_abc123 Authorization: Bearer abcdefghijklmnop SECURITY_KEY=topsecretvalue", ErrorID: "DI-50000-AAAA1111"})
	snap := Snapshot(1, 0, true)
	if len(snap) != 1 {
		t.Fatalf("want 1 entry, got %d", len(snap))
	}
	m := snap[0].Message
	for _, secret := range []string{"hunter2", "di_pat_abc123", "abcdefghijklmnop", "topsecretvalue"} {
		if strings.Contains(m, secret) {
			t.Fatalf("secret %q leaked in ring buffer message: %q", secret, m)
		}
	}
}
