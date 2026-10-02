package extract

import "testing"

func TestInferColumns(t *testing.T) {
	table := &Table{
		Header: []string{"id", "price", "active", "day", "ts", "label", "empty"},
		Rows: [][]string{
			{"1", "9.5", "true", "2024-01-01", "2024-01-01 10:00:00", "a", ""},
			{"2", "8", "false", "2024/02/03", "2024-01-02 11:30:00", "b", ""},
			{"3", "10.25", "true", "2024-2-4", "2024-01-03T09:00:00Z", "c", ""},
		},
	}
	want := map[string]string{
		"id":     TypeInteger,
		"price":  TypeFloat,
		"active": TypeBoolean,
		"day":    TypeDate,
		"ts":     TypeDateTime,
		"label":  TypeString,
		"empty":  TypeString,
	}
	cols := InferColumns(table)
	for _, c := range cols {
		if want[c.Name] != c.Type {
			t.Errorf("column %q inferred as %q, want %q", c.Name, c.Type, want[c.Name])
		}
	}
}

func TestInferColumnsMixedFallsBackToString(t *testing.T) {
	table := &Table{
		Header: []string{"mixed"},
		Rows:   [][]string{{"1"}, {"abc"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeString {
		t.Fatalf("mixed column inferred as %q, want string", cols[0].Type)
	}
}

func TestInferColumnsDateAndDateTimeMixIsDateTime(t *testing.T) {
	table := &Table{
		Header: []string{"t"},
		Rows:   [][]string{{"2024-01-01"}, {"2024-01-02 10:00:00"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeDateTime {
		t.Fatalf("mixed date/datetime inferred as %q, want datetime", cols[0].Type)
	}
}

func TestInferColumnsZeroOneIsIntegerNotBoolean(t *testing.T) {
	table := &Table{
		Header: []string{"flag"},
		Rows:   [][]string{{"0"}, {"1"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeInteger {
		t.Fatalf("0/1 column inferred as %q, want integer", cols[0].Type)
	}
}

func TestInferColumnsSkipsBlankCells(t *testing.T) {
	table := &Table{
		Header: []string{"amount"},
		Rows:   [][]string{{"1"}, {""}, {"42"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeInteger {
		t.Fatalf("column with blank cells inferred as %q, want integer", cols[0].Type)
	}
}
