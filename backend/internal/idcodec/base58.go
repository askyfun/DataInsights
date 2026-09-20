// Package idcodec 负责分享链接里 id 的 URL 呈现编码。
//
// 存储层始终是 PostgreSQL 的 uuid 类型（128 位定长）；本包只在「出地址栏」与
// 「进地址栏」两侧做定长 base58 编解码，让链接更短、无连字符、不含易混字符。
// 定长是刻意的：地址栏形态稳定，便于比对、转述与断言。
package idcodec

import (
	"errors"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

// alphabet 是不含易混字符的 base58 字母表（Bitcoin 风格）：去掉了 0 O I l，
// 避免链接被念出来或手抄时出现「这是零还是欧」的歧义。
const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// EncodedLen 是编码后的固定字符数。
//
// 取 21 位而不是「装下整个 128 位值域」所需的最小 22 位（58^21 < 2^128 ≤ 58^22），
// 代价是编码不再是全值域函数：值 ≥ 58^21 的 uuid 编不出来，Encode 会返回
// ErrNotEncodable 而不是静默截断。这个代价是**刻意接受**的，因为 query_id 由
// 本服务用 uuid.NewV7() 生成，其值域天然受限：
//
//	value = 毫秒时间戳(48 位) << 80 | 版本/变体/随机位(80 位)
//
// 要求 value < 58^21 = 10764351351569111513009094806216900608，解得时间戳上界
// 8904062744726 毫秒 —— 约公元 2252 年（把 2^43 毫秒即 2248 年当作保守下界）。
// 也就是说 21 位覆盖 uuid v7 直到 2252 年，之后需要改回 22 位。
//
// 已写进测试的不变量：TestEncodedLen_CoversV7Until2252 钉住 v7 值域，
// TestEncode_RejectsValuesBeyond21Digits 钉住越界必须报错。
//
// 不足 21 位的值在左侧补 alphabet[0]（'1'），因此返回值长度恒为 EncodedLen。
const EncodedLen = 21

// rawLen 是 uuid 的字节长度。
const rawLen = 16

var (
	// ErrInvalidLength 表示输入长度不等于 EncodedLen。
	ErrInvalidLength = errors.New("idcodec: base58 串长度必须为 21")
	// ErrInvalidChar 表示输入含 base58 字母表之外的字符。
	ErrInvalidChar = errors.New("idcodec: base58 串含非法字符")
	// ErrNotEncodable 表示 uuid 的值超出 21 位 base58 的表达范围（≥ 58^21）。
	// 只有非 v7 生成的 uuid（如手写的 v4）才可能触发。
	ErrNotEncodable = errors.New("idcodec: uuid 超出 21 位 base58 的表达范围")
	// ErrOutOfRange 表示输入解出的整数超出 128 位，不可能是合法的 uuid。
	//
	// 在 EncodedLen=21 下该分支**不可达**：58^21 < 2^128，任何 21 字符串解出的
	// 值都塞得进 16 字节。保留它是为了让 Decode 对「未来有人调大 EncodedLen」
	// 保持不 panic（下方 copy 的起始下标一旦为负会直接崩），
	// 不变量由 TestEncodedLen_BoundsDecodeToRawBytes 钉住。
	ErrOutOfRange = errors.New("idcodec: base58 串超出 uuid 值域")
)

// radix 是 base58 的进制。
var radix = big.NewInt(int64(len(alphabet)))

// encodableLimit 是 21 位 base58 能表达的值上界（不含），即 58^21。
var encodableLimit = new(big.Int).Exp(big.NewInt(int64(len(alphabet))), big.NewInt(EncodedLen), nil)

// Encode 把 uuid 编成定长 21 字符的 base58 串。
//
// 值超出 58^21 时返回 ErrNotEncodable；低位值（含 uuid.Nil）在左侧补 '1'，
// 因此成功时返回值长度恒为 EncodedLen。
func Encode(id uuid.UUID) (string, error) {
	n := new(big.Int).SetBytes(id[:])
	if n.Cmp(encodableLimit) >= 0 {
		return "", ErrNotEncodable
	}
	buf := make([]byte, EncodedLen)
	rem := new(big.Int)
	for i := EncodedLen - 1; i >= 0; i-- {
		n.DivMod(n, radix, rem)
		buf[i] = alphabet[rem.Int64()]
	}
	return string(buf), nil
}

// Decode 把定长 base58 串还原成 uuid。
//
// 前导 '1' 一律按零处理 —— 它既可能是编码时左补齐产生的，也可能对应真实的前导零
// 字节，两者在数值上等价。因此 Decode(Encode(id)) == id 恒成立。
func Decode(s string) (uuid.UUID, error) {
	if len(s) != EncodedLen {
		return uuid.Nil, ErrInvalidLength
	}
	n := new(big.Int)
	unit := new(big.Int)
	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(alphabet, s[i])
		if idx < 0 {
			return uuid.Nil, ErrInvalidChar
		}
		n.Mul(n, radix)
		n.Add(n, unit.SetInt64(int64(idx)))
	}
	b := n.Bytes()
	if len(b) > rawLen {
		return uuid.Nil, ErrOutOfRange
	}
	var out uuid.UUID
	copy(out[rawLen-len(b):], b)
	return out, nil
}
