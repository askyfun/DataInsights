package extract

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestFileKind(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     Kind
		wantErr  bool
	}{
		{"csv lower", "data.csv", KindCSV, false},
		{"xlsx lower", "data.xlsx", KindXLSX, false},
		{"upper extension", "DATA.CSV", KindCSV, false},
		{"legacy xls rejected", "data.xls", "", true},
		{"txt rejected", "data.txt", "", true},
		{"no extension", "data", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileKind(tc.filename)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("FileKind(%q) = %q, want error", tc.filename, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FileKind(%q): %v", tc.filename, err)
			}
			if got != tc.want {
				t.Fatalf("FileKind(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	data := "\ufeffid,name,score\n1,alice,9.5\n2,bob,8\n"
	table, err := Parse("people.csv", []byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(table.Header, ","); got != "id,name,score" {
		t.Fatalf("header = %q (BOM not stripped?)", got)
	}
	if len(table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.Rows))
	}
	if table.Rows[0][1] != "alice" {
		t.Fatalf("rows[0][1] = %q, want alice", table.Rows[0][1])
	}
}

func TestParseCSVNormalizesHeaderAndRows(t *testing.T) {
	// Blank header cell, duplicate name, a ragged row and a trailing blank row.
	data := "id,,id\n1,2,3\n4,5\n,\n"
	table, err := Parse("x.csv", []byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"id", "column_2", "id_2"}
	for i, w := range want {
		if table.Header[i] != w {
			t.Fatalf("header[%d] = %q, want %q", i, table.Header[i], w)
		}
	}
	// The trailing all-blank row must be dropped; the ragged row padded to 3.
	if len(table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (blank row dropped)", len(table.Rows))
	}
	if len(table.Rows[1]) != 3 || table.Rows[1][2] != "" {
		t.Fatalf("ragged row not padded to header width: %#v", table.Rows[1])
	}
}

func TestParseCSVEmpty(t *testing.T) {
	if _, err := Parse("empty.csv", []byte("")); err == nil {
		t.Fatal("empty csv must be rejected (no header row)")
	}
}

func TestColumnIndex(t *testing.T) {
	tests := map[string]int{"A1": 0, "B2": 1, "Z9": 25, "AA1": 26, "AB3": 27, "a1": 0}
	for ref, want := range tests {
		if got := columnIndex(ref); got != want {
			t.Fatalf("columnIndex(%q) = %d, want %d", ref, got, want)
		}
	}
}

func TestParseXLSX(t *testing.T) {
	shared := []string{"id", "name", "score"}
	sheet := `<worksheet><sheetData>` +
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>` +
		`<row r="2"><c r="A2"><v>1</v></c><c r="B2" t="inlineStr"><is><t>alice</t></is></c><c r="C2"><v>9.5</v></c></row>` +
		`<row r="3"><c r="A3"><v>2</v></c><c r="C3"><v>8</v></c></row>` + // B3 omitted -> blank cell
		`</sheetData></worksheet>`

	table, err := Parse("people.xlsx", buildXLSX(t, sheet, shared, ""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(table.Header, ","); got != "id,name,score" {
		t.Fatalf("header = %q", got)
	}
	if len(table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.Rows))
	}
	if table.Rows[0][1] != "alice" {
		t.Fatalf("inline string cell = %q, want alice", table.Rows[0][1])
	}
	if table.Rows[1][1] != "" {
		t.Fatalf("omitted cell should be blank, got %q", table.Rows[1][1])
	}
}

func TestParseXLSXDateCell(t *testing.T) {
	styles := `<styleSheet><cellXfs count="2"><xf numFmtId="0"/><xf numFmtId="14"/></cellXfs></styleSheet>`
	sheet := `<worksheet><sheetData>` +
		`<row r="1"><c r="A1" t="inlineStr"><is><t>day</t></is></c></row>` +
		`<row r="2"><c r="A2" s="1"><v>45292</v></c></row>` +
		`</sheetData></worksheet>`

	table, err := Parse("dates.xlsx", buildXLSX(t, sheet, nil, styles))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := table.Rows[0][0]; got != "2024-01-01" {
		t.Fatalf("date-styled serial rendered as %q, want 2024-01-01", got)
	}
}

func TestParseXLSXDateTimeCell(t *testing.T) {
	styles := `<styleSheet><cellXfs count="2"><xf numFmtId="0"/><xf numFmtId="22"/></cellXfs></styleSheet>`
	sheet := `<worksheet><sheetData>` +
		`<row r="1"><c r="A1" t="inlineStr"><is><t>ts</t></is></c></row>` +
		`<row r="2"><c r="A2" s="1"><v>45292.5</v></c></row>` +
		`</sheetData></worksheet>`

	table, err := Parse("datetimes.xlsx", buildXLSX(t, sheet, nil, styles))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := table.Rows[0][0]; got != "2024-01-01 12:00:00" {
		t.Fatalf("datetime serial rendered as %q, want 2024-01-01 12:00:00", got)
	}
}

func TestParseXLSXMissingWorksheet(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("docProps/app.xml")
	_, _ = w.Write([]byte("<Properties/>"))
	_ = zw.Close()
	if _, err := Parse("nope.xlsx", buf.Bytes()); err == nil {
		t.Fatal("xlsx without a worksheet must fail")
	}
}

// buildXLSX writes a minimal workbook archive. shared may be nil (inline-only
// workbooks omit sharedStrings.xml) and styles may be empty.
func buildXLSX(t *testing.T, sheetXML string, shared []string, stylesXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if shared != nil {
		var sb strings.Builder
		sb.WriteString(`<sst>`)
		for _, s := range shared {
			sb.WriteString(`<si><t>` + s + `</t></si>`)
		}
		sb.WriteString(`</sst>`)
		add("xl/sharedStrings.xml", sb.String())
	}
	if stylesXML != "" {
		add("xl/styles.xml", stylesXML)
	}
	add("xl/worksheets/sheet1.xml", sheetXML)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
