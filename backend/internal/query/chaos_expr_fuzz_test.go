package query

// chaos/fuzz 目标（可行性验证，2026-10-17）：钉住 internal/expr 的表达式围栏不变式。
//
// 为什么住 query 包而不是 expr 包：expr.Validate 的判据要看「闸后真正落到 SQL 里的
// 文本」，而表达式文本由 query 包渲染（column_index.expr → bun_builder）。放这边才能
// 端到端断言。种子语料刻意不包含已知的绕过形态，绕过由 fuzz 自己挖。
//
// 两条不变式（第一条来自 expr 包文档承诺，第二条是 metamorphic 性质）：
//  1. 放行的表达式在「字面量之外」不得出现 SQL 注释序列 `--` / `/*` —— 包文档写的是
//     「注释在语法层即不可达，天然拒绝」，这条断言就是那句文档的可执行版本。
//     注意判据只看字面量之外：`'a--b'` 里的 -- 在 SQL 里是数据，不是注释，
//     拿整段文本去查会被 fuzz 打成假阳性；
//  2. ValidateArg 是 Validate 的子集：实参位放行 ⇒ 整体表达式也必须放行。

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

// FuzzZZExprNoCommentSequence 放行 ⇒ 字面量之外的文本不含注释序列。
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
		outside := zzBlankLiterals(zzTrimmedText(in))
		for _, bad := range []string{"--", "/*"} {
			if strings.Contains(outside, bad) {
				t.Fatalf("expr.Validate(%q) 放行，但真实 SQL 文本里存在字面量之外的注释序列 %q\n"+
					"（涂白字面量后的判据文本：%q）该表达式被 bun_builder 原样内插，%q 之后到行尾会被数据库当注释吞掉",
					in, bad, outside, bad)
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
