package query

import (
	"math"
	"strings"
	"testing"

	"dataray/internal/datasource"
)

// percentile_test.go — percentile 原语（R-54）单测。覆盖：
//   - percentile_cont 各 p 的 SQL 形态（含 0.5/0.25/0.75 精确格式，%.4g 不产尾零）；
//   - 其他策略全部**显式错误、不静默近似**（plan §4.2）；
//   - p 边界（0/1 允许、-0.1/1.1/NaN 拒绝、caps==nil 拒绝）；
//   - AstRequiresPercentile 判定（含/不含 median 指标、nil ast）；
//   - CheckPercentileSupport 与 executor 前置门语义一致。

func TestBuildPercentileExpr_Strategies(t *testing.T) {
	for _, tc := range []struct {
		name        string
		field       string
		p           float64
		caps        *datasource.DialectCapabilities
		wantExpr    string
		wantErrSubs string // 期望 error 文案子串；空表示无错
	}{
		{
			name:     "percentile_cont p=0.5 中位数",
			field:    `"amount"`,
			p:        0.5,
			caps:     &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantExpr: `percentile_cont(0.5) WITHIN GROUP (ORDER BY "amount")`,
		},
		{
			name:     "percentile_cont p=0.25 Q1",
			field:    `x`,
			p:        0.25,
			caps:     &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantExpr: `percentile_cont(0.25) WITHIN GROUP (ORDER BY x)`,
		},
		{
			name:     "percentile_cont p=0.75 Q3",
			field:    `x`,
			p:        0.75,
			caps:     &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantExpr: `percentile_cont(0.75) WITHIN GROUP (ORDER BY x)`,
		},
		{
			name:     "percentile_cont p=0（下界合法）",
			field:    `x`,
			p:        0,
			caps:     &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantExpr: `percentile_cont(0) WITHIN GROUP (ORDER BY x)`,
		},
		{
			name:     "percentile_cont p=1（上界合法）",
			field:    `x`,
			p:        1,
			caps:     &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantExpr: `percentile_cont(1) WITHIN GROUP (ORDER BY x)`,
		},
		{
			name:        "unsupported 策略显式报错（CH/MySQL/StarRocks 现状，Task 3-0）",
			field:       `x`,
			p:           0.5,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "unsupported"},
			wantErrSubs: "not supported",
		},
		{
			name:        "空策略视为不支持",
			field:       `x`,
			p:           0.5,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: ""},
			wantErrSubs: "not supported",
		},
		{
			name:        "quantilesExactInclusive 声明但未实现（防脱节）",
			field:       `x`,
			p:           0.5,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "quantilesExactInclusive"},
			wantErrSubs: "not yet implemented",
		},
		{
			name:        "window_ntile 声明但未实现（防脱节）",
			field:       `x`,
			p:           0.5,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "window_ntile"},
			wantErrSubs: "not yet implemented",
		},
		{
			name:        "未知策略视为配置错误",
			field:       `x`,
			p:           0.5,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "bogus"},
			wantErrSubs: "unknown",
		},
		{
			name:        "caps 为 nil",
			field:       `x`,
			p:           0.5,
			caps:        nil,
			wantErrSubs: "nil",
		},
		{
			name:        "p 下界外（<0）",
			field:       `x`,
			p:           -0.1,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantErrSubs: "p must be in [0,1]",
		},
		{
			name:        "p 上界外（>1）",
			field:       `x`,
			p:           1.1,
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantErrSubs: "p must be in [0,1]",
		},
		{
			name:        "p 为 NaN 被拒",
			field:       `x`,
			p:           math.NaN(),
			caps:        &datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"},
			wantErrSubs: "p must be in [0,1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildPercentileExpr(tc.field, tc.p, tc.caps)
			if tc.wantErrSubs != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got expr=%q", tc.wantErrSubs, got)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubs) {
					t.Errorf("error %q missing substring %q", err.Error(), tc.wantErrSubs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantExpr {
				t.Errorf("expr = %q, want %q", got, tc.wantExpr)
			}
		})
	}
}

// TestAstRequiresPercentile 覆盖 AST 判定：nil / 无 median / 有 median 三态。
// 无 median 时 executor 不能触碰 Capabilities()——这是零开销保证（DoD 5）。
func TestAstRequiresPercentile(t *testing.T) {
	if AstRequiresPercentile(nil) {
		t.Error("nil ast → false")
	}
	if AstRequiresPercentile(&QueryAST{Metrics: []MetricExpr{{Agg: AggSum}, {Agg: AggAvg}}}) {
		t.Error("无 median 指标 → false")
	}
	if !AstRequiresPercentile(&QueryAST{Metrics: []MetricExpr{{Agg: AggSum}, {Agg: AggMedian}}}) {
		t.Error("含 median 指标 → true")
	}
}

// TestCheckPercentileSupport 覆盖 executor 前置门语义：pg percentile_cont → nil；
// 其余（含 CH/MySQL/StarRocks 现状 unsupported）→ error。
func TestCheckPercentileSupport(t *testing.T) {
	if err := CheckPercentileSupport(&datasource.DialectCapabilities{PercentileStrategy: "percentile_cont"}); err != nil {
		t.Errorf("percentile_cont should pass, got %v", err)
	}
	if err := CheckPercentileSupport(&datasource.DialectCapabilities{PercentileStrategy: "unsupported"}); err == nil {
		t.Error("unsupported must fail (CH/MySQL/StarRocks 现状)")
	}
	if err := CheckPercentileSupport(nil); err == nil {
		t.Error("nil caps must fail")
	}
}

// TestRenderMetricSelect_Median 覆盖 builder 的 median 分支：产 percentile_cont(0.5)
// WITHIN GROUP (ORDER BY <safe field>) AS <quoted alias>，形态与 percentile_cont 契约一致。
// 用默认 NewBunQueryBuilder()（MySQL 方言：合法标识符裸出、别名反引号），与既有 builder 测试同口径。
func TestRenderMetricSelect_Median(t *testing.T) {
	qb := NewBunQueryBuilder()
	got := qb.renderMetricSelect(MetricExpr{Field: "amount", FieldExpr: "amount", Agg: AggMedian, Alias: "med"})
	want := "percentile_cont(0.5) WITHIN GROUP (ORDER BY amount) AS `med`"
	if got != want {
		t.Errorf("median SQL = %q, want %q", got, want)
	}
}
