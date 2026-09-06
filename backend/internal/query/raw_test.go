package query

import (
	"strings"
	"testing"
)

func TestBuildFieldDistributionSQL(t *testing.T) {
	sql, err := BuildFieldDistributionSQL("region", "orders", SourceTypeTable, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT region, COUNT(*) as _count FROM orders GROUP BY region ORDER BY _count DESC LIMIT 20"
	if sql != want {
		t.Fatalf("got %s want %s", sql, want)
	}
}

func TestBuildFieldDistributionSQLSchemaQualifiedTable(t *testing.T) {
	sql, err := BuildFieldDistributionSQL("status", "public.orders", SourceTypeTable, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT status, COUNT(*) as _count FROM public.orders GROUP BY status ORDER BY _count DESC LIMIT 20"
	if sql != want {
		t.Fatalf("got %s want %s", sql, want)
	}
}

func TestBuildFieldDistributionSQLSubquery(t *testing.T) {
	sql, err := BuildFieldDistributionSQL("region", "SELECT * FROM t", SourceTypeSQL, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sql, "SELECT region, COUNT(*) as _count FROM (SELECT * FROM t) as _subquery") {
		t.Fatalf("unexpected: %s", sql)
	}
}

func TestBuildFieldDistributionSQLSubqueryAllowsUserSQL(t *testing.T) {
	// sql 分支的 source 是产品功能允许的任意用户 SQL，不做标识符校验。
	sql, err := BuildFieldDistributionSQL("status", "SELECT status; DROP TABLE x", SourceTypeSQL, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sql, "SELECT status, COUNT(*) as _count FROM (SELECT status; DROP TABLE x) as _subquery") {
		t.Fatalf("unexpected: %s", sql)
	}
}

func TestBuildFieldDistributionSQLInvalidField(t *testing.T) {
	_, err := BuildFieldDistributionSQL("a;b", "orders", SourceTypeTable, 20)
	if err == nil {
		t.Fatal("expected error for invalid identifier")
	}
}

func TestBuildFieldDistributionSQLInvalidFieldSQLBranch(t *testing.T) {
	// sql 分支同样必须校验 field 名。
	if _, err := BuildFieldDistributionSQL("status; DELETE FROM t", "SELECT * FROM t", SourceTypeSQL, 20); err == nil {
		t.Fatal("expected error for invalid field name in sql branch")
	}
}

func TestBuildFieldDistributionSQLInvalidTable(t *testing.T) {
	_, err := BuildFieldDistributionSQL("status", "orders; DROP TABLE orders", SourceTypeTable, 20)
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("expected invalid table name error, got %v", err)
	}
}

func TestWrapPreviewSQL(t *testing.T) {
	if got := WrapPreviewSQL("SELECT * FROM t", SourceTypeSQL, 10); got != "SELECT * FROM (SELECT * FROM t) AS _preview LIMIT 10" {
		t.Fatalf("got %s", got)
	}
	if got := WrapPreviewSQL("orders", SourceTypeTable, 10); got != "SELECT * FROM orders LIMIT 10" {
		t.Fatalf("got %s", got)
	}
}

func TestWrapPreviewSQLNoLimit(t *testing.T) {
	if got := WrapPreviewSQL("SELECT * FROM t", SourceTypeSQL, 0); got != "SELECT * FROM (SELECT * FROM t) AS _preview" {
		t.Fatalf("got %s", got)
	}
}

func TestWrapPreviewSQLInvalidTableFallsBack(t *testing.T) {
	// 无错误返回值：非法表名退化为安全占位标识符，由数据库拒绝执行。
	if got := WrapPreviewSQL("orders; DROP TABLE orders", SourceTypeTable, 10); got != "SELECT * FROM _invalid_identifier LIMIT 10" {
		t.Fatalf("got %s", got)
	}
}

func TestWrapCountSQL(t *testing.T) {
	if got := WrapCountSQL("SELECT * FROM t", SourceTypeSQL); got != "SELECT COUNT(*) as _total FROM (SELECT * FROM t) as _subquery" {
		t.Fatalf("got %s", got)
	}
	if got := WrapCountSQL("public.orders", SourceTypeTable); got != "SELECT COUNT(*) as _total FROM public.orders" {
		t.Fatalf("got %s", got)
	}
}

func TestBuildTableDataSQL(t *testing.T) {
	sql, err := BuildTableDataSQL("public.orders", "id", "desc", 20, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM public.orders ORDER BY id DESC LIMIT 20 OFFSET 40"
	if sql != want {
		t.Fatalf("got %s want %s", sql, want)
	}
}

func TestBuildTableDataSQLNoOrder(t *testing.T) {
	sql, err := BuildTableDataSQL("orders", "", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sql != "SELECT * FROM orders LIMIT 20 OFFSET 0" {
		t.Fatalf("got %s", sql)
	}
}

func TestBuildTableDataSQLNormalizesOrder(t *testing.T) {
	sql, err := BuildTableDataSQL("orders", "id", "Desc", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sql != "SELECT * FROM orders ORDER BY id DESC LIMIT 20 OFFSET 0" {
		t.Fatalf("got %s", sql)
	}
}

func TestBuildTableDataSQLInvalidTable(t *testing.T) {
	_, err := BuildTableDataSQL("orders; DROP TABLE orders", "id", "ASC", 20, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid table name") {
		t.Fatalf("expected invalid table name error, got %v", err)
	}
}

func TestBuildTableDataSQLInvalidOrderField(t *testing.T) {
	_, err := BuildTableDataSQL("orders", "id; DROP TABLE orders", "ASC", 20, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid sort field") {
		t.Fatalf("expected invalid sort field error, got %v", err)
	}
}

func TestNormalizeSortOrder(t *testing.T) {
	cases := map[string]string{
		"":        "ASC",
		"ASC":     "ASC",
		"asc":     "ASC",
		"DESC":    "DESC",
		"desc":    "DESC",
		"Desc":    "DESC",
		" desc ":  "DESC",
		"INVALID": "ASC",
		"123":     "ASC",
		"DES":     "ASC",
		"DESCC":   "ASC",
	}
	for in, want := range cases {
		if got := normalizeSortOrder(in); got != want {
			t.Errorf("normalizeSortOrder(%q) = %q, want %q", in, got, want)
		}
	}
}
