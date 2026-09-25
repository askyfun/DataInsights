package model

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住类型映射器枚举闭集与表达式推断纯函数。

import (
	"testing"
)

// FuzzZZTypeMappers 类型映射器：喂随机串必须不 panic，且返回值必在枚举内或报错。
func FuzzZZTypeMappers(f *testing.F) {
	for _, s := range []string{
		"", " ", "int", "INT", " int ", "int(11)", "tinyint(1)", "decimal(10,2)",
		"varchar(255)", "array<int>", "map<string,int>", "DateTime64(3)",
		"Tuple(String,Int)", "enum('a','b')", "numeric(1,2,3,4)",
		"(", ")", "(1)", "int(", "int)", "decimal(", "array<", "map<",
		"0", "已删除的schema", "😀", "\x00", "int\n;DROP TABLE t",
	} {
		f.Add(s)
	}
	mappers := []DataTypeMapper{
		&StarRocksMapper{}, &PostgreSQLMapper{}, &MySQLMapper{}, &ClickHouseMapper{},
	}
	valid := map[StandardDataType]bool{
		TypeFloat: true, TypeInteger: true, TypeBoolean: true, TypeString: true,
		TypeDate: true, TypeDateTime: true, TypeArray: true, TypeMap: true,
		TypeJSON: true, TypeUnknown: true,
	}
	f.Fuzz(func(t *testing.T, in string) {
		for _, m := range mappers {
			std, cfg, err := m.ToStandard(in)
			if err != nil {
				continue
			}
			if !valid[std] {
				t.Fatalf("%s.ToStandard(%q) = %q：不在枚举内", m.GetSourceName(), in, std)
			}
			// 回写必须不 panic 且非空
			if out := m.ToSource(std, cfg); out == "" {
				t.Fatalf("%s.ToSource(%q) 返回空串", m.GetSourceName(), std)
			}
			// 已知类型必须能显示
			if std.GetDisplayName() == "" {
				t.Fatalf("GetDisplayName(%q) 为空", std)
			}
		}
	})
}

// FuzzZZInferExpressionResultType 推断函数必须是**纯函数**：同一输入重复调用结果恒定。
// 顺带钉住「不可能返回枚举外的值」。
func FuzzZZInferExpressionResultType(f *testing.F) {
	for _, s := range []string{
		"", "a", "a + b", "a || b", "SUM(x)", "COUNT(*)", "MONTH(x)", "ABS(MONTH(x))",
		"SUM(ABS(x))", "DATEDIFF(YEAR, a, b)", "CASE WHEN x THEN y END", "COALESCE(a,b)",
		"a-b", "MOD(a,b)", "ROUND(POWER(2,3))", "MAXIMUM", "SUMMARY", "MONTHLY_SALES",
		"lower(UPPER(x))", "SUBSTRING(x,1,2) + LENGTH(y)",
	} {
		f.Add(s)
	}
	valid := map[StandardDataType]bool{
		TypeFloat: true, TypeInteger: true, TypeBoolean: true, TypeString: true,
		TypeDate: true, TypeDateTime: true, TypeArray: true, TypeMap: true,
		TypeJSON: true, TypeUnknown: true,
	}
	f.Fuzz(func(t *testing.T, in string) {
		first := InferExpressionResultType(in)
		if !valid[first] {
			t.Fatalf("InferExpressionResultType(%q) = %q：不在枚举内", in, first)
		}
		for i := 0; i < 64; i++ {
			if got := InferExpressionResultType(in); got != first {
				t.Fatalf("非纯函数：InferExpressionResultType(%q) 第 %d 次返回 %q，首次为 %q",
					in, i+2, got, first)
			}
		}
	})
}
