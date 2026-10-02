package extract

import (
	"strconv"
	"strings"
	"time"
)

// Inferred type names reuse the standard vocabulary (model.StandardDataType)
// so an inferred column can be written straight into entity.DatasetColumn.Type
// without a translation step.
const (
	TypeInteger  = "integer"
	TypeFloat    = "float"
	TypeBoolean  = "boolean"
	TypeDate     = "date"
	TypeDateTime = "datetime"
	TypeString   = "string"
)

// Column is a parsed column with its inferred standard type. The stable
// DatasetColumn.id is assigned later, at persistence time.
type Column struct {
	// Name is the display name (the upload's header cell); Physical is the
	// bare identifier the extract table actually uses. They differ because a
	// header may carry characters (spaces, CJK) that are illegal in a bare SQL
	// identifier; Expr on the persisted column points at Physical.
	Name     string
	Physical string
	Type     string
}

// InferColumns derives one type per column by scanning the data rows.
// A column is only typed when every non-empty value agrees; any disagreement
// (or no values at all) falls back to string, the safe lossless choice.
func InferColumns(t *Table) []Column {
	cols := make([]Column, len(t.Header))
	for i, name := range t.Header {
		cols[i] = Column{Name: name, Type: inferColumnType(t, i)}
	}
	return cols
}

func inferColumnType(t *Table, col int) string {
	hasValue := false
	allInt, allFloat, allBool := true, true, true
	allDate, allTemporal := true, true

	for _, row := range t.Rows {
		if col >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[col])
		if v == "" {
			continue
		}
		hasValue = true

		if allInt && !isInteger(v) {
			allInt = false
		}
		if allFloat {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				allFloat = false
			}
		}
		if allBool && !isBoolean(v) {
			allBool = false
		}
		if allDate && !isDateOnly(v) {
			allDate = false
		}
		if allTemporal && !isTemporal(v) {
			allTemporal = false
		}
		if !allInt && !allFloat && !allBool && !allDate && !allTemporal {
			break
		}
	}

	switch {
	case !hasValue:
		return TypeString
	case allInt:
		return TypeInteger
	case allFloat:
		return TypeFloat
	case allBool:
		return TypeBoolean
	case allDate:
		return TypeDate
	case allTemporal:
		return TypeDateTime
	default:
		return TypeString
	}
}

func isInteger(s string) bool {
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		return false
	}
	return true
}

func isBoolean(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false":
		return true
	default:
		return false
	}
}

var dateOnlyLayouts = []string{
	"2006-01-02",
	"2006/01/02",
	"2006/1/2",
	"2006-1-2",
}

var dateTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006/01/02 15:04:05",
	"2006/1/2 15:04:05",
	time.RFC3339,
}

func isDateOnly(s string) bool {
	for _, layout := range dateOnlyLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

// isTemporal accepts both date-only and datetime values, so a column that
// mixes the two still resolves to datetime rather than degrading to string.
func isTemporal(s string) bool {
	if isDateOnly(s) {
		return true
	}
	for _, layout := range dateTimeLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}
