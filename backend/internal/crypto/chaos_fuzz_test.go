package crypto

// fuzz 回归探针（chaos 测试转正，2026-09-20）：钉住加解密对畸形输入不 panic。

import (
	"testing"
)

func FuzzZZDecrypt(f *testing.F) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	valid, _ := Encrypt(key, "hello")

	seeds := []string{
		"",
		"v1:",
		"v1:AAAA",
		"v1:!!!!",
		"v1:====",
		"v1:" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"plaintext",
		"V1:AAAA",
		valid,
		"v1:aGVsbG8=", // 非密文但合法 base64
	}
	for _, s := range seeds {
		f.Add(s)
	}

	shortKey := []byte("short")
	emptyKey := []byte{}
	f.Fuzz(func(t *testing.T, s string) {
		// 必须不 panic
		pt, err := Decrypt(key, s)
		if err == nil {
			// 成功：必须是本包产出的密文；明文不得为空串意外通过
			if len(s) < len(cipherPrefix)+1 {
				t.Fatalf("Decrypt(%q) 无错返回 %q，但输入过短不可能是密文", s, pt)
			}
			re, rerr := Encrypt(key, pt)
			if rerr != nil {
				t.Fatalf("Encrypt(Decrypt(%q)) 失败: %v", s, rerr)
			}
			_ = re
		}
		// 弱/空 key 也必须走错误路径而非 panic
		if _, err := Decrypt(shortKey, s); err == nil {
			t.Fatalf("Decrypt(短 key, %q) 未报错", s)
		}
		if _, err := Decrypt(emptyKey, s); err == nil {
			t.Fatalf("Decrypt(空 key, %q) 未报错", s)
		}
		if _, err := Encrypt(shortKey, s); err == nil {
			t.Fatalf("Encrypt(短 key, %q) 未报错", s)
		}
	})
}
