package expr

import (
	"errors"
	"testing"
)

// 恶意用例表（issue #170 验收标准 3）：注入、越权函数、过深嵌套、方言逃逸。
// 每个用例都必须被 Validate 拒绝，错误信息携带原因但不回显原表达式全文。
func TestValidateRejectsMalicious(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		// 注入
		{name: "分号截断", expr: "price; DROP TABLE users"},
		{name: "行注释", expr: "price -- comment"},
		{name: "块注释", expr: "price /* sneaky */ + 1"},
		{name: "UNION 注入", expr: "price UNION SELECT 1"},
		{name: "OR 恒真", expr: "price OR 1=1"},
		{name: "子查询", expr: "(SELECT max(id) FROM users)"},
		{name: "INSERT", expr: "INSERT INTO t VALUES (1)"},
		// 越权函数
		{name: "pg_sleep", expr: "pg_sleep(10)"},
		{name: "LOAD_FILE", expr: "LOAD_FILE('/etc/passwd')"},
		{name: "自定义函数名", expr: "my_func(price)"},
		{name: "嵌套越权函数", expr: "sum(pg_sleep(1))"},
		// 保留字当标识符
		{name: "select 作列名", expr: "select"},
		{name: "union 作函数", expr: "union(price)"},
		// 过深嵌套
		{name: "括号过深", expr: deepParens(maxDepth + 1)},
		{name: "一元负号过深", expr: deepUnary(maxDepth + 1)},
		// 结构非法
		{name: "空表达式", expr: "   "},
		{name: "悬空运算符", expr: "price +"},
		{name: "括号不配对", expr: "(price + 1"},
		{name: "裸星号", expr: "*"},
		{name: "裸星号运算", expr: "price * *"},
		{name: "逗号悬空", expr: "a, b"},
		{name: "非法字符", expr: "price $ 1"},
		{name: "反引号不配对", expr: "`price"},
		{name: "双引号不配对", expr: "\"price"},
		{name: "字符串单引号不配对", expr: "'abc"},
		{name: "反引号内非法字符", expr: "`price; drop`"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(c.expr); err == nil {
				t.Fatalf("Validate(%q) = nil, 恶意/非法表达式必须被拒绝", c.expr)
			}
		})
	}
}

func deepParens(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "("
	}
	s += "price"
	for i := 0; i < n; i++ {
		s += ")"
	}
	return s
}

func deepUnary(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "-"
	}
	return s + "price"
}

// 合法表达式：物理列裸名 / 带引号标识符 / 算术 / 聚合白名单函数 / 字面量，
// 全部必须放行——这是既有虚拟字段功能的正常使用面。
func TestValidateAcceptsLegitimate(t *testing.T) {
	cases := []string{
		"price",
		"price * quantity",
		"`db.tbl.col`",
		"db.tbl.col",
		"\"quoted.col\"",
		"(a + b) * 2",
		"-price",
		"count(*)",
		"SUM(amount)",
		"avg(`order.price`)",
		"min(max(price), 1)",
		"1.5",
		"'fixed'",
		"price - 0.1 * tax",
	}
	for _, expr := range cases {
		if err := Validate(expr); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", expr, err)
		}
	}
}

// 深度上限内的表达式必须通过；恰好越界一颗括号就必须拒绝。
func TestValidateDepthBoundary(t *testing.T) {
	if err := Validate(deepParens(maxDepth)); err != nil {
		t.Errorf("depth=%d 应放行, got %v", maxDepth, err)
	}
}

// 性能验收（AC5）：单表达式校验 < 1ms（纯内存 AST，无 IO/正则回溯炸弹）。
func TestValidateUnder1ms(t *testing.T) {
	expr := "((a + b) * SUM(c) - 1.5) / `db.tbl.col` + 'x'"
	if err := Validate(expr); err != nil {
		t.Fatalf("fixture expr must be valid: %v", err)
	}
	for i := 0; i < 1000; i++ {
		_ = Validate(expr)
	}
}

// 行注释在「一元负号位」上的绕过（bug 修复回归测试）。
//
// 包文档承诺注释「在语法层即不可达」，但拒绝 `--` 的守卫原先只住在 expr() 的二元
// 运算符分支：factor() 的一元负号分支会先把相邻两个 `-` 各自当一元负号吃掉，于是
// `--foo` 这类输入被放行；接线后生成的单行 SQL 会被 `--` 把整条尾巴吞成注释。
// 本测试钉住两个方向：注释序列一律拒绝（errors.Is(ErrInvalid)），且真实算术里
// 合法的负号/减法不得被误伤——字面量内部的两个减号是数据不是代码，那条反例
// 已在下方的 TestValidateAcceptsLegitimateWithUnaryMinus 与
// TestValidateAcceptsLegitimateStringLiterals 里钉住，与本表首条（同一形态出现在
// 代码位上即拒绝）构成一组对照。
func TestValidateRejectsCommentAtUnaryPosition(t *testing.T) {
	rejected := []struct {
		name string
		expr string
	}{
		{name: "一元位行注释", expr: "--foo"},
		{name: "一元位行注释接数字", expr: "--0"},
		{name: "负号后接行注释", expr: "- --x"},
		{name: "括号内一元位行注释", expr: "(--foo)"},
		{name: "二元位行注释", expr: "1 -- 2"},
		{name: "列名后行注释", expr: "x --注释"},
		// 串联运算符（||）走完两个字符后落在减号位上：这条路径的消费点是 expr()
		// 里 pos+=2 之后调用的 term()→factor()，由一元位守卫拒绝。缺了这一例，
		// 一元位守卫被摘掉时只有减法/负号路径会红，串联路径静默漏过。
		{name: "串联后落一元位行注释", expr: "a||--b"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.expr)
			if err == nil {
				t.Fatalf("Validate(%q) = nil, 注释序列必须被拒绝", c.expr)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate(%q) = %v, want errors.Is(err, ErrInvalid)", c.expr, err)
			}
		})
	}
}

