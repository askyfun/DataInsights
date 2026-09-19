package datasource

import (
	"context"
	"testing"
)

// TestCapabilitiesStaticDrivers 断言未接真实探针的 3 个驱动（CH/MySQL/StarRocks）的
// Capabilities() 静态返回值符合 Task 3-0 裁定：
//   - ClickHouse 按失败模式非对称处理：grouping-sets/window 保持 true（文档化 CH 21.x+
//     事实，失败是 loud SQL 报错）；PercentileStrategy 保守 "unsupported"（silent
//     wrong-data 风险，待真实实例探针验证 quantilesExactInclusive 后翻转）。
//   - MySQL/StarRocks 全保守默认（无实例未实测）。
//
// PostgreSQL 不在本测试内：Task 3-0 已把 PG 从静态 stub 升级为真实懒探针，由
// capabilities_integration_test.go（//go:build integration）连真实 PG 验证。
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
