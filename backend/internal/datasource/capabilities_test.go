package datasource

import (
	"context"
	"testing"
)

// TestCapabilitiesStaticDrivers 用 zero-value 连接（db/conn=nil）钉死 3 个非 PG 驱动
// Capabilities() 的**基线返回**（nil 保底路径），等价于"无实例"时的行为：
//   - ClickHouse：**刻意保持静态**（无实例可安全只升不降探测的布尔能力，且 percentile
//     需语义而非语法校验）——grouping-sets/window=true（文档化 CH 21.x+ 事实，失败 loud），
//     PercentileStrategy 保守 "unsupported"。
//   - MySQL/StarRocks：已升级为懒探针（见 probeCapabilities），但**只升不降 + nil 保底**，
//     故 zero-value 连接返回的正是探针未运行时的基线静态值——MySQL 全保守、StarRocks
//     percentile=args_first（2026-09-19 实测值作为基线）。接真实实例时探针只会把 false
//     升 true，不会低于本基线，故本测试同时锁定"无实例行为不回归"。
//
// PostgreSQL 不在此：其探针由 capabilities_integration_test.go（//go:build integration）连真实 PG 验证。
func TestCapabilitiesStaticDrivers(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		conn Connection
		want DialectCapabilities
	}{
		{
			name: "clickhouse",
			conn: &clickhouseConnection{},
			want: DialectCapabilities{
				SupportsGroupingSets:    true,
				SupportsPercentileCont:  false,
				SupportsWindowFunctions: true,
				PercentileStrategy:      "unsupported",
			},
		},
		{
			name: "mysql",
			conn: &mysqlConnection{},
			want: DialectCapabilities{
				SupportsGroupingSets:    false,
				SupportsPercentileCont:  false,
				SupportsWindowFunctions: false,
				PercentileStrategy:      "unsupported",
			},
		},
		{
			name: "starrocks",
			conn: &starRocksConnection{},
			want: DialectCapabilities{
				SupportsGroupingSets:    false,
				SupportsPercentileCont:  true,
				SupportsWindowFunctions: false,
				PercentileStrategy:      "percentile_cont_args_first",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.conn.Capabilities(ctx)
			if err != nil {
				t.Fatalf("Capabilities() returned error: %v", err)
			}
			if got == nil {
				t.Fatal("Capabilities() returned nil")
			}
			if *got != tt.want {
				t.Errorf("Capabilities() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

// TestPostgresCapabilitiesNilPoolConservative 钉死 PG nil-pool 兜底：zero-value
// &postgresqlConnection{}（pool=nil，生产路径不会出现，仅为杜绝误用 panic）调
// Capabilities() 必须不 panic 且返回保守默认值（全 false + "unsupported"）。
func TestPostgresCapabilitiesNilPoolConservative(t *testing.T) {
	got, err := (&postgresqlConnection{}).Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() returned error: %v", err)
	}
	want := DialectCapabilities{PercentileStrategy: "unsupported"}
	if *got != want {
		t.Errorf("Capabilities() with nil pool = %+v, want conservative %+v", *got, want)
	}
}
