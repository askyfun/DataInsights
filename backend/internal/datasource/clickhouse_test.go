package datasource

import (
	"context"
	"strings"
	"testing"
)

// TestClickHouseGetColumnsRejectsInvalidTableName 验证 GetColumns 在拼入
// system.columns 查询前先做标识符校验（与 postgresql.go GetColumns 同模式）。
// conn 为 nil：校验必须在任何查询执行前失败，否则会触发空指针。
func TestClickHouseGetColumnsRejectsInvalidTableName(t *testing.T) {
	c := &clickhouseConnection{}
	_, err := c.GetColumns(context.Background(), "orders'; DROP TABLE system.columns --")
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("expected invalid table name error, got %v", err)
	}
}
