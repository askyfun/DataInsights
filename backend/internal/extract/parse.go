package extract

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxUploadBytes caps a single upload. Issue #138 sets the bar at 50MB;
// the limit lives here next to the parser so both the handler and the tests
// share one number.
const MaxUploadBytes = 50 << 20

// Kind identifies the accepted upload formats.
type Kind string

const (
	KindCSV  Kind = "csv"
	KindXLSX Kind = "xlsx"
)

// FileKind derives the format from the extension. Only .csv and .xlsx are
// accepted; anything else (including .xls, the legacy binary format this
// parser cannot read) is rejected with a message naming the offender.
func FileKind(filename string) (Kind, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return KindCSV, nil
	case ".xlsx":
		return KindXLSX, nil
	default:
		if ext == "" {
			return "", errors.New("file has no extension: expected .csv or .xlsx")
		}
		return "", fmt.Errorf("unsupported file type %q: only .csv and .xlsx are accepted", ext)
	}
}

// Table is the parsed content of an upload. Every cell is rendered as text
// (the wire form), so XLSX and CSV converge before type inference runs.
type Table struct {
	Header []string
	Rows   [][]string
}

// Parse decodes an upload into a Table. The header row is normalized (blank
// names filled, duplicates suffixed) and every data row is padded/truncated
// to the header width, so downstream code never sees ragged rows.
func Parse(filename string, data []byte) (*Table, error) {
	kind, err := FileKind(filename)
	if err != nil {
		return nil, err
	}

	var header []string
	var rows [][]string
	switch kind {
	case KindCSV:
		header, rows, err = parseCSV(data)
	case KindXLSX:
		header, rows, err = parseXLSX(data)
	}
	if err != nil {
		return nil, err
	}
	if len(header) == 0 {
		return nil, errors.New("file has no header row")
	}

	header = normalizeHeader(header)
	rows = normalizeRows(rows, len(header))
	return &Table{Header: header, Rows: rows}, nil
}

// normalizeHeader fills empty header cells and de-duplicates repeated names.
// The dataset contract rejects duplicate column names (they become SQL output
// aliases), so collisions are resolved here rather than surfacing as a 400 on
// the user's first upload.
func normalizeHeader(header []string) []string {
	used := make(map[string]int, len(header))
	out := make([]string, len(header))
	for i, h := range header {
		name := strings.TrimSpace(h)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		if n, ok := used[name]; ok {
			used[name] = n + 1
			name = fmt.Sprintf("%s_%d", name, n+1)
		} else {
			used[name] = 1
		}
		out[i] = name
	}
	return out
}

// normalizeRows pads or truncates each row to width and drops rows that are
// entirely blank (trailing blank lines are common in hand-edited CSV).
func normalizeRows(rows [][]string, width int) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		blank := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		fixed := make([]string, width)
		copy(fixed, row)
		out = append(out, fixed)
	}
	return out
}

func parseCSV(data []byte) ([]string, [][]string, error) {
	// A UTF-8 BOM would otherwise become part of the first header cell.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // ragged rows are padded later, not rejected here
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, nil
	}
	return records[0], records[1:], nil
}

// --- XLSX ---

// excelEpoch is the day 0 of Excel's serial date system (1900 system, with
// the well-known leap-year bug already folded into the epoch).
var excelEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)

func parseXLSX(data []byte) ([]string, [][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("parse xlsx: %w", err)
	}

	shared, err := readSharedStrings(zr)
	if err != nil {
		return nil, nil, err
	}
	dateStyles, err := readDateStyles(zr)
	if err != nil {
		return nil, nil, err
	}

	sheetPath, err := firstSheetPath(zr)
	if err != nil {
		return nil, nil, err
	}
	sheetData, err := readZipFile(zr, sheetPath)
	if err != nil {
		return nil, nil, err
	}

	var sheet xlsxSheet
	if err := xml.Unmarshal(sheetData, &sheet); err != nil {
		return nil, nil, fmt.Errorf("parse xlsx sheet: %w", err)
	}

	header, rows := sheetToTable(&sheet, shared, dateStyles)
	return header, rows, nil
}

