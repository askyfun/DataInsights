// Package expr 提供 SQL 表达式的安全围栏（issue #170，G1 门禁）。
//
// 数据集「虚拟字段」的 expr 是用户可控且会原样进入 SQL 的表达式（bi_dataset.columns
// → query 包列索引 → SELECT 输出），在它之外结构化查询已由 bun_builder 的参数化与
// 标识符白名单守住。本包是表达式的唯一白名单出口：
//
//   - 递归下降解析单个标量表达式（标识符 / 字面量 / 算术 / 函数调用 / 括号），
//     解析失败即拒绝——不是完整 SQL 解析器，方言差异交给数据库报错（non-goal）。
//   - 函数名收敛到既有聚合白名单（count/sum/avg/min/max，与 query.aggExprPattern
//     同一词表，不另立）；保留字不得作为标识符出现。
//   - 嵌套深度上限 maxDepth，拒绝拖垮数据库的过深表达式。
//   - 注释（-- /* */）、分号、子查询等在语法层即不可达，天然拒绝。
//
// 接线点：datasetService.UpdateColumns（写入口，校验前移）与 query.buildColumnIndex
// （查询期兜底，覆盖绕过写入口落库的历史/恶意数据）。自定义 SQL 本体保持
// 「作者自负」，经它派生的列表达式一律过闸。
//
// 性能：纯内存单遍解析，单表达式 < 1ms。
package expr

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid 是所有校验失败的哨兵；调用方用 errors.Is 判定，
// 具体原因经 %w 包装保留在错误链里。
var ErrInvalid = errors.New("invalid expression")

// maxDepth 是表达式嵌套深度上限（括号嵌套 + 一元链）。行业对标 DataWind
// 的「单图 LOD 字段 ≤5」：表达式必须有围栏。32 足够覆盖任何真实派生列。
const maxDepth = 32

// funcWhitelist 与 query 包的聚合白名单（aggExprPattern / entity.ChartMetric.Agg
// 契约）同词表；新增函数必须两边同步。
var funcWhitelist = map[string]bool{
	"count": true,
	"sum":   true,
	"avg":   true,
	"min":   true,
	"max":   true,
}

// reserved 保留字：即便语法上能解析成标识符/函数，也一律拒绝。
// 覆盖跨方言的 DML/DDL/查询结构关键字，防「union 作列名」类逃逸。
var reserved = map[string]bool{
	"select": true, "insert": true, "update": true, "delete": true,
	"drop": true, "alter": true, "create": true, "truncate": true,
	"union": true, "all": true, "distinct": true, "from": true, "where": true,
	"group": true, "order": true, "by": true, "having": true, "limit": true,
	"offset": true, "join": true, "left": true, "right": true, "inner": true,
	"outer": true, "full": true, "cross": true, "on": true, "as": true,
	"and": true, "or": true, "not": true, "in": true, "is": true, "null": true,
	"case": true, "when": true, "then": true, "else": true, "end": true,
	"between": true, "like": true, "exists": true, "into": true, "values": true,
	"set": true, "grant": true, "revoke": true, "exec": true, "execute": true,
	"sleep": true, "benchmark": true, "load_file": true,
}

// Validate 校验单个标量 SQL 表达式；nil 表示安全放行。
func Validate(expr string) error {
	return (&parser{src: []rune(strings.TrimSpace(expr))}).run()
}

// ValidateArg 在 Validate 之上额外禁止表达式内出现函数调用。
// 用途：聚合函数实参位（query.safeAggArg）——实参必须是纯标识符/字面量/
// 算术复合表达式，防止 SUM(count(x)) 之类的嵌套聚合。
func ValidateArg(expr string) error {
	return (&parser{src: []rune(strings.TrimSpace(expr)), noFunc: true}).run()
}

type parser struct {
	src     []rune
	pos     int
	sawFunc bool
	noFunc  bool
}

// run 解析整个表达式并做终检。
func (p *parser) run() error {
	if len(p.src) == 0 {
		return fmt.Errorf("%w: empty", ErrInvalid)
	}
	if err := p.expr(0); err != nil {
		return err
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		return fmt.Errorf("%w: unexpected token at %d", ErrInvalid, p.pos)
	}
	if p.noFunc && p.sawFunc {
		return fmt.Errorf("%w: nested function call", ErrInvalid)
	}
	return nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' ||
		p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
		p.pos++
	}
}

