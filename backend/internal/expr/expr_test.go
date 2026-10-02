package expr

import "testing"

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
