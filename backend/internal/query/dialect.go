package query

import (
	"strings"
)

type DialectType string

const (
	DialectMySQL      DialectType = "mysql"
	DialectPostgreSQL DialectType = "postgresql"
	DialectClickHouse DialectType = "clickhouse"
)

func ParseDialect(s string) DialectType {
	switch strings.ToLower(s) {
	case "mysql", "starrocks":
		return DialectMySQL
	case "postgresql", "postgres":
		return DialectPostgreSQL
	case "clickhouse":
		return DialectClickHouse
	default:
		return DialectMySQL
	}
}

// BuildQueryStringWithBun 构建带占位符的 SQL 并返回参数化 args。
// 值经 args 传给驱动，不落入 SQL 文本。
func BuildQueryStringWithBun(dialect DialectType, ast *QueryAST) (string, string, []any) {
	qb := NewBunSQLBuilder(dialect)
	selectSQL, selectArgs := qb.BuildSelect(ast)
	countSQL, countArgs := qb.BuildCount(ast)
	_ = countArgs // count 与 select 的 filter args 相同
	return selectSQL, countSQL, selectArgs
}
