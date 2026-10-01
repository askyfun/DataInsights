package extract

import (
	"fmt"
	"strings"
	"unicode"
)

// Dialect identifies the SQL family of the designated extract storage
// datasource. Only the dialects the repo can actually talk to are modelled;
// anything else is rejected at ingest time with a clear message rather than
// emitting dialect-invalid DDL.
type Dialect string

const (
	DialectStarRocks  Dialect = "starrocks"
	DialectMySQL      Dialect = "mysql"
	DialectPostgreSQL Dialect = "postgresql"
)

// ParseDialect maps a stored datasource type to a supported extract dialect.
func ParseDialect(dsType string) (Dialect, error) {
	switch d := Dialect(strings.ToLower(strings.TrimSpace(dsType))); d {
	case DialectStarRocks, DialectMySQL, DialectPostgreSQL:
		return d, nil
	default:
		return "", fmt.Errorf("extract storage datasource type %q is not supported (use starrocks, mysql or postgresql)", dsType)
	}
}

func quote(d Dialect, name string) string {
	if d == DialectPostgreSQL {
		return `"` + name + `"`
	}
	return "`" + name + "`"
}

// sqlType maps an inferred standard type to the dialect's storage type.
func sqlType(d Dialect, standard string) (string, error) {
	switch standard {
	case TypeInteger:
		return "BIGINT", nil
	case TypeFloat:
		if d == DialectPostgreSQL {
			return "DOUBLE PRECISION", nil
		}
		return "DOUBLE", nil
	case TypeBoolean:
		return "BOOLEAN", nil
	case TypeDate:
		return "DATE", nil
	case TypeDateTime:
		if d == DialectPostgreSQL {
			return "TIMESTAMP", nil
		}
		return "DATETIME", nil
	case TypeString:
		switch d {
		case DialectStarRocks:
			return "VARCHAR(65533)", nil
		default:
			return "TEXT", nil
		}
	default:
		return "", fmt.Errorf("unsupported extract column type %q", standard)
	}
}

// keyableType reports whether a type may back a StarRocks key column.
// StarRocks rejects FLOAT/DOUBLE keys, so float columns can never be the key.
func keyableType(standard string) bool {
	switch standard {
	case TypeInteger, TypeBoolean, TypeDate, TypeDateTime, TypeString:
		return true
	default:
		return false
	}
}

// syntheticKeyColumn is the physical name of the row-index key added only when
// no user column can serve as a StarRocks key (an all-float upload).
const syntheticKeyColumn = "_di_row"

// layout is the physical column order for one extract table plus the key
// column name StarRocks needs (empty for dialects without key requirements).
type layout struct {
	ordered []Column
	key     string
}

// physicalLayout decides the physical column order. StarRocks requires a key
// column that is a prefix of the table, and its first column may not be DOUBLE,
// so the first keyable column is promoted to the front; an all-float upload
// gets a synthetic row-index key. The dataset's own column list keeps the
// original display order — Expr points at the physical name, so promotion is
// transparent to the query layer.
func physicalLayout(d Dialect, cols []Column) layout {
	if d != DialectStarRocks {
		return layout{ordered: cols}
	}

	for i, c := range cols {
		if !keyableType(c.Type) {
			continue
		}
		if i == 0 {
			return layout{ordered: cols, key: c.Physical}
		}
		ordered := make([]Column, 0, len(cols))
		ordered = append(ordered, c)
		ordered = append(ordered, cols[:i]...)
		ordered = append(ordered, cols[i+1:]...)
		return layout{ordered: ordered, key: c.Physical}
	}

	// No keyable column: prepend a synthetic row-index key.
	ordered := make([]Column, 0, len(cols)+1)
	ordered = append(ordered, Column{Name: syntheticKeyColumn, Physical: syntheticKeyColumn, Type: TypeInteger})
	ordered = append(ordered, cols...)
	return layout{ordered: ordered, key: syntheticKeyColumn}
}