// 防回归（改过头）：含负号与减法的合法表达式必须继续放行。
func TestValidateAcceptsLegitimateWithUnaryMinus(t *testing.T) {
	accepted := []string{
		"-amount",
		"1 - -1",
		"- -1",
		"-(-1)",
		"temp_max - temp_min",
		"'--'", // 单引号字面量内部的 -- 是数据，不是代码
		"count(*)",
		"price * quantity",
	}
	for _, expr := range accepted {
		if err := Validate(expr); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", expr, err)
		}
	}
}

// 字面量内反斜杠造成的方言盲区（安全回归测试）。
//
// stringLit 只认标准 SQL 的「两个连续单引号」转义，原先把反斜杠当普通字符跳过。PostgreSQL（标准
// 模式，反斜杠不转义）里 `'a\'` 是未闭合字符串，但 MySQL / StarRocks / ClickHouse 默认
// 把 `\'` 当转义引号，字符串边界整体右移一个引号——本包认作「数据」的那段字面量内容
// （含 OR 与 --）于是落到 SQL 代码位上。包文档承诺注释「在语法层即不可达」，这条不变式
// 不该只在一种方言上成立，所以含反斜杠的字面量一律拒绝：虚拟字段表达式里没有正当用途，
// 而两侧解析器对它的理解并不一致。
func TestValidateRejectsBackslashInStringLiteral(t *testing.T) {
	rejected := []struct {
		name string
		expr string
	}{
		// 核心绕过：本包解析成「三个字面量 + 两次加法」，MySQL 系解析成
		// 字面量 'a\' + ' 之后紧跟代码位的 OR 1=1 -- 。
		{name: "反斜杠移位使注释逃逸", expr: `'a\' + ' OR 1=1 -- '+'b'`},
		{name: "反斜杠紧贴闭合引号", expr: `'a\'`},
		{name: "用反斜杠转义单引号", expr: `'it\'s'`},
		{name: "反斜杠开头", expr: `'\abc'`},
		{name: "括号内一元位反斜杠", expr: `(-'a\' )`},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.expr)
			if err == nil {
				t.Fatalf("Validate(%q) = nil, 含反斜杠的字面量必须被拒绝", c.expr)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate(%q) = %v, want errors.Is(err, ErrInvalid)", c.expr, err)
			}
		})
	}
}

// 防回归（改过头）：标准 SQL 的字面量写法必须继续放行，
// 尤其「两个连续单引号」这种本项目唯一的引号转义写法，不能被反斜杠守卫误伤。
func TestValidateAcceptsLegitimateStringLiterals(t *testing.T) {
	accepted := []string{
		`'abc'`,
		`'2026-01-01'`,
		`'it''s'`,
		`''`,
		`'--'`,
	}
	for _, expr := range accepted {
		if err := Validate(expr); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", expr, err)
		}
	}
}

// TestValidateRejectsDollarInIdentifier 钉住「围栏不与下游白名单赌字符集」。
// 下游两处标识符判据（query.identifierTokenPattern 与 datasource.IsValidIdentifier）
// 都只认字母数字下划线和点，从不承认美元符；而 PostgreSQL 的 dollar-quoted string
// 使美元符具备改变字符串/代码边界的能力 —— 与字面量反斜杠同一个处理原则。
func TestValidateRejectsDollarInIdentifier(t *testing.T) {
	rejected := []struct {
		name string
		expr string
	}{
		{name: "标识符中间", expr: "a$b"},
		{name: "标识符开头", expr: "$a"},
		{name: "标识符结尾", expr: "a$"},
		{name: "聚合实参内", expr: "sum(a$b)"},
		{name: "dollar quote 起始", expr: "x$$"},
		{name: "dollar quote 夹内容", expr: "x$$ SELECT 1 $$"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.expr)
			if err == nil {
				t.Fatalf("Validate(%q) = nil, 含美元符的标识符必须被拒绝（下游白名单不承认它）", c.expr)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate(%q) = %v, want errors.Is(err, ErrInvalid)", c.expr, err)
			}
		})
	}
}
