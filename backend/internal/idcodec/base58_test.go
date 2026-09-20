package idcodec

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestEncode_KnownVectors 用外部独立算出的向量钉死编码结果。
// 向量由 Python 的 int/divmod 实现产出，与被测实现无共享代码。
func TestEncode_KnownVectors(t *testing.T) {
	cases := []struct{ id, want string }{
		{"00000000-0000-0000-0000-000000000000", "111111111111111111111"},
		{"00000000-0000-0000-0000-000000000001", "111111111111111111112"},
		{"00000000-0000-0000-0000-0000000000ff", "11111111111111111115Q"},
		{"0198f2c3-4d5e-7a6b-8c9d-0e1f2a3b4c5d", "CSatF9qXyRoQXFtAA3iZS"},
		{"0198f2c3-4000-7a6b-8c9d-0e1f2a3b4c5d", "CSatEuCxuTWvSRAgRVkDS"},
	}
	for _, c := range cases {
		if len(c.want) != EncodedLen {
			t.Fatalf("测试向量 %q 长度 = %d, want %d（向量写错了）", c.want, len(c.want), EncodedLen)
		}
		got, err := Encode(uuid.MustParse(c.id))
		if err != nil {
			t.Errorf("Encode(%s) 意外失败: %v", c.id, err)
			continue
		}
		if got != c.want {
			t.Errorf("Encode(%s) = %q, want %q", c.id, got, c.want)
			continue
		}
		back, err := Decode(c.want)
		if err != nil {
			t.Errorf("Decode(%q) 意外失败: %v", c.want, err)
			continue
		}
		if back.String() != c.id {
			t.Errorf("Decode(%q) = %s, want %s", c.want, back, c.id)
		}
	}
}

// TestEncodeDecode_RoundTrip 覆盖真实生成路径。
//
// 只用 v7：21 位定长（58^21 ≈ 2^123.04）装不下整个 128 位值域，v4 的首字节是
// 随机的，绝大多数（首字节 > 0x08 的约 97%）根本编不出来。我们的 query_id 全部
// 由 uuid.NewV7() 生成，值域天然受限，所以这是设计约束而不是缺陷——见
// EncodedLen 的 doc 与 TestEncode_RejectsValuesBeyond21Digits。
func TestEncodeDecode_RoundTrip(t *testing.T) {
	ids := []uuid.UUID{
		uuid.Nil,
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-0000000000ff"),
	}
	for i := 0; i < 500; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("NewV7 失败: %v", err)
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		enc, err := Encode(id)
		if err != nil {
			t.Fatalf("Encode(%s) 失败: %v", id, err)
		}
		if len(enc) != EncodedLen {
			t.Fatalf("Encode(%s) 长度 = %d, want %d", id, len(enc), EncodedLen)
		}
		got, err := Decode(enc)
		if err != nil {
			t.Fatalf("Decode(%q) 失败: %v", enc, err)
		}
		if got != id {
			t.Fatalf("往返不一致: %s -> %q -> %s", id, enc, got)
		}
	}
}

// TestEncode_LeadingZeroBytes 低位值必须靠左补齐 '1' 保住定长，而不是被截断
// （朴素的「大整数除法 + 反转字符串」写法会吃掉前导零，解码回来少字节、查库落空）。
func TestEncode_LeadingZeroBytes(t *testing.T) {
	cases := []string{
		"00000000-0000-0000-0000-000000000000",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-0000000000ff",
	}
	for _, id := range cases {
		enc, err := Encode(uuid.MustParse(id))
		if err != nil {
			t.Errorf("Encode(%s) 失败: %v", id, err)
			continue
		}
		if len(enc) != EncodedLen {
			t.Errorf("Encode(%s) 长度 = %d, want %d", id, len(enc), EncodedLen)
			continue
		}
		if enc[0] != alphabet[0] {
			t.Errorf("Encode(%s) = %q，低位值应补 %q", id, enc, string(alphabet[0]))
			continue
		}
		back, err := Decode(enc)
		if err != nil {
			t.Errorf("Decode(%q) 失败: %v", enc, err)
			continue
		}
		if back.String() != id {
			t.Errorf("往返不一致: %s -> %q -> %s", id, enc, back)
		}
	}
}

