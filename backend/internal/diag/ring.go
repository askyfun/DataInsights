// Package diag 维护进程内的最近错误 ring buffer，供诊断包（R-104 / #229）导出。
// 记录内容一律经 logx.Redact 脱敏后再入队，绝不保存明文机密。
package diag

import (
	"sync"
	"time"

	"tablewave/internal/logx"
)

// Entry 是一条最近错误记录。user_id 供前端「最近错误」按用户过滤。
type Entry struct {
	Time    string `json:"time"`
	ErrorID string `json:"error_id"`
	Code    int    `json:"code"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	UserID  int    `json:"user_id"`
	Message string `json:"message"`
}

type buffer struct {
	mu    sync.Mutex
	size  int
	items []Entry
}

var defaultBuf = &buffer{size: 256}

// Add 脱敏后写一条错误记录（超过容量丢最旧）。
func Add(e Entry) {
	e.Message = logx.Redact(e.Message)
	if e.Time == "" {
		e.Time = time.Now().Format(time.RFC3339)
	}
	defaultBuf.mu.Lock()
	defer defaultBuf.mu.Unlock()
	defaultBuf.items = append(defaultBuf.items, e)
	if len(defaultBuf.items) > defaultBuf.size {
		defaultBuf.items = defaultBuf.items[len(defaultBuf.items)-defaultBuf.size:]
	}
}

// Snapshot 取最近 n 条；all=false 时只返回该 userID 的记录（前端「最近错误」用）。
func Snapshot(n int, userID int, all bool) []Entry {
	defaultBuf.mu.Lock()
	defer defaultBuf.mu.Unlock()
	out := make([]Entry, 0, len(defaultBuf.items))
	for _, e := range defaultBuf.items {
		if all || e.UserID == userID {
			out = append(out, e)
		}
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}
