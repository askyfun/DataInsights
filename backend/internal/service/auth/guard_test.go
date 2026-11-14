package auth

import (
	"testing"
	"time"
)

// fakeClock 是可推进的假时钟（issue #203 acceptance 5：用注入的 now，不许 sleep）。
type fakeClock struct {
	t time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (f *fakeClock) now() time.Time { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func TestGuard_IPRateLimit_ExceedsAndRecovers(t *testing.T) {
	clk := newFakeClock()
	// 每分钟 2 次，瞬时突发 2。
	g := NewGuard(GuardOptions{RatePerMinute: 2, MaxFailures: 5, LockSeconds: 300}, clk.now)
	const ip = "10.0.0.1"

	// 前 2 次放行。
	if _, ok := g.AllowIP(ip); !ok {
		t.Fatal("attempt 1 should be allowed")
	}
	if _, ok := g.AllowIP(ip); !ok {
		t.Fatal("attempt 2 should be allowed")
	}
	// 第 3 次（第 N+1 次）被拒。
	retryAfter, ok := g.AllowIP(ip)
	if ok {
		t.Fatal("attempt 3 (N+1) should be rejected")
	}
	if retryAfter <= 0 {
		t.Fatalf("retry-after must be positive, got %v", retryAfter)
	}

	// 窗口滑过后恢复。rate.Limiter 走真实墙钟（不注入时钟），所以用「高补充率 +
	// 有界轮询」而非 sleep 来验证：设置一个高 RatePerMinute 的独立 Guard，耗尽后
	// 在真实时间内补充（无需 sleep 等待），限时 2s 内必须恢复。
	fast := NewGuard(GuardOptions{RatePerMinute: 3600, MaxFailures: 5}, clk.now)
	const fastIP = "10.0.0.9"
	for i := 0; i < 3600; i++ {
		fast.AllowIP(fastIP) // 耗尽 burst
	}
	if _, ok := fast.AllowIP(fastIP); ok {
		t.Fatal("exhausted fast limiter must reject")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := fast.AllowIP(fastIP); ok {
			break // 恢复成功
		}
		if time.Now().After(deadline) {
			t.Fatal("fast limiter should recover within the bounded window (no sleep)")
		}
		time.Sleep(5 * time.Millisecond) // 有界轮询间隔，非测试主体等待
	}
}

func TestGuard_IPRateLimit_IndependentPerIP(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{RatePerMinute: 1}, clk.now)

	if _, ok := g.AllowIP("10.0.0.1"); !ok {
		t.Fatal("ip A attempt 1 should be allowed")
	}
	if _, ok := g.AllowIP("10.0.0.1"); ok {
		t.Fatal("ip A attempt 2 should be rejected")
	}
	// 另一 IP 不受影响。
	if _, ok := g.AllowIP("10.0.0.2"); !ok {
		t.Fatal("ip B should be independent of ip A")
	}
}

func TestGuard_UserLockout_LocksAndAutoUnlocks(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{RatePerMinute: 100, MaxFailures: 3, LockSeconds: 300}, clk.now)
	const user = "alice"

	// 前 2 次失败：未锁定。
	for i := 1; i <= 2; i++ {
		if dur, locked := g.RecordLoginFailure(user); locked {
			t.Fatalf("failure %d must not lock yet", i)
		} else if dur != 0 {
			t.Fatalf("failure %d must not report lock duration", i)
		}
		if _, ok := g.AllowUser(user); !ok {
			t.Fatalf("failure %d: user must not be locked", i)
		}
	}

	// 第 3 次失败（= M 次）：触发锁定。
	if dur, locked := g.RecordLoginFailure(user); !locked {
		t.Fatal("3rd failure must trigger lockout")
	} else if dur != 300*time.Second {
		t.Fatalf("lock duration should be 300s, got %v", dur)
	}

	// 锁定窗口内：用户名维度被拒。
	if retryAfter, ok := g.AllowUser(user); ok {
		t.Fatal("locked user must be rejected")
	} else if retryAfter <= 0 || retryAfter > 300*time.Second {
		t.Fatalf("retry-after must be within lock window, got %v", retryAfter)
	}

	// 锁定到期自动解锁。
	clk.advance(301 * time.Second)
	if _, ok := g.AllowUser(user); !ok {
		t.Fatal("after lock expires the user should be allowed again")
	}
}

func TestGuard_UserLockout_IndependentPerUser(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{RatePerMinute: 100, MaxFailures: 2}, clk.now)

	g.RecordLoginFailure("alice")
	g.RecordLoginFailure("alice") // alice 锁定
	if _, ok := g.AllowUser("alice"); ok {
		t.Fatal("alice should be locked")
	}
	// bob 不受影响。
	if _, ok := g.AllowUser("bob"); !ok {
		t.Fatal("bob must be independent of alice")
	}
}

func TestGuard_SuccessResetsFailures(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{RatePerMinute: 100, MaxFailures: 2}, clk.now)
	const user = "carol"

	// 1 次失败，然后成功清零。
	g.RecordLoginFailure(user)
	g.RecordLoginSuccess(user)
	// 清零后只 1 次失败：计数应为 1 (< 2)，不锁定 —— 证明成功把计数归零。
	if _, locked := g.RecordLoginFailure(user); locked {
		t.Fatal("after a success resets the counter, a single fresh failure must not lock")
	}
	// 第 2 次失败（清零后的第 2 次）才锁定 —— 证明计数从 0 重新累计。
	if _, locked := g.RecordLoginFailure(user); !locked {
		t.Fatal("the 2nd post-reset failure must lock (counter reached MaxFailures)")
	}
}

func TestGuard_DefaultsApplied(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{}, clk.now) // 全零 → 内置默认
	if g.ratePerMinute != defaultRatePerMinute {
		t.Fatalf("default rate should be %d, got %d", defaultRatePerMinute, g.ratePerMinute)
	}
	if g.maxFailures != defaultMaxFailures {
		t.Fatalf("default failures should be %d, got %d", defaultMaxFailures, g.maxFailures)
	}
	if g.lockDuration != defaultLockSeconds*time.Second {
		t.Fatalf("default lock should be %ds, got %v", defaultLockSeconds, g.lockDuration)
	}
}

// TestGuard_ConcurrentAccess 验证并发安全（race detector 跑这个）。
func TestGuard_ConcurrentAccess(t *testing.T) {
	clk := newFakeClock()
	g := NewGuard(GuardOptions{RatePerMinute: 50, MaxFailures: 3}, clk.now)

	done := make(chan struct{})
	const goroutines = 8
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			ip := "10.0.0.1"
			user := "alice"
			if n%2 == 0 {
				g.AllowIP(ip)
			}
			g.RecordLoginFailure(user)
			g.AllowUser(user)
			g.RecordLoginSuccess(user)
			g.AllowIP("10.0.0.2")
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
