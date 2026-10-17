package query

// chaos/fuzz 目标（可行性验证，2026-10-17）：钉住 internal/expr 的表达式围栏不变式。
//
// 为什么住 query 包而不是 expr 包：expr.Validate 的判据要看「闸后真正落到 SQL 里的
// 文本」，而表达式文本由 query 包渲染（column_index.expr → bun_builder）。放这边才能
// 端到端断言。种子语料刻意不包含已知的绕过形态，绕过由 fuzz 自己挖。
//
// 四条不变式（第一条来自 expr 包文档承诺，第二条是 metamorphic 性质，第三、四条钉住
// 方言与下游词法的盲区）：
//  1. 放行的表达式在「字面量之外」不得出现 SQL 注释序列 `--` / `/*`，以及块注释
//     结束符 `*/` 与语句分隔符 `;` —— 包文档写的是「注释在语法层即不可达，天然
//     拒绝」，这条断言就是那句文档的可执行版本。判据表里的 `*/` 与 `;` 在当前
//     语法下同样不可达（`*` 与 `/` 之间必须夹一个因子；分号根本不是本语法里的
//     token），留着它们是**未来在该语法里新增运算符时的防线**：一旦有人加了
//     块注释闭合或语句分隔，fuzz 会立刻红，而不是等下一个绕过上线。
//     注意判据只看字面量之外：字面量内部那两个减号、分号在 SQL 里是数据，不是
//     代码，拿整段文本去查会被 fuzz 打成假阳性；
//  2. ValidateArg 是 Validate 的子集：实参位放行 ⇒ 整体表达式也必须放行。
//  3. 放行的表达式不得含反斜杠。这条**不能靠第 1 条代替**：本文件的涂白按 Go 的
//     字面量语义做，而 MySQL 系对 `\'` 的理解不同——`'a\' + ' OR 1=1 -- '` 里的注释
//     序列在 Go 语义下正好落在被涂白的字面量内，第 1 条永远看不到它（结构性失明）。
//     所以方言盲区必须由「禁止反斜杠」这条独立不变式守住。
//  4. 放行的表达式在「字面量之外」不得含美元符。下游两处标识符白名单
//     （query.identifierTokenPattern 与 datasource.IsValidIdentifier）只认
//     [a-zA-Z0-9_.]，从不承认它；而 PostgreSQL 的 dollar-quoted string 用美元符
//     界定字符串，围栏比下游多放它就等于把字符串边界交给方言。这条与第 3 条同形：
//     都是「围栏词法 ⊃ 下游/方言词法」造成的缺口，不能靠第 1 条的判据表覆盖。

import (
	"strings"
	"testing"

	"data-insights/internal/expr"
)

// zzTrimmedText 是「闸后落到 SQL 里的形态」：调用方（datasetService.UpdateColumns /
// query.buildColumnIndex）存的是原始 col.Expr，渲染时原样内插，两侧空白不参与语法。
func zzTrimmedText(s string) string { return strings.TrimSpace(s) }

// zzBlankLiterals 把单引号字面量覆盖的位置涂成空格，**保持原文长度不变**。
// 为什么要保长度：判据是「注释序列在真实 SQL 文本里是否真的相邻」。
// 「减号 + 空字面量 + 减号 + 空字面量」这类输入，两个减号之间隔着字面量；若把
// 字面量整段删掉再找，它们就假性地相邻成注释序列（fuzz 实测抓到过这个假阳性），
// 而真实 SQL 文本里这两个减号中间还有引号字符，不构成注释。涂白保长度才与数据库看到的一致。
// SQL 里「两个连续单引号」是引号转义。
// ⚠️ 本注释刻意不用那对字面量书写：gofmt 会把文档注释里的直引号对换成印刷引号
// （go1.27 实测），写出来会变成看不懂的东西，本文件的作者在这里踩过两次。
func zzBlankLiterals(s string) string {
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\'' {
			continue
		}
		// 字面量：连同两侧引号一起涂白，内部的双写转义不算结束。
		rs[i] = ' '
		for i++; i < len(rs); i++ {
			if rs[i] == '\'' {
				rs[i] = ' '
				if i+1 < len(rs) && rs[i+1] == '\'' {
					rs[i+1] = ' '
					i++
					continue
				}
				break
			}
			rs[i] = ' '
		}
	}
	return string(rs)
}

