package idcodec

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住 base58 短码解码的边界行为。

import (
	"testing"

	"github.com/google/uuid"
)

func FuzzZZDecode(f *testing.F) {
	seeds := []string{
		"",
		"1",
		"111111111111111111111",   // 21 个 1 → uuid.Nil
		"11111111111111111111",    // 20
		"1111111111111111111111",  // 22
		"zzzzzzzzzzzzzzzzzzzzz",   // 21 个最大字符
		"222222222222222222222",   // 大于 58^21 的候选
		"0OIl00000000000000000",   // 非字母表字符
		"ABCDEFGHJKLMNPQRSTUVWXY", // 23 长
		"aaaaaaaaaaaaaaaaaaaaa",   // 21
		"1' OR '1'='1'--",         // 注入形态
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		"😀😀😀😀😀😀😀", // 多字节
	}
	for _, s := range seeds {
		f.Add(s)
	}

	fresh := func() map[uuid.UUID]string { return make(map[uuid.UUID]string, 1024) }
	seen := fresh()
	f.Fuzz(func(t *testing.T, s string) {
		id, err := Decode(s)
		if err != nil {
			if id != uuid.Nil {
				t.Fatalf("Decode(%q) 出错却返回非 Nil uuid %v", s, id)
			}
			return
		}
		// 成功路径：必须长度 21
		if len(s) != EncodedLen {
			t.Fatalf("Decode(%q) 在长度 %d 下成功", s, len(s))
		}
		// 单射性：不同输入不得解出同一 UUID
		if prev, ok := seen[id]; ok && prev != s {
			t.Fatalf("碰撞：%q 与 %q 都解出 %v", prev, s, id)
		}
		seen[id] = s
		if len(seen) > 100000 {
			seen = fresh() // 有界内存：碰撞检测窗口滚动
		}
		// 往返
		enc, err := Encode(id)
		if err != nil {
			t.Fatalf("Encode(Decode(%q)) 失败: %v", s, err)
		}
		if enc != s {
			t.Fatalf("往返不等：Decode(%q)=%v，再 Encode=%q", s, id, enc)
		}
	})
}
