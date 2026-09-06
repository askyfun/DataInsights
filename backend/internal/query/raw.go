package query

import (
	"fmt"
	"strings"

	"dataray/internal/datasource"
)

// WrapPreviewSQL wraps a user source (table or subquery) for preview.
// For SQL sources the user SQL is wrapped in a subquery so a trailing LIMIT
// inside the user SQL stays a valid statement. For table sources the
// identifier is sanitized via safeIdentifier (invalid names degrade to the
// "_invalid_identifier" placeholder and are rejected by the database).
// limit <= 0 omits the LIMIT clause.
func WrapPreviewSQL(source string, sourceType SourceType, limit int) string {
	if sourceType == SourceTypeSQL {
		if limit > 0 {
			return fmt.Sprintf("SELECT * FROM (%s) AS _preview LIMIT %d", source, limit)
		}
		return fmt.Sprintf("SELECT * FROM (%s) AS _preview", source)
	}
	if limit > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d", safeIdentifier(source), limit)
	}
	return fmt.Sprintf("SELECT * FROM %s", safeIdentifier(source))
}

// BuildFieldDistributionSQL builds GROUP BY distribution over a source.
// fieldName is always validated as an identifier before interpolation. In the
// table branch, source (the table name) is validated too; in the sql branch,
// source is the user-provided querySQL, which the product intentionally allows
// to be arbitrary SQL, so it is not identifier-validated.
func BuildFieldDistributionSQL(field string, source string, sourceType SourceType, limit int) (string, error) {
	if !datasource.IsValidIdentifier(field) {
		return "", fmt.Errorf("invalid field name: %q", field)
	}
	if sourceType == SourceTypeSQL {
		return fmt.Sprintf("SELECT %s, COUNT(*) as _count FROM (%s) as _subquery GROUP BY %s ORDER BY _count DESC LIMIT %d",
			field, source, field, limit), nil
	}
	if !datasource.IsValidIdentifier(source) {
		return "", fmt.Errorf("invalid table name: %q", source)
	}
	return fmt.Sprintf("SELECT %s, COUNT(*) as _count FROM %s GROUP BY %s ORDER BY _count DESC LIMIT %d",
		field, source, field, limit), nil
}

// WrapCountSQL builds SELECT COUNT(*) over a source. In the sql branch source
// is arbitrary user SQL (product feature); in the table branch the identifier
// is sanitized via safeIdentifier — callers that need fail-fast validation
// should run BuildFieldDistributionSQL (or IsValidIdentifier) beforehand.
func WrapCountSQL(source string, sourceType SourceType) string {
	if sourceType == SourceTypeSQL {
		return fmt.Sprintf("SELECT COUNT(*) as _total FROM (%s) as _subquery", source)
	}
	return fmt.Sprintf("SELECT COUNT(*) as _total FROM %s", safeIdentifier(source))
}

// BuildTableDataSQL builds a paginated table scan with optional ordering.
// table and orderField must be valid identifiers; orderDir is normalized
// case-insensitively to ASC/DESC (anything not "desc" falls back to ASC).
func BuildTableDataSQL(table, orderField, orderDir string, limit, offset int) (string, error) {
	if !datasource.IsValidIdentifier(table) {
		return "", fmt.Errorf("invalid table name: %q", table)
	}
	order := ""
	if orderField != "" {
		if !datasource.IsValidIdentifier(orderField) {
			return "", fmt.Errorf("invalid sort field: %q", orderField)
		}
		order = fmt.Sprintf(" ORDER BY %s %s", orderField, normalizeSortOrder(orderDir))
	}
	return fmt.Sprintf("SELECT * FROM %s%s LIMIT %d OFFSET %d", table, order, limit, offset), nil
}

// normalizeSortOrder validates sort direction case-insensitively; anything
// other than "desc" (any case, trimmed) falls back to "ASC".
func normalizeSortOrder(order string) string {
	if strings.EqualFold(strings.TrimSpace(order), "DESC") {
		return "DESC"
	}
	return "ASC"
}
