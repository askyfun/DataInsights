package idgen

import (
	"strings"
	"testing"
)

func TestNewFormat(t *testing.T) {
	seen := make(map[string]struct{})
	for range 10000 {
		id := New()
		if len(id) != width {
			t.Fatalf("New() = %q, 长度 %d, 期望 %d", id, len(id), width)
		}
		if strings.ToLower(id) != id || strings.TrimSpace(id) != id {
			t.Fatalf("New() = %q, 含非法字符", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 10000 {
		t.Fatalf("10000 次生成只得到 %d 个不同 ID", len(seen))
	}
}

func TestNewConcurrentUnique(t *testing.T) {
	const n = 100000
	results := make(chan string, n)
	for range 10 {
		go func() {
			for range n / 10 {
				results <- New()
			}
		}()
	}
	seen := make(map[string]struct{}, n)
	for range n {
		id := <-results
		if _, dup := seen[id]; dup {
			t.Fatalf("并发下出现重复 ID %q", id)
		}
		seen[id] = struct{}{}
	}
}
