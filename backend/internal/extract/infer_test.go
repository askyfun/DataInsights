package extract

import "testing"

func TestInferColumns(t *testing.T) {
	table := &Table{
		Header: []string{"id", "price", "active", "day", "ts", "label", "empty"},
		Rows: [][]string{
			{"1", "9.5", "true", "2024-01-01", "2024-01-01 10:00:00", "a", ""},
			{"2", "8", "false", "2024/02/03", "2024-01-02 11:30:00", "b", ""},
			{"3", "10.25", "true", "2024-2-4", "2024-01-03T09:00:00Z", "c", ""},
		},
	}
	want := map[string]string{
		"id":     TypeInteger,
		"price":  TypeFloat,
		"active": TypeBoolean,
		"day":    TypeDate,
		"ts":     TypeDateTime,
		"label":  TypeString,
		"empty":  TypeString,
	}
	cols := InferColumns(table)
	for _, c := range cols {
		if want[c.Name] != c.Type {
			t.Errorf("column %q inferred as %q, want %q", c.Name, c.Type, want[c.Name])
		}
	}
}

func TestInferColumnsMixedFallsBackToString(t *testing.T) {
	table := &Table{
		Header: []string{"mixed"},
		Rows:   [][]string{{"1"}, {"abc"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeString {
		t.Fatalf("mixed column inferred as %q, want string", cols[0].Type)
	}
}

func TestInferColumnsDateAndDateTimeMixIsDateTime(t *testing.T) {
	table := &Table{
		Header: []string{"t"},
		Rows:   [][]string{{"2024-01-01"}, {"2024-01-02 10:00:00"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeDateTime {
		t.Fatalf("mixed date/datetime inferred as %q, want datetime", cols[0].Type)
	}
}

func TestInferColumnsZeroOneIsIntegerNotBoolean(t *testing.T) {
	table := &Table{
		Header: []string{"flag"},
		Rows:   [][]string{{"0"}, {"1"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeInteger {
		t.Fatalf("0/1 column inferred as %q, want integer", cols[0].Type)
	}
}

func TestInferColumnsSkipsBlankCells(t *testing.T) {
	table := &Table{
		Header: []string{"amount"},
		Rows:   [][]string{{"1"}, {""}, {"42"}},
	}
	cols := InferColumns(table)
	if cols[0].Type != TypeInteger {
		t.Fatalf("column with blank cells inferred as %q, want integer", cols[0].Type)
	}
}

// TestInferColumnsMessyValuesFallBackToString（R-85 验收 5：推断精度验收）
//
// 这批用例钉的是「不猜」这条不变式：真实世界的表格里混着千分位、百分号、货币符号、
// 单位后缀，某一列只要出现一个和其余值不同形态的脏值，整列就必须退回字符串。
//
// 为什么不容许「尽力解析」：把 "1,234" 猜成 1234 或把 "50%" 猜成 50，
// 都是在替用户编数据——用户看不出区别，图却已经是错的。退回字符串则至少可见。
func TestInferColumnsMessyValuesFallBackToString(t *testing.T) {
	cases := map[string][]string{
		"千分位":        {"1,234", "5,678"},
		"千分位与纯数字混":  {"1,234", "5678"},
		"百分号":        {"50%", "30%"},
		"货币符号":       {"$1.50", "$2.00"},
		"带单位后缀":       {"10kg", "20kg"},
		"纯数字与文字混":    {"100", "N/A"},
		"会计负数括号":     {"(100)", "(200)"},
		"中文数字":       {"一", "二"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			rows := make([][]string, len(values))
			for i, v := range values {
				rows[i] = []string{v}
			}
			cols := InferColumns(&Table{Header: []string{"v"}, Rows: rows})
			if cols[0].Type != TypeString {
				t.Fatalf("%s inferred as %q, want string (values %v)", name, cols[0].Type, values)
			}
		})
	}
}

// TestInferColumnsCleanNumberColumnStaysNumeric 是上条的反向对照：
// 干净的数值列必须仍然被识别为数值，否则「退回字符串」就成了懒惰的兜底。
//
// 这里刻意包含两个容易被误当成「脏值」的形态：科学计数法（1e5）与前导零编号（007）。
// 它们都是合法数值文本，识别成数值才是对的——把它们算作脏值会让推断既不可信
// 又不可解释（用户看到 "007" 变7 会困惑，而这不是系统该替他做的决定）。
func TestInferColumnsCleanNumberColumnStaysNumeric(t *testing.T) {
	cases := map[string]struct {
		values []string
		want   string
	}{
		"整数":     {[]string{"1", "2", "3000"}, TypeInteger},
		"浮点":     {[]string{"1.5", "2.25", "3"}, TypeFloat},
		"负数":     {[]string{"-5", "10"}, TypeInteger},
		"科学计数法整数": {[]string{"1000000", "2000000"}, TypeInteger},
		"科学计数法浮点": {[]string{"1e5", "2.5e3"}, TypeFloat},
		"前导零编号":   {[]string{"007", "008"}, TypeInteger},
		"布尔":     {[]string{"true", "FALSE", "True"}, TypeBoolean},
		"全空列":     {[]string{"", "  ", ""}, TypeString},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rows := make([][]string, len(tc.values))
			for i, v := range tc.values {
				rows[i] = []string{v}
			}
			cols := InferColumns(&Table{Header: []string{"v"}, Rows: rows})
			if cols[0].Type != tc.want {
				t.Fatalf("%s inferred as %q, want %q", name, cols[0].Type, tc.want)
			}
		})
	}
}
