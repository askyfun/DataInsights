package extract

import (
	"strings"
	"testing"
)

func TestParseDialect(t *testing.T) {
	for _, ok := range []string{"starrocks", "MySQL", "postgresql", " STARROCKS "} {
		if _, err := ParseDialect(ok); err != nil {
			t.Errorf("ParseDialect(%q): %v", ok, err)
		}
	}
	if _, err := ParseDialect("clickhouse"); err == nil {
		t.Fatal("clickhouse must be rejected as an extract dialect")
	}
}

func TestAssignPhysical(t *testing.T) {
	cols := []Column{
		{Name: "订单 编号", Type: TypeInteger},
		{Name: "2024 Sales", Type: TypeFloat},
		{Name: "", Type: TypeString},
		{Name: "2024 Sales", Type: TypeString},
	}
	got := AssignPhysical(cols)
	want := []string{"col", "c_2024_sales", "col_2", "c_2024_sales_2"}
	for i := range got {
		if got[i].Physical != want[i] {
			t.Errorf("column %d physical = %q, want %q", i, got[i].Physical, want[i])
		}
	}
}

func TestCreateTableSQLStarRocksPromotesKeyableColumn(t *testing.T) {
	cols := AssignPhysical([]Column{
		{Name: "price", Type: TypeFloat},
		{Name: "id", Type: TypeInteger},
	})
	sql, err := CreateTableSQL(DialectStarRocks, "di_extract_1", cols)
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	if !strings.Contains(sql, "DUPLICATE KEY(`id`)") {
		t.Fatalf("key column not promoted: %s", sql)
	}
	if !strings.HasPrefix(sql, "CREATE TABLE `di_extract_1` (`id` BIGINT, `price` DOUBLE)") {
		t.Fatalf("unexpected column order: %s", sql)
	}
}

func TestCreateTableSQLStarRocksSyntheticKeyForAllFloat(t *testing.T) {
	cols := AssignPhysical([]Column{
		{Name: "a", Type: TypeFloat},
		{Name: "b", Type: TypeFloat},
	})
	sql, err := CreateTableSQL(DialectStarRocks, "di_extract_2", cols)
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	if !strings.Contains(sql, "(`_di_row` BIGINT, `a` DOUBLE, `b` DOUBLE)") {
		t.Fatalf("synthetic key column missing: %s", sql)
	}
	if !strings.Contains(sql, "DUPLICATE KEY(`_di_row`)") {
		t.Fatalf("synthetic key not used: %s", sql)
	}
}

func TestCreateTableSQLPostgreSQLNoKey(t *testing.T) {
	cols := AssignPhysical([]Column{
		{Name: "a", Type: TypeInteger},
		{Name: "b", Type: TypeFloat},
		{Name: "c", Type: TypeDateTime},
	})
	sql, err := CreateTableSQL(DialectPostgreSQL, "di_extract_3", cols)
	if err != nil {
		t.Fatalf("CreateTableSQL: %v", err)
	}
	want := `CREATE TABLE "di_extract_3" ("a" BIGINT, "b" DOUBLE PRECISION, "c" TIMESTAMP)`
	if sql != want {
		t.Fatalf("sql = %s, want %s", sql, want)
	}
}

func TestInsertSQLPlaceholders(t *testing.T) {
	cols := AssignPhysical([]Column{
		{Name: "id", Type: TypeInteger},
		{Name: "name", Type: TypeString},
	})
	sr, err := InsertSQL(DialectStarRocks, "di_extract_1", cols, 2)
	if err != nil {
		t.Fatalf("InsertSQL: %v", err)
	}
	if sr != "INSERT INTO `di_extract_1` (`id`, `name`) VALUES (?, ?), (?, ?)" {
		t.Fatalf("starrocks insert = %s", sr)
	}

	pg, err := InsertSQL(DialectPostgreSQL, "di_extract_1", cols, 2)
	if err != nil {
		t.Fatalf("InsertSQL: %v", err)
	}
	if pg != `INSERT INTO "di_extract_1" ("id", "name") VALUES ($1, $2), ($3, $4)` {
		t.Fatalf("postgres insert = %s", pg)
	}
}

func TestInsertSQLZeroRowsIsError(t *testing.T) {
	cols := AssignPhysical([]Column{{Name: "id", Type: TypeInteger}})
	if _, err := InsertSQL(DialectStarRocks, "t", cols, 0); err == nil {
		t.Fatal("zero-row insert must error")
	}
}

func TestRowArgsSyntheticKeyAndFloatConversion(t *testing.T) {
	// All-float columns cannot back a StarRocks key, so a synthetic row-index
	// key is prepended and must lead the arg tuple.
	cols := AssignPhysical([]Column{
		{Name: "a", Type: TypeFloat},
		{Name: "b", Type: TypeFloat},
	})
	rows := [][]string{{"1.5", "2.5"}, {"", ""}}

	args, err := RowArgs(DialectStarRocks, cols, rows)
	if err != nil {
		t.Fatalf("RowArgs: %v", err)
	}
	if args[0][0] != int64(1) || args[1][0] != int64(2) {
		t.Fatalf("synthetic key not filled: %#v", args)
	}
	if args[0][1] != 1.5 || args[0][2] != 2.5 {
		t.Fatalf("float conversion: %#v", args[0])
	}
	if args[1][1] != nil || args[1][2] != nil {
		t.Fatalf("blank float cells must be NULL: %#v", args[1])
	}
}

func TestRowArgsConvertsBooleanAndString(t *testing.T) {
	cols := AssignPhysical([]Column{
		{Name: "id", Type: TypeInteger},
		{Name: "flag", Type: TypeBoolean},
		{Name: "note", Type: TypeString},
	})
	rows := [][]string{{"1", "true", "hi"}, {"2", "false", ""}}

	args, err := RowArgs(DialectPostgreSQL, cols, rows)
	if err != nil {
		t.Fatalf("RowArgs: %v", err)
	}
	if args[0][0] != int64(1) || args[0][1] != true || args[0][2] != "hi" {
		t.Fatalf("row 0 conversion: %#v", args[0])
	}
	if args[1][0] != int64(2) || args[1][1] != false || args[1][2] != "" {
		t.Fatalf("row 1 conversion: %#v", args[1])
	}
}

func TestRowArgsRejectsUnparseableValue(t *testing.T) {
	cols := AssignPhysical([]Column{{Name: "n", Type: TypeInteger}})
	if _, err := RowArgs(DialectPostgreSQL, cols, [][]string{{"abc"}}); err == nil {
		t.Fatal("non-integer value in an integer column must error")
	}
}
