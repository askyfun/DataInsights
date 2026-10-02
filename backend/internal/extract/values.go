package extract

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RowArgs converts parsed rows into driver arguments aligned with the physical
// column layout produced by PhysicalColumns/InsertSQL. Cells are converted to
// Go values matching their inferred type so every driver receives a value it
// understands (ints as int64, dates as time.Time, blanks as NULL for typed
// columns). A synthetic key column, when present, is filled with the 1-based
// row number.
func RowArgs(d Dialect, cols []Column, rows [][]string) ([][]any, error) {
	ordered := PhysicalColumns(d, cols)

	// Map each physical column back to its source cell index; the synthetic
	// key has no source.
	source := make(map[string]int, len(cols))
	for i, c := range cols {
		source[c.Physical] = i
	}

	out := make([][]any, len(rows))
	for r, row := range rows {
		args := make([]any, len(ordered))
		for c, col := range ordered {
			if col.Physical == syntheticKeyColumn {
				args[c] = int64(r + 1)
				continue
			}
			idx, ok := source[col.Physical]
			if !ok {
				return nil, fmt.Errorf("column %q has no source", col.Physical)
			}
			raw := ""
			if idx < len(row) {
				raw = strings.TrimSpace(row[idx])
			}
			v, err := convertValue(col.Type, raw)
			if err != nil {
				return nil, fmt.Errorf("row %d column %q: %w", r+1, col.Name, err)
			}
			args[c] = v
		}
		out[r] = args
	}
	return out, nil
}

// convertValue renders one cell as the Go value for its standard type. Blank
// cells become NULL for every type except string, where an empty cell is a
// genuine empty string.
func convertValue(standard, raw string) (any, error) {
	if raw == "" {
		if standard == TypeString {
			return "", nil
		}
		return nil, nil
	}
	switch standard {
	case TypeString:
		return raw, nil
	case TypeInteger:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", raw)
		}
		return n, nil
	case TypeFloat:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", raw)
		}
		return f, nil
	case TypeBoolean:
		return strings.EqualFold(raw, "true"), nil
	case TypeDate, TypeDateTime:
		t, err := parseTemporal(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q", raw)
		}
		return t, nil
	default:
		return raw, nil
	}
}

func parseTemporal(raw string) (time.Time, error) {
	for _, layout := range dateOnlyLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	for _, layout := range dateTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", raw)
}
