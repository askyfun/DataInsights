package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
)

// WriteXLSX 写一个最小但合法的 .xlsx（OOXML）：单工作表、表头与数据行，
// 字符串用 inlineStr、数值用数值单元格。刻意不引第三方库——只需要一个
// 能双击打开的表格，样式/公式/多 sheet 都不在本期范围内。
//
// 落盘顺序：zip 条目逐个写进 w，不先在内存里拼完整文件（与 CSV 同理）。
func WriteXLSX(w io.Writer, t *Table, sheetName string) error {
	if sheetName == "" {
		sheetName = "Data"
	}
	zw := zip.NewWriter(w)

	parts := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", rootRelsXML},
		{"xl/workbook.xml", fmt.Sprintf(workbookXML, escapeXML(sheetName))},
		{"xl/_rels/workbook.xml.rels", workbookRelsXML},
		{"xl/worksheets/sheet1.xml", sheetXML(t)},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	return zw.Close()
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="%s" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`

func sheetXML(t *Table) string {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	header := make([]any, len(t.Columns))
	for i, col := range t.Columns {
		header[i] = col
	}
	writeRow(&b, 1, header)
	for i, row := range t.Rows {
		cells := make([]any, len(t.Columns))
		copy(cells, row)
		writeRow(&b, i+2, cells)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func writeRow(b *bytes.Buffer, rowNum int, cells []any) {
	fmt.Fprintf(b, `<row r="%d">`, rowNum)
	for i, v := range cells {
		ref := columnName(i) + strconv.Itoa(rowNum)
		switch n := v.(type) {
		case float64:
			fmt.Fprintf(b, `<c r="%s"><v>%s</v></c>`, ref, strconv.FormatFloat(n, 'f', -1, 64))
		case float32:
			fmt.Fprintf(b, `<c r="%s"><v>%s</v></c>`, ref, strconv.FormatFloat(float64(n), 'f', -1, 64))
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			fmt.Fprintf(b, `<c r="%s"><v>%v</v></c>`, ref, n)
		case bool:
			v := "0"
			if n {
				v = "1"
			}
			fmt.Fprintf(b, `<c r="%s" t="b"><v>%s</v></c>`, ref, v)
		default:
			fmt.Fprintf(b, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
				ref, escapeXML(CellText(v)))
		}
	}
	b.WriteString(`</row>`)
}

// columnName 0 -> A, 25 -> Z, 26 -> AA。
func columnName(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
