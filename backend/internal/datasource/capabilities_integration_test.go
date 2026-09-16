//go:build integration

package datasource

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// 本文件在真实 PostgreSQL 上验证 Task 3-0 的方言能力懒探针（plan §6 gate：
// "探针必须在真实数据库实例上运行，不能只靠代码推断"）：
//   - 三条探针 SQL（GROUPING SETS / percentile_cont / 窗口函数）在 PG 12+ 全成功，
//     Capabilities() 返回 {true, true, true, "percentile_cont"}；
//   - sync.Once 缓存生效：连续两次调用返回同一 *DialectCapabilities 指针（未重探）。
//
// 安全约束（binding）：只读、零 DDL、零写入——探针 SQL 自带内联 (VALUES(1))/(SELECT 1)
// 数据；不跑 migrations、不建表。凭证只从 TEST_DATABASE_URL 读取，禁止硬编码。
// 约定照抄 query/pivot_correctness_integration_test.go（Task 2-4）。

// openCapabilitiesTestConn 用生产 PostgreSQL 驱动连接 TEST_DATABASE_URL 指向的真实库；
// 未设置时跳过。
func openCapabilitiesTestConn(t *testing.T) Connection {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 5432 // PG 默认端口
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
	drv, err := NewDriver(DriverPostgreSQL)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	conn, err := drv.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect (is the dev DB reachable?): %v", err)
	}
	ctx := context.Background()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping (is the dev DB reachable/auth ok?): %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestCapabilitiesProbe_RealPG 连真实 PG 跑能力探针，断言 PG 12+ 的实测真值。
func TestCapabilitiesProbe_RealPG(t *testing.T) {
	conn := openCapabilitiesTestConn(t)
	ctx := context.Background()

	// 记录真实服务器版本作为实测证据（只读）。
	if vr, err := conn.Execute(ctx, `SELECT version()`); err == nil && len(vr.Rows) > 0 {
		for _, v := range vr.Rows[0] {
			t.Logf("server version: %v", v)
			break
		}
	}

	got, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("Capabilities() returned error: %v", err)
	}
	if got == nil {
		t.Fatal("Capabilities() returned nil")
	}
	want := DialectCapabilities{
		SupportsGroupingSets:    true,
		SupportsPercentileCont:  true,
		SupportsWindowFunctions: true,
		PercentileStrategy:      "percentile_cont",
	}
	if *got != want {
		t.Errorf("Capabilities() = %+v, want %+v (probe SQLs should all succeed on PG 12+)", *got, want)
	}
}

// TestCapabilitiesProbe_RealPG_Cached 断言懒探针缓存生效：第二次调用返回同一
// *DialectCapabilities 指针（sync.Once，未重新探针）。
func TestCapabilitiesProbe_RealPG_Cached(t *testing.T) {
	conn := openCapabilitiesTestConn(t)
	ctx := context.Background()

	got1, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("first Capabilities() returned error: %v", err)
	}
	got2, err := conn.Capabilities(ctx)
	if err != nil {
		t.Fatalf("second Capabilities() returned error: %v", err)
	}
	if got1 != got2 {
		t.Errorf("expected cached pointer identity across calls (sync.Once), got %p vs %p", got1, got2)
	}
}
