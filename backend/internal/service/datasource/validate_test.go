package datasource

import (
	"testing"
)

// isValidSQLIdentifier 与 normalizeSortOrder 已下沉至 query 包
// （raw.go 的 BuildTableDataSQL / normalizeSortOrder）与 datasource.IsValidIdentifier，
// 对应测试随实现迁移，此处仅保留仍在 service 层使用的 helper 测试。

func TestContainsString(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		val   string
		want  bool
	}{
		{"found in slice", []string{"id", "name"}, "id", true},
		{"not found", []string{"id", "name"}, "email", false},
		{"empty slice", []string{}, "id", false},
		{"nil slice", nil, "id", false},
		{"empty val found", []string{""}, "", true},
		{"case sensitive", []string{"ID"}, "id", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsString(tt.slice, tt.val)
			if got != tt.want {
				t.Errorf("containsString(%v, %q) = %v, want %v", tt.slice, tt.val, got, tt.want)
			}
		})
	}
}
