package datasource

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住标识符白名单字符集。

import (
	"strings"
	"testing"
)

// FuzzZZIsValidIdentifier 钉住标识符白名单的字符集：通过者只可能含 [a-zA-Z0-9_.]。
func FuzzZZIsValidIdentifier(f *testing.F) {
	for _, s := range []string{
		"", " ", "t", "t.a", "a..b", ".", "..", "a.", ".a", "1t", "t;",
		"t'", "t\"", "t`", "t ", " t", "t\n", "t\n;DROP", "t\u00a0",
		"t-t", "t(t)", "t,t", "t=t", "t%", "t*", "t/x", "t\\x",
		"已删除的schema", "schema.table", "public.users",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		ok := IsValidIdentifier(in)
		if !ok {
			return
		}
		for i := 0; i < len(in); i++ {
			c := in[i]
			switch {
			case c >= 'a' && c <= 'z':
			case c >= 'A' && c <= 'Z':
			case c >= '0' && c <= '9':
			case c == '_' || c == '.':
			default:
				t.Fatalf("IsValidIdentifier(%q)=true，但含白名单外字节 %q", in, c)
			}
		}
		if strings.ContainsAny(in, " \t\r\n'\"`;()[]{}-*/%=,<>|&\\") {
			t.Fatalf("IsValidIdentifier(%q)=true，但含危险字符", in)
		}
	})
}
