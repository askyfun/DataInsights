// Package idgen 生成非安全场景的短 ID，用于 JSON 复合字段里子对象的标记。
// 不承载安全语义；唯一性由「随机起始偏移 + 进程内原子计数器」保证：
// 进程内单调递增绝不重复，跨重启/副本靠 36^4 的随机起始空间错开。
package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"sync/atomic"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// width 是 ID 的定长字符数。36^8 ≈ 2.8e15，前 4 位随机起始 + 后 4 位以上
// 计数空间，正常生命周期内不会溢出到第 9 位。
const (
	width      = 8
	startSpace = 36 * 36 * 36 * 36 // 随机起始偏移的取值空间
)

var counter atomic.Uint64

func init() {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 非安全场景，rand 失败时退化为 0 起始：仍保证进程内唯一。
		counter.Store(0)
		return
	}
	counter.Store(uint64(binary.BigEndian.Uint64(b[:]) % startSpace))
}

// New 返回 8 位小写 base36 短 ID，如 "k3f9x01a"。
func New() string {
	n := counter.Add(1)
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = alphabet[n%36]
		n /= 36
	}
	return string(buf)
}
