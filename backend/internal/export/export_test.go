package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/query"
)

func TestWriteCSV_BOMHeaderRows(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSV(&buf, &Table{
		Columns: []string{"渠道", "销售额"},
		Rows: [][]any{
			{"线上", 1234.5},
			{"线下", nil},
		},
	})
	if err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\ufeff") {
		t.Fatalf("missing UTF-8 BOM, first bytes: %q", got[:min(len(got), 8)])
	}
	want := "\ufeff渠道,销售额\n线上,1234.5\n线下,\n"
	if got != want {
		t.Errorf("csv body mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteCSV_QuotesCommasAndGuardsFormulas(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSV(&buf, &Table{
		Columns: []string{"a"},
		Rows:    [][]any{{"a,b"}, {"=1+1"}, {-1.5}},
	})
	if err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"a,b"`) {
		t.Errorf("comma value not quoted: %q", got)
	}
	if !strings.Contains(got, "'=1+1") {
		t.Errorf("formula injection not neutralized: %q", got)
	}
	if !strings.Contains(got, "\n-1.5\n") {
		t.Errorf("negative number must stay numeric (no apostrophe): %q", got)
	}
}

func TestWriteXLSX_IsReadableArchive(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteXLSX(&buf, &Table{
		Columns: []string{"维度", "值"},
		Rows:    [][]any{{"A&B", 2.5}, {"<tag>", 3}},
	}, "Data"); err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		files[f.Name] = string(body)
	}
	for _, required := range []string{
		"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml",
		"xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml",
	} {
		if _, ok := files[required]; !ok {
			t.Errorf("missing part %s", required)
		}
	}
	sheet := files["xl/worksheets/sheet1.xml"]
	if err := xml.Unmarshal([]byte(sheet), new(struct{})); err != nil {
		t.Errorf("sheet1.xml is not well-formed: %v", err)
	}
	if !strings.Contains(sheet, "A&amp;B") || !strings.Contains(sheet, "&lt;tag&gt;") {
		t.Errorf("cells not XML-escaped: %s", sheet)
	}
	// 数值单元格不能落在 inlineStr 里，否则 Excel 里是文本、没法直接求和。
	if !strings.Contains(sheet, `<c r="B2"><v>2.5</v></c>`) {
		t.Errorf("numeric cell not numeric: %s", sheet)
	}
}

func TestTabularize_TableKeepsDisplayColumns(t *testing.T) {
	req := &entity.ChartQueryRequest{ChartType: "table", Dims: []string{"渠道"}}
	data := &query.TableResponse{
		Columns: []string{"渠道", "销售额", "销售额(上期)", "销售额(增长率%)"},
		Data: []map[string]any{
			{"渠道": "线上", "销售额": 10.0, "销售额(上期)": 8.0, "销售额(增长率%)": 25.0},
		},
	}
	got := Tabularize(req, data, nil)
	if got == nil {
		t.Fatal("TableResponse must be tabularizable")
	}
	if strings.Join(got.Columns, ",") != strings.Join(data.Columns, ",") {
		t.Errorf("columns drifted: %v", got.Columns)
	}
	if len(got.Rows) != 1 || len(got.Rows[0]) != 4 {
		t.Fatalf("unexpected rows: %v", got.Rows)
	}
	if got.Rows[0][2] != 8.0 {
		t.Errorf("comparison column value lost: %v", got.Rows[0])
	}
}

func TestTabularize_AxisAndPivotV2(t *testing.T) {
	req := &entity.ChartQueryRequest{ChartType: "line", Dims: []string{"月份"}}
	axis := Tabularize(req, &query.AxisResponse{
		XAxis:  []string{"2026-01", "2026-02"},
		Series: []query.AxisSeries{{Name: "销售额", Data: []any{1.0, 2.0}}},
	}, nil)
	if axis == nil || axis.Columns[0] != "月份" || axis.Columns[1] != "销售额" {
		t.Fatalf("axis columns: %+v", axis)
	}
	if axis.Rows[1][1] != 2.0 {
		t.Errorf("axis row value: %v", axis.Rows)
	}

	pivot := Tabularize(nil, &query.PivotResponseV2{
		RowHeaders:  []string{"渠道"},
		ColHeaders:  []string{"Q1", "Q2"},
		MetricNames: []string{"销售额"},
		Cells:       []query.PivotRow{{RowKey: []string{"线上"}, Values: map[string]float64{"Q1|销售额": 3, "Q2|销售额": 4}}},
	}, nil)
	if pivot == nil {
		t.Fatal("PivotResponseV2 must be tabularizable")
	}
	if strings.Join(pivot.Columns, ",") != "渠道,Q1|销售额,Q2|销售额" {
		t.Errorf("pivot columns: %v", pivot.Columns)
	}
	if pivot.Rows[0][0] != "线上" || pivot.Rows[0][2] != 4.0 {
		t.Errorf("pivot row: %v", pivot.Rows[0])
	}
}

func TestTabularize_EmptyDataFallsBackToRequestColumns(t *testing.T) {
	req := &entity.ChartQueryRequest{
		ChartType: "table",
		Dims:      []string{"渠道"},
		Metrics:   []entity.MetricConfig{{Field: "amount", Alias: "销售额"}},
	}
	got := Tabularize(req, nil, map[string]string{"渠道": "渠道名"})
	if got == nil {
		t.Fatal("nil data should still export a header-only table")
	}
	if strings.Join(got.Columns, ",") != "渠道名,销售额" {
		t.Errorf("fallback columns: %v", got.Columns)
	}
	if len(got.Rows) != 0 {
		t.Errorf("fallback rows: %v", got.Rows)
	}
}

func TestTabularize_UnknownShapeReturnsNil(t *testing.T) {
	if got := Tabularize(nil, map[string]any{"x": 1}, nil); got != nil {
		t.Errorf("unknown shape should be rejected, got %+v", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"月度 销售/渠道:明细": "月度_销售渠道明细",
		`a"b\c`:       "abc",
		"   ":         "",
		"正常-名称_v2":    "正常-名称_v2",
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestColumnName(t *testing.T) {
	cases := []struct {
		index int
		want  string
	}{{0, "A"}, {25, "Z"}, {26, "AA"}, {51, "AZ"}, {52, "BA"}}
	for _, c := range cases {
		if got := columnName(c.index); got != c.want {
			t.Errorf("columnName(%d) = %q, want %q", c.index, got, c.want)
		}
	}
}

func TestTabularize_AxisResolvesColumnIDToName(t *testing.T) {
	req := &entity.ChartQueryRequest{ChartType: "bar", Dims: []string{"0000i52x"}}
	got := Tabularize(req, &query.AxisResponse{
		XAxis:  []string{"广东省"},
		Series: []query.AxisSeries{{Name: "total_sales", Data: []any{1.0}}},
	}, map[string]string{"0000i52x": "province"})
	if got == nil || got.Columns[0] != "province" {
		t.Fatalf("dimension id must be translated to the column name, got %+v", got)
	}
}