// FuzzZZExprNoCommentSequence 放行 ⇒ 字面量之外的文本不含注释序列与分号。
// 目标名保留 NoCommentSequence（语料目录按此名落在 testdata/fuzz 下，改名会让
// 已入库的种子回归失效），判据表见函数体内的 bad 列表。
func FuzzZZExprNoCommentSequence(f *testing.F) {
	// 合法形态：真实派生列
	f.Add("amount")
	f.Add("price * quantity")
	f.Add("count(*)")
	f.Add("SUM(`db.tbl.col`)")
	f.Add("temp_max - temp_min")
	f.Add("-amount")
	f.Add("a / (b + 1)")
	f.Add("'x' || name")
	// 反例锚点：字面量内部的 -- 与引号都是**数据**，放行且**不该**被判失败。
	// 没有这两条种子，下面的判据很容易被写回「整段文本查 --」那种过宽版本
	// （fuzz 实测会把它打成假阳性：'000"' 就是历史案例）。
	f.Add("'a--b'")
	f.Add("'000\"'")
	f.Add("-''-''")
	// 反斜杠形态（方言盲区，不变式 3）：Go 侧读成三个合法字面量，MySQL 侧以 `'a\' + '`
	// 闭合、其后的 OR 与 -- 落进代码位。围栏一律拒绝，种子负责钉住这条契约。
	f.Add(`'a\' + ' OR 1=1 -- '+'b'`)
	f.Add(`'a\'`)
	f.Add(`'it\'s'`)
	// 美元符形态（下游白名单不认，不变式 4）：PostgreSQL 的 dollar-quoted string
	// 以美元符界定字符串，`x$$` 起头的文本在 PG 上会被读成字符串起始。
	f.Add("a$b")
	f.Add("x$$")
	f.Add("sum($a)")
	// 敌意形态（历史用例，均已拒绝）
	f.Add("price; DROP TABLE users")
	f.Add("pg_sleep(10)")
	f.Add("amount UNION SELECT password")
	f.Add("a--b")
	f.Add("a /* x */ b")
	f.Add("/**/")
	f.Add("--")
	f.Add("(SELECT max(id) FROM users)")

	f.Fuzz(func(t *testing.T, in string) {
		if err := expr.Validate(in); err != nil {
			return // 拒绝即安全
		}
		// 不变式 3：放行的表达式不得含反斜杠。这条必须独立存在——下面的涂白判据按
		// Go 的字面量语义做，反斜杠移位进来的注释序列恰好落在被涂白的区域内，
		// 单靠那条判据对这一类绕过完全失明（回滚围栏守卫时本步也不会红）。
		if strings.ContainsRune(in, '\\') {
			t.Fatalf("expr.Validate(%q) 放行，但表达式含反斜杠：MySQL / StarRocks / ClickHouse "+
				"把「反斜杠+引号」当转义引号，字符串边界与本解析器不一致，"+
				"字面量内的注释序列因此在这些方言上落到 SQL 代码位", in)
		}
		outside := zzBlankLiterals(zzTrimmedText(in))
		// 不变式 4：字面量之外的美元符。字面量内它是数据（涂白已盖掉），
		// 代码位上它是 PostgreSQL dollar-quoted string 的定界符。
		if strings.Contains(outside, "$") {
			t.Fatalf("expr.Validate(%q) 放行，但字面量之外含美元符（判据文本：%q）："+
				"query.identifierTokenPattern 与 datasource.IsValidIdentifier 都不承认它，"+
				"而 PostgreSQL 用「美元符串」界定字符串，围栏多放即把字符串边界交给方言",
				in, outside)
		}
		// 判据表：注释序列（行/块两块）+ 块注释结束符 + 语句分隔符。
		// 四者在当前语法下都到不了代码位，后两条是给未来新增运算符留的防线。
		// 分号在字面量内是数据，上面的涂白已经把字面量整段（含引号）盖成空格。
		for _, bad := range []string{"--", "/*", "*/", ";"} {
			if strings.Contains(outside, bad) {
				t.Fatalf("expr.Validate(%q) 放行，但真实 SQL 文本里存在字面量之外的禁用序列 %q\n"+
					"（涂白字面量后的判据文本：%q）该表达式被 bun_builder 原样内插，"+
					"注释序列会让其后的内容被数据库当注释吞掉，分号会截断语句并让尾巴成为新语句",
					in, bad, outside)
			}
		}
	})
}

// FuzzZZExprArgSubset 实参位（ValidateArg）放行必须是整体放行（Validate）的子集。
func FuzzZZExprArgSubset(f *testing.F) {
	f.Add("count(*)")
	f.Add("price * quantity")
	f.Add("sum(count(x))")
	f.Add("a.b")
	f.Add("amount + 1")

	f.Fuzz(func(t *testing.T, in string) {
		if expr.ValidateArg(in) != nil {
			return
		}
		if err := expr.Validate(in); err != nil {
			t.Fatalf("ValidateArg(%q) 放行而 Validate 拒绝（%v）：子集关系破裂", in, err)
		}
	})
}