// readZipFile returns the contents of one entry; missing entries are an error
// so a truncated workbook surfaces clearly instead of parsing as empty.
func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open xlsx part %s: %w", name, err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("xlsx part %s not found", name)
}

// firstSheetPath picks the first worksheet part. Worksheet parts are named
// xl/worksheets/sheetN.xml; sorting the names gives a deterministic "first
// sheet" without parsing workbook relationships.
func firstSheetPath(zr *zip.Reader) (string, error) {
	var names []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/") && strings.HasSuffix(f.Name, ".xml") {
			names = append(names, f.Name)
		}
	}
	if len(names) == 0 {
		return "", errors.New("xlsx has no worksheet")
	}
	sort.Strings(names)
	return names[0], nil
}

type xlsxSharedStrings struct {
	SI []struct {
		T struct {
			Value string `xml:",chardata"`
		} `xml:"t"`
		R []struct {
			T struct {
				Value string `xml:",chardata"`
			} `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	data, err := readZipFile(zr, "xl/sharedStrings.xml")
	if err != nil {
		// A workbook with only inline strings may omit the part entirely.
		return nil, nil
	}
	var sst xlsxSharedStrings
	if err := xml.Unmarshal(data, &sst); err != nil {
		return nil, fmt.Errorf("parse xlsx shared strings: %w", err)
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		out[i] = si.T.Value
		for _, run := range si.R {
			out[i] += run.T.Value
		}
	}
	return out, nil
}

type xlsxStyles struct {
	NumFmts struct {
		Fmt []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmt"`
	} `xml:"numFmts"`
	CellXfs struct {
		Xf []struct {
			NumFmtID int `xml:"numFmtId,attr"`
		} `xml:"xf"`
	} `xml:"cellXfs"`
}

// readDateStyles returns the set of cellXfs indices whose number format is a
// date/time format, so numeric cells carrying those styles can be rendered as
// dates instead of raw serial numbers.
func readDateStyles(zr *zip.Reader) (map[int]bool, error) {
	data, err := readZipFile(zr, "xl/styles.xml")
	if err != nil {
		return map[int]bool{}, nil
	}
	var styles xlsxStyles
	if err := xml.Unmarshal(data, &styles); err != nil {
		return nil, fmt.Errorf("parse xlsx styles: %w", err)
	}

	custom := make(map[int]bool, len(styles.NumFmts.Fmt))
	for _, f := range styles.NumFmts.Fmt {
		if isDateFormatCode(f.Code) {
			custom[f.ID] = true
		}
	}

	out := make(map[int]bool)
	for i, xf := range styles.CellXfs.Xf {
		if builtinDateFormatIDs[xf.NumFmtID] || custom[xf.NumFmtID] {
			out[i] = true
		}
	}
	return out, nil
}

// builtinDateFormatIDs are the number-format ids Excel reserves for dates and
// times (ECMA-376 §18.8.30).
var builtinDateFormatIDs = map[int]bool{
	14: true, 15: true, 16: true, 17: true, 18: true, 19: true, 20: true, 21: true, 22: true,
	27: true, 28: true, 29: true, 30: true, 31: true, 32: true, 33: true, 34: true, 35: true, 36: true,
	45: true, 46: true, 47: true,
	50: true, 51: true, 52: true, 53: true, 54: true, 55: true, 56: true, 57: true, 58: true,
}

// isDateFormatCode reports whether a custom format code looks like a date.
// Literal sections (quoted text, escapes) are stripped first so a code such as
// "0.00\"days\"" is not mistaken for a date.
func isDateFormatCode(code string) bool {
	cleaned := stripFormatLiterals(code)
	cleaned = strings.ToLower(cleaned)
	return strings.ContainsAny(cleaned, "yd") || strings.Contains(cleaned, "h") && strings.Contains(cleaned, "m")
}

func stripFormatLiterals(code string) string {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
			// skip
		case c == '\\' && i+1 < len(code):
			i++
		case c == '[':
			for i < len(code) && code[i] != ']' {
				i++
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

type xlsxSheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxRow struct {
	R     string     `xml:"r,attr"`
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	R  string `xml:"r,attr"`
	T  string `xml:"t,attr"`
	S  string `xml:"s,attr"`
	V  string `xml:"v"`
	IS struct {
		T struct {
			Value string `xml:",chardata"`
		} `xml:"t"`
		R []struct {
			T struct {
				Value string `xml:",chardata"`
			} `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

// sheetToTable lays cells onto a dense grid. Excel omits empty cells and can
// place cells out of order, so each cell is positioned by its reference.
func sheetToTable(sheet *xlsxSheet, shared []string, dateStyles map[int]bool) ([]string, [][]string) {
	grid := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		// A missing r attribute means "next row"; use the write position.
		rowIdx := parseRowIndex(row.R)
		if rowIdx < 0 {
			rowIdx = len(grid)
		}
		for len(grid) <= rowIdx {
			grid = append(grid, nil)
		}
		for _, cell := range row.Cells {
			col := columnIndex(cell.R)
			if col < 0 {
				continue
			}
			for len(grid[rowIdx]) <= col {
				grid[rowIdx] = append(grid[rowIdx], "")
			}
			grid[rowIdx][col] = cellText(cell, shared, dateStyles)
		}
	}

	// Trim leading blank rows; they would otherwise become an empty header.
	start := 0
	for start < len(grid) && allBlank(grid[start]) {
		start++
	}
	if start == len(grid) {
		return nil, nil
	}

	trimmed := grid[start:]
	header := trimmed[0]
	var rows [][]string
	for _, r := range trimmed[1:] {
		rows = append(rows, r)
	}
	return header, rows
}

func cellText(cell xlsxCell, shared []string, dateStyles map[int]bool) string {
	switch cell.T {
	case "s": // shared string
		idx, err := strconv.Atoi(strings.TrimSpace(cell.V))
		if err != nil || idx < 0 || idx >= len(shared) {
			return ""
		}
		return shared[idx]
	case "inlineStr":
		out := cell.IS.T.Value
		for _, run := range cell.IS.R {
			out += run.T.Value
		}
		return out
	case "str": // formula result rendered as text
		return cell.V
	case "b":
		if strings.TrimSpace(cell.V) == "1" {
			return "true"
		}
		return "false"
	case "e": // error
		return cell.V
	default: // numeric or date-styled numeric
		raw := strings.TrimSpace(cell.V)
		if raw == "" {
			return ""
		}
		if styleIdx, err := strconv.Atoi(strings.TrimSpace(cell.S)); err == nil && dateStyles[styleIdx] {
			if formatted, ok := excelSerialToDate(raw); ok {
				return formatted
			}
		}
		return raw
	}
}

// excelSerialToDate converts an Excel serial date to text. Day-only serials
// render as YYYY-MM-DD; serials with a time fraction render to the second.
func excelSerialToDate(raw string) (string, bool) {
	serial, err := strconv.ParseFloat(raw, 64)
	if err != nil || serial <= 0 {
		return "", false
	}
	days := int(serial)
	frac := serial - float64(days)
	t := excelEpoch.AddDate(0, 0, days).Add(time.Duration(frac * float64(24*time.Hour)))
	if frac == 0 {
		return t.Format("2006-01-02"), true
	}
	return t.Format("2006-01-02 15:04:05"), true
}

func parseRowIndex(ref string) int {
	if ref == "" {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(ref))
	if err != nil || n <= 0 {
		return -1
	}
	return n - 1
}

// columnIndex maps a cell reference ("B12") to a 0-based column index.
func columnIndex(ref string) int {
	n := 0
	seen := false
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			if r < 'a' || r > 'z' {
				break
			}
			r = r - 'a' + 'A'
		}
		n = n*26 + int(r-'A'+1)
		seen = true
	}
	if !seen {
		return -1
	}
	return n - 1
}

func allBlank(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
