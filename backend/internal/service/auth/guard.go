package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// 认证端点限流与失败锁定（R-87 / issue #203）。
//
// 只保护认证端点（/api/auth/login、/api/auth/register），其余端点本期不挂。
// 两个维度互不干扰：
//   - IP 维度：同一来源 IP 每分钟 N 次（令牌桶，golang.org/x/time/rate）。
//   - 用户名维度：同一用户名连续失败 M 次后锁定 K 秒。
//
// 实现是**单进程内存态**：多副本部署时各副本独立计数，口径差异写进
// docs/user-guide/configuration.md（issue #203 明确不做分布式限流，属 #75 Phase C）。
// 内存态意味着进程重启后计数清零 —— 对「爆破防护」可接受，对「审计留痕」
// 不负责（留痕走 slog，见 service 内调用点）。
//
// 并发安全：Guard 内部一把 mutex 保护用户名状态表；rate.Limiter 自身线程安全。
// 所有时间都走注入的 now（生产传 time.Now，测试传假时钟），因此无需 sleep。
type Guard struct {
	mu    sync.Mutex
	now   func() time.Time
	ips   map[string]*rate.Limiter
	users map[string]*userState

	ratePerMinute int
	maxFailures   int
	lockDuration  time.Duration
}

// userState 是某个用户名的失败计数与锁定窗口。
type userState struct {
	failures  int
	lockUntil time.Time
}

// GuardOptions 是 Guard 的构造参数。零值字段回退内置默认值。
type GuardOptions struct {
	// RatePerMinute 是同一来源 IP 每分钟允许的请求数，默认 defaultRatePerMinute。
	RatePerMinute int
	// MaxFailures 是同一用户名触发锁定的连续失败次数，默认 defaultMaxFailures。
	MaxFailures int
	// LockSeconds 是锁定持续秒数，默认 defaultLockSeconds。
	LockSeconds int
}

const (
	defaultRatePerMinute = 10
	defaultMaxFailures   = 5
	defaultLockSeconds   = 300
)

// NewGuard 构造一个认证守卫。now 为时钟源（生产传 time.Now，测试传假时钟）。
func NewGuard(opts GuardOptions, now func() time.Time) *Guard {
	if now == nil {
		now = time.Now
	}
	rpm := opts.RatePerMinute
	if rpm <= 0 {
		rpm = defaultRatePerMinute
	}
	mf := opts.MaxFailures
	if mf <= 0 {
		mf = defaultMaxFailures
	}
	lock := time.Duration(opts.LockSeconds) * time.Second
	if opts.LockSeconds <= 0 {
		lock = defaultLockSeconds * time.Second
	}
	return &Guard{
		now:           now,
		ips:           make(map[string]*rate.Limiter),
		users:         make(map[string]*userState),
		ratePerMinute: rpm,
		maxFailures:   mf,
		lockDuration:  lock,
	}
}

// limiterFor 返回该 IP 的令牌桶（懒创建）。rate.Limiter 并发安全，map 访问由 g.mu 保护。
func (g *Guard) limiterFor(ip string) *rate.Limiter {
	lim, ok := g.ips[ip]
	if !ok {
		// 每分钟 N 次 = 每秒 N/60 的补充速率，瞬时突发上限 N（一个完整窗口）。
		lim = rate.NewLimiter(rate.Limit(float64(g.ratePerMinute)/60.0), g.ratePerMinute)
		g.ips[ip] = lim
	}
	return lim
}

// AllowIP 检查 IP 维度是否放行。返回 (retryAfter, allowed)：retryAfter>0 表示被限流，
// 值是下一次可放行的估算等待时间。
func (g *Guard) AllowIP(ip string) (time.Duration, bool) {
	g.mu.Lock()
	lim := g.limiterFor(ip)
	g.mu.Unlock()
	if lim.Allow() {
		return 0, true
	}
	// 令牌耗尽：估算等到下一个令牌（一个窗口的 1/N）。
	return time.Duration(float64(time.Minute) / float64(g.ratePerMinute)), false
}

// AllowUser 检查用户名维度是否放行（锁定窗口内拒绝）。返回 (retryAfter, allowed)。
func (g *Guard) AllowUser(username string) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.users[username]
	if !ok {
		return 0, true
	}
	now := g.now()
	if st.lockUntil.After(now) {
		return st.lockUntil.Sub(now), false
	}
	return 0, true
}

// RecordLoginFailure 记录一次登录失败。当连续失败达到上限时触发锁定，
// 返回 (lockDuration, locked)：locked=true 表示本次调用恰好触发了锁定。
func (g *Guard) RecordLoginFailure(username string) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.users[username]
	if !ok {
		st = &userState{}
		g.users[username] = st
	}
	st.failures++
	if st.failures >= g.maxFailures {
		st.lockUntil = g.now().Add(g.lockDuration)
		return g.lockDuration, true
	}
	return 0, false
}

// RecordLoginSuccess 清空该用户名的失败计数与锁定窗口（登录成功即重置）。
func (g *Guard) RecordLoginSuccess(username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if st, ok := g.users[username]; ok {
		st.failures = 0
		st.lockUntil = time.Time{}
	}
}

// ClearUser 移除某个用户名的状态（供测试或后续管理操作使用）。
func (g *Guard) ClearUser(username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.users, username)
}
