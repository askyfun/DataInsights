//go:build integration

package datasource

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// 本文件在真实 MySQL / StarRocks 实例上验证"只升不降 + nil 保底"探针的行为
// （plan §6 gate：探针须在真实库运行，不能只靠代码推断）。与
// capabilities_integration_test.go（PG）同构，凭证只从 env 读取，未设置即跳过。
//
// 断言的是**探针语义正确性**，而非硬编码版本真值：
//   - 不变式：探针绝不把基线"降下去"（MySQL grouping 恒 false / percentile 恒 unsupported；
//     StarRocks percentile 恒 args_first，2026-09-19 实测值作基线）；
//   - 升级路径：MySQL 8+ → SupportsWindowFunctions=true；StarRocks 的 GROUPING SETS /
//     窗口能力按实测服务器记录（不同大版本支持度不同，故不断言固定值，只断言"探针不 panic
//     + 结果合法"）。
//   - 缓存：两次 Capabilities() 返回同一指针（sync.Once）。
//
// 安全：只读、零 DDL、零写入——探针 SQL 自带内联 (SELECT 1) 数据。

// openNonPGTestConn 用生产驱动连接 env 指定的实例；未设置则 skip。
// DSN 形如 mysql://user:pass@host:port/db 或 starrocks://user:pass@host:port/db。
func openNonPGTestConn(t *testing.T, dt DriverType, envVar string, defaultPort int) Connection {
	t.Helper()
	dsn := os.Getenv(envVar)
	if dsn == "" {
		t.Skipf("%s not set; skipping %s capabilities probe integration test", envVar, dt)
	}
	// url.Parse 要求 scheme 为已知小写；starrocks:// 非注册 scheme 但 Parse 仍可行。
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", envVar, err)
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = defaultPort
	}
	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	cfg := ConnectionConfig{
		Host:         u.Hostname(),
		Port:         port,
		DatabaseName: strings.TrimPrefix(u.Path, "/"),
		Username:     user,
		Password:     pass,
	}
	drv, err := NewDriver(dt)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	ctx := context.Background()
	conn, err := drv.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect (is the %s instance reachable?): %v", dt, err)
	}
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping (auth ok?): %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// logServerVersion 记录真实服务器版本作为实测证据（只读，失败不致命）。
func logServerVersion(t *testing.T, conn Connection, sql string) {
	t.Helper()
	if vr, err := conn.Execute(context.Background(), sql); err == nil && len(vr.Rows) > 0 {
		for _, v := range vr.Rows[0] {
			t.Logf("server version: %v", v)
			break
		}
	}
}

// TestCapabilitiesProbe_RealMySQL 在真实 MySQL 上验证探针：
//   - grouping=false、percentile=unsupported（文档事实，探针不得改）；
//   - 版本感知：解析 VERSION()，8+ 断言 window=true，5.7 断言 window=false。
func TestCapabilitiesProbe_RealMySQL(t *testing.T) {
	conn := openNonPGTestConn(t, DriverMySQL, "TEST_MYSQL_URL", 3306)
	ctx := context.Background()
	logServerVersion(t, conn, "SELECT VERSION()")

	got, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities() error: %v", err)
	}
	if got == nil {
		t.Fatal("Capabilities() returned nil")
	}
	// 不变式：这两项由文档钉死，探针**不应**改动它们。
	if got.SupportsGroupingSets {
		t.Errorf("MySQL must report SupportsGroupingSets=false (无 GROUPING SETS，仅 WITH ROLLUP)")
	}
	if got.PercentileStrategy != "unsupported" {
		t.Errorf("MySQL PercentileStrategy = %q, want unsupported (无标量 percentile 路径)", got.PercentileStrategy)
	}

	// 版本感知的窗口断言。
	var major int
	if vr, err := conn.Execute(ctx, "SELECT VERSION()"); err == nil && len(vr.Rows) > 0 {
		for _, v := range vr.Rows[0] {
			major = parseMajorVersion(toStr(v))
			break
		}
	}
	switch {
	case major >= 8 && !got.SupportsWindowFunctions:
		t.Errorf("MySQL %d+ 应支持窗口函数，探针未升 SupportsWindowFunctions=true", major)
	case major > 0 && major < 8 && got.SupportsWindowFunctions:
		t.Errorf("MySQL %d.x 不支持窗口函数，探针不应置 SupportsWindowFunctions=true", major)
	}
}

// TestCapabilitiesProbe_RealMySQL_Cached 断言 sync.Once 缓存：两次调用同一指针。
func TestCapabilitiesProbe_RealMySQL_Cached(t *testing.T) {
	conn := openNonPGTestConn(t, DriverMySQL, "TEST_MYSQL_URL", 3306)
	ctx := context.Background()
	a, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("first Capabilities(): %v", err)
	}
	b, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("second Capabilities(): %v", err)
	}
	if a != b {
		t.Errorf("expected cached pointer (sync.Once), got %p vs %p", a, b)
	}
}

// TestCapabilitiesProbe_RealStarRocks 在真实 StarRocks 上验证探针：
//   - 不变式：percentile 恒 args_first（2026-09-19 实测基线，探针不得降级）；
//   - GROUPING SETS / 窗口按版本记录（不断言固定值，只保证探针不 panic、结果合法）。
func TestCapabilitiesProbe_RealStarRocks(t *testing.T) {
	conn := openNonPGTestConn(t, DriverStarRocks, "TEST_STARROCKS_URL", 9030)
	ctx := context.Background()
	logServerVersion(t, conn, "SELECT VERSION()")

	got, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities() error: %v", err)
	}
	if got == nil {
		t.Fatal("Capabilities() returned nil")
	}
	if got.PercentileStrategy != "percentile_cont_args_first" {
		t.Errorf("StarRocks PercentileStrategy = %q, want percentile_cont_args_first（已实测基线，探针只升不降不得改动）", got.PercentileStrategy)
	}
	if !got.SupportsPercentileCont {
		t.Error("StarRocks SupportsPercentileCont 应为 true（基线已实测）")
	}
	// grouping/window 是版本相关的升级项：记录实测值，供翻转基线时参考（不断言）。
	slog.Info("starrocks probed caps", "groupingSets", got.SupportsGroupingSets, "window", got.SupportsWindowFunctions)
}

// TestCapabilitiesProbe_RealStarRocks_Cached 断言 sync.Once 缓存：两次调用同一指针。
func TestCapabilitiesProbe_RealStarRocks_Cached(t *testing.T) {
	conn := openNonPGTestConn(t, DriverStarRocks, "TEST_STARROCKS_URL", 9030)
	ctx := context.Background()
	a, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("first Capabilities(): %v", err)
	}
	b, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("second Capabilities(): %v", err)
	}
	if a != b {
		t.Errorf("expected cached pointer (sync.Once), got %p vs %p", a, b)
	}
}

// parseMajorVersion 从 "8.0.36" / "5.7.42-log" 之类字符串取主版本号；解析失败返回 0。
func parseMajorVersion(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "."); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// toStr 把 Execute 结果里的 interface{} 值转字符串（探针仅用于日志/版本解析）。
func toStr(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