// TestEncode_RejectsValuesBeyond21Digits 越界必须显式报错，绝不静默截断成另一个 id。
func TestEncode_RejectsValuesBeyond21Digits(t *testing.T) {
	cases := []struct{ name, id string }{
		{"全 f（128 位最大值）", "ffffffff-ffff-ffff-ffff-ffffffffffff"},
		{"v4 随机样本（首字节 0x55）", "550e8400-e29b-41d4-a716-446655440000"},
		{"v7 形状但时间戳全 1（公元 10889 年）", "ffffffff-ffff-7a6b-8c9d-0e1f2a3b4c5d"},
		{"恰好等于 58^21（表达上界，不含）", "0819237f-3896-f2f3-0bb8-32ce3da00000"},
	}
	for _, c := range cases {
		if _, err := Encode(uuid.MustParse(c.id)); !errors.Is(err, ErrNotEncodable) {
			t.Errorf("%s: Encode(%s) 错误 = %v, want ErrNotEncodable", c.name, c.id, err)
		}
	}
}

func TestDecode_Errors(t *testing.T) {
	pad := strings.Repeat("1", EncodedLen-1)
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"空串", "", ErrInvalidLength},
		{"短一位", strings.Repeat("1", EncodedLen-1), ErrInvalidLength},
		{"长一位", strings.Repeat("1", EncodedLen+1), ErrInvalidLength},
		{"旧 22 位定长", strings.Repeat("1", 22), ErrInvalidLength},
		{"含数字 0", "0" + pad, ErrInvalidChar},
		{"含大写 O", "O" + pad, ErrInvalidChar},
		{"含大写 I", "I" + pad, ErrInvalidChar},
		{"含小写 l", "l" + pad, ErrInvalidChar},
		{"含连字符（旧 UUID 形态）", pad + "-", ErrInvalidChar},
	}
	for _, c := range cases {
		_, err := Decode(c.in)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: Decode(%q) 错误 = %v, want %v", c.name, c.in, err, c.want)
		}
	}
}

// TestDecode_MaxDigitsIsMaxValue 21 个最大字符解出的必须是 58^21-1（而不是报错或截断）。
func TestDecode_MaxDigitsIsMaxValue(t *testing.T) {
	got, err := Decode(strings.Repeat("z", EncodedLen))
	if err != nil {
		t.Fatalf("Decode(21 个 z) 失败: %v", err)
	}
	want := new(big.Int).Sub(encodableLimit, big.NewInt(1))
	if new(big.Int).SetBytes(got[:]).Cmp(want) != 0 {
		t.Fatalf("Decode(21 个 z) = %s, want %s", got, want)
	}
}

func TestAlphabet_ExcludesAmbiguousChars(t *testing.T) {
	if len(alphabet) != 58 {
		t.Fatalf("字母表长度 = %d, want 58", len(alphabet))
	}
	for _, c := range "0OIl" {
		if strings.ContainsRune(alphabet, c) {
			t.Errorf("字母表不应包含易混字符 %q", c)
		}
	}
}

// TestEncodedLen_CoversV7UntilAtLeast2248 把「21 位够用」的数学依据固化成断言。
//
// uuid v7 的值 = 毫秒时间戳(48 位) << 80 | 低位(80 位)，因此「能编出来」等价于
// 时间戳 < 58^21 >> 80。这里用 2^43 毫秒（≈2248 年）作为保守下界：真上界
// （约 2252 年）由 TestDecode_MaxDigitsIsMaxValue 间接钉住，不必写死日期。
func TestEncodedLen_CoversV7UntilAtLeast2248(t *testing.T) {
	maxTimestampMs := new(big.Int).Rsh(encodableLimit, 80)
	lowerBound := new(big.Int).Lsh(big.NewInt(1), 43) // 2^43 ms ≈ 2248 年
	if maxTimestampMs.Cmp(lowerBound) < 0 {
		t.Fatalf(
			"21 位只能覆盖到时间戳 %s ms（早于 2^43 ms ≈ 2248 年），v7 的 id 会编不出来；"+
				"EncodedLen 需要改回 22",
			maxTimestampMs,
		)
	}
}

// TestEncodedLen_BoundsDecodeToRawBytes 钉住 Decode 的安全前提：
// 58^EncodedLen ≤ 2^128，任何 EncodedLen 长的串解出的值都塞得进 16 字节。
// 一旦有人调大 EncodedLen 破坏这条不变量，ErrOutOfRange 分支就必须重新变成可达的。
func TestEncodedLen_BoundsDecodeToRawBytes(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 8*rawLen)
	if encodableLimit.Cmp(limit) > 0 {
		t.Fatalf("58^%d > 2^%d，Decode 可能解出超过 16 字节的值", EncodedLen, 8*rawLen)
	}
}
