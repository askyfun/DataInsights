package datasource

import (
	"context"
	"testing"
)

// TestCapabilitiesStubs 断言 4 个驱动的 Capabilities() 静态 stub 返回值符合
// plan §4.1 设计目标（stub 不依赖连接状态，直接用 zero-value struct 调用）。
func TestCapabilitiesStubs(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		conn Connection
		want DialectCapabilities
	}{
		{
			name: "postgresql",
			conn: &postgresqlConnection{},
			want: DialectCapabilities{
				SupportsGroupingSets:    true,
				SupportsPercentileCont:  true,
				SupportsWindowFunctions: true,
				PercentileStrategy:      "percentile_cont",
			},
		},
		{
			name: "clickhouse",
			conn: &clickhouseConnection{},
			want: DialectCapabilities{
				SupportsGroupingSets:    true,
				SupportsPercentileCont:  false,
				SupportsWindowFunctions: true,
				PercentileStrategy:      "quantilesExactInclusive",
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
				SupportsPercentileCont:  false,
				SupportsWindowFunctions: false,
				PercentileStrategy:      "unsupported",
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