func (p *parser) fail(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// expr := term (('+'|'-'|'||') term)*
func (p *parser) expr(depth int) error {
	if depth > maxDepth {
		return p.fail("nesting too deep (>%d)", maxDepth)
	}
	if err := p.term(depth); err != nil {
		return err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil
		}
		r := p.src[p.pos]
		if r == '+' || r == '-' {
			// '--' 是行注释，不是两次减法；在语法层直接拒绝。
			if r == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '-' {
				return p.fail("comment is not allowed")
			}
			p.pos++
		} else if r == '|' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '|' {
			p.pos += 2
		} else {
			return nil
		}
		if err := p.term(depth); err != nil {
			return err
		}
	}
}

// term := factor (('*'|'/'|'%') factor)*
func (p *parser) term(depth int) error {
	if err := p.factor(depth); err != nil {
		return err
	}
	for {
		p.skipSpace()
		if p.pos < len(p.src) {
			r := p.src[p.pos]
			if r == '*' || r == '/' || r == '%' {
				p.pos++
				if err := p.factor(depth); err != nil {
					return err
				}
				continue
			}
		}
		return nil
	}
}

// factor := ('-')? primary
func (p *parser) factor(depth int) error {
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
		if depth+1 > maxDepth {
			return p.fail("nesting too deep (>%d)", maxDepth)
		}
		return p.factor(depth + 1)
	}
	return p.primary(depth)
}

// primary := number | string | ident | funcall | '(' expr ')'
func (p *parser) primary(depth int) error {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return p.fail("unexpected end")
	}
	r := p.src[p.pos]
	switch {
	case r == '(':
		p.pos++
		if depth+1 > maxDepth {
			return p.fail("nesting too deep (>%d)", maxDepth)
		}
		if err := p.expr(depth + 1); err != nil {
			return err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return p.fail("unbalanced parenthesis")
		}
		p.pos++
		return nil
	case r >= '0' && r <= '9':
		return p.number()
	case r == '\'':
		return p.stringLit()
	case r == '`' || r == '"':
		return p.quotedIdent()
	default:
		return p.identOrCall(depth)
	}
}

func (p *parser) number() error {
	start := p.pos
	for p.pos < len(p.src) && (isDigit(p.src[p.pos]) || p.src[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return p.fail("bad number")
	}
	return nil
}

func (p *parser) stringLit() error {
	// 单引号字符串字面量；'' 为转义。内容不进白名单——字面量是数据不是代码。
	p.pos++ // opening '
	for {
		if p.pos >= len(p.src) {
			return p.fail("unterminated string literal")
		}
		r := p.src[p.pos]
		if r == '\'' {
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '\'' {
				p.pos += 2
				continue
			}
			p.pos++
			return nil
		}
		p.pos++
	}
}

func (p *parser) quotedIdent() error {
	q := p.src[p.pos]
	p.pos++
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != q {
		p.pos++
	}
	if p.pos >= len(p.src) {
		return p.fail("unbalanced quote")
	}
	inner := string(p.src[start:p.pos])
	p.pos++
	if inner == "" || !validQuotedInner(inner) {
		return p.fail("illegal characters in quoted identifier")
	}
	return nil
}

func validQuotedInner(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// identOrCall 解析裸标识符或白名单函数调用。星号仅在 count(*) 内合法。
func (p *parser) identOrCall(depth int) error {
	start := p.pos
	for p.pos < len(p.src) && isIdentChar(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return p.fail("unexpected rune %q at %d", string(p.src[p.pos]), p.pos)
	}
	name := strings.ToLower(string(p.src[start:p.pos]))
	if reserved[name] {
		return p.fail("reserved keyword %q", name)
	}
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '(' {
		if !funcWhitelist[name] {
			return p.fail("function %q not in whitelist", name)
		}
		p.sawFunc = true
		p.pos++
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == '*' {
			if name != "count" {
				return p.fail("%s(*) is not allowed", name)
			}
			p.pos++
			p.skipSpace()
			if p.pos >= len(p.src) || p.src[p.pos] != ')' {
				return p.fail("unbalanced parenthesis")
			}
			p.pos++
			return nil
		}
		if p.pos < len(p.src) && p.src[p.pos] == ')' {
			p.pos++
			return nil
		}
		for {
			if depth+1 > maxDepth {
				return p.fail("nesting too deep (>%d)", maxDepth)
			}
			if err := p.expr(depth + 1); err != nil {
				return err
			}
			p.skipSpace()
			if p.pos < len(p.src) && p.src[p.pos] == ',' {
				p.pos++
				continue
			}
			break
		}
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return p.fail("unbalanced parenthesis")
		}
		p.pos++
		return nil
	}
	return nil
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isIdentChar(r rune) bool {
	// '.' 属于标识符字符：与 query.identifierTokenPattern 的裸标识符形态
	// [a-zA-Z0-9_.]+ 对齐（db.tbl.col）。
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '$' || r == '.'
}
