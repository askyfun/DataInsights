package export

import (
	"encoding/csv"
	"io"
	"strings"
)

// utf8BOM 让 Excel 在双击打开 CSV 时按 UTF-8 解码（否则中文列名在中文 Windows
// 的 Excel 里是乱码）。这是 issue #120 验收里点名的行为，单测逐字节断言。
const utf8BOM = "\ufeff"

// WriteCSV 把表写成 CSV（RFC 4180，UTF-8 + BOM）。直接写 io.Writer，不先拼整份
// 字符串——行数上限（默认 1 万）虽在 handler 侧裁剪，但这里也不制造第二份全量副本。
func WriteCSV(w io.Writer, t *Table) error {
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(t.Columns); err != nil {
		return err
	}
	record := make([]string, len(t.Columns))
	for _, row := range t.Rows {
		for i := range record {
			if i < len(row) {
				record[i] = csvCell(row[i])
			} else {
				record[i] = ""
			}
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvCell 文本化 + 公式注入防护。
// CSV 里的 `=...`、`+...`、`-...`、`@...` 在 Excel/LibreOffice 中会被当公式执行
// （OWASP CSV Injection）。前缀单引号让 Excel 按文本处理；只在真正的字符串上做，
// 数字（-1.5）不能被改写成 "'-1.5"。
func csvCell(v any) string {
	s := CellText(v)
	if _, isString := v.(string); !isString {
		return s
	}
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// SanitizeFilename 把图表名变成可安全放进 Content-Disposition 的文件名片段：
// 去掉路径分隔符、控制字符与引号，空白折成下划线。空结果由调用方兜底。
func SanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == '"' || r == ':' || r == '*' || r == '?' ||
			r == '<' || r == '>' || r == '|' || r < 0x20 || r == 0x7f:
			continue
		case r == ' ' || r == '\t' || r == '\n':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._")
}