// CreateTableSQL builds the DDL for a fresh extract table. The physical column
// order is taken from the layout so the key prefix contract holds.
func CreateTableSQL(d Dialect, table string, cols []Column) (string, error) {
	if len(cols) == 0 {
		return "", fmt.Errorf("extract table %s has no columns", table)
	}
	lay := physicalLayout(d, cols)

	parts := make([]string, len(lay.ordered))
	for i, c := range lay.ordered {
		t, err := sqlType(d, c.Type)
		if err != nil {
			return "", err
		}
		parts[i] = quote(d, c.Physical) + " " + t
	}
	body := strings.Join(parts, ", ")

	if d == DialectStarRocks {
		key := quote(d, lay.key)
		return fmt.Sprintf(
			"CREATE TABLE %s (%s) ENGINE=OLAP DUPLICATE KEY(%s) DISTRIBUTED BY HASH(%s) BUCKETS 1 PROPERTIES(\"replication_num\"=\"1\")",
			quote(d, table), body, key, key,
		), nil
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", quote(d, table), body), nil
}

// InsertSQL builds a parameterized multi-row INSERT. Values are never
// interpolated: the caller passes them as args (SQL red line). The column list
// follows the physical layout, so callers must order row values the same way.
func InsertSQL(d Dialect, table string, cols []Column, rowCount int) (string, error) {
	if rowCount < 1 {
		return "", fmt.Errorf("insert with %d rows", rowCount)
	}
	lay := physicalLayout(d, cols)

	names := make([]string, len(lay.ordered))
	for i, c := range lay.ordered {
		names[i] = quote(d, c.Physical)
	}
	placeholder := "?"
	if d == DialectPostgreSQL {
		placeholder = "$%d"
	}

	rows := make([]string, rowCount)
	arg := 0
	for r := range rows {
		cells := make([]string, len(lay.ordered))
		for c := range cells {
			if d == DialectPostgreSQL {
				arg++
				cells[c] = fmt.Sprintf(placeholder, arg)
			} else {
				cells[c] = placeholder
			}
		}
		rows[r] = "(" + strings.Join(cells, ", ") + ")"
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		quote(d, table), strings.Join(names, ", "), strings.Join(rows, ", ")), nil
}

// PhysicalColumns returns the physical column order for a dataset's column
// list, so callers can align row values with InsertSQL. It is exported for the
// dataset service, which builds the value tuples.
func PhysicalColumns(d Dialect, cols []Column) []Column {
	return physicalLayout(d, cols).ordered
}

// DropTableSQL builds the DDL that removes an extract table (dataset delete).
func DropTableSQL(d Dialect, table string) string {
	return "DROP TABLE IF EXISTS " + quote(d, table)
}

// TruncateSQL clears an extract table ahead of a replace upload.
func TruncateSQL(d Dialect, table string) string {
	if d == DialectStarRocks || d == DialectMySQL {
		return "TRUNCATE TABLE " + quote(d, table)
	}
	return "TRUNCATE TABLE " + quote(d, table)
}

// AssignPhysical fills each column's physical name from its display name,
// producing a bare identifier that is safe to interpolate into DDL/DML.
// Non-alphanumeric characters collapse to underscores; blank or digit-leading
// results get a c_ prefix; collisions get a numeric suffix.
func AssignPhysical(cols []Column) []Column {
	out := make([]Column, len(cols))
	used := make(map[string]int, len(cols))
	for i, c := range cols {
		name := sanitizeIdentifier(c.Name)
		if n, ok := used[name]; ok {
			used[name] = n + 1
			name = fmt.Sprintf("%s_%d", name, n+1)
		} else {
			used[name] = 1
		}
		out[i] = Column{Name: c.Name, Physical: name, Type: c.Type}
	}
	return out
}

func sanitizeIdentifier(name string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r > unicode.MaxASCII:
			// Non-ASCII (e.g. CJK) headers have no faithful ASCII form; each
			// run collapses to a single underscore rather than being dropped.
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "col"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "c_" + out
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
