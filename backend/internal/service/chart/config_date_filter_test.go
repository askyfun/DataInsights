package chart

import (
	"fmt"
	"testing"
	"time"

	"data-insights/internal/model"
)

// 日期筛选意图在**取数时**被现算（而不是用保存时写下的快照）。
//
// 为什么这是关键：分享页与仪表盘的取数是后端自己读 config 组查询的，前端没参与。
// 这些用例钉住「读 config 的路径也会跟着时间走」—— 没有它们，仪表盘上挂了三周的
// 「最近 7 天」会一直显示三周前那七天，且全程无报错。

const dateDayLayout = "2006-01-02"

// configWithDateFilter 造一份 v2 config，其中一条过滤条件带日期意图。
// operator/value/value_end 故意填成**过期快照**（2020 年），用来证明取数没有用它。
// 必须给一个维度绑定：v2 解析契约里「维度与指标同时为空」视为读不懂的文档（ok=false）。
func configWithDateFilter(intentJSON string) string {
	return fmt.Sprintf(
		`{"version":2,"chartType":"table","query":{"dimensionGroups":[{"id":"dims",`+
			`"bindings":[{"bindingId":"b-0","fieldId":"c-date"}]}],"metricGroups":[],`+
			`"filters":[{"fieldId":"c-date","operator":"between","value":"2020-01-01","value_end":"2020-01-02",`+
			`"logic":"and","date":%s}]},"fieldMeta":{}}`,
		intentJSON,
	)
}

func queryFromConfig(t *testing.T, config string) []string {
	t.Helper()
	chart := &model.Chart{ID: 7, DatasetID: 10, ChartType: "table", Config: config}
	req, ok := chartDataQueryFromConfig(chart)
	if !ok {
		t.Fatal("expected v2 config to parse (ok=true), got ok=false")
	}
	// 返回每条条件的「算子|值|值末|logic」便于一次性断言整串形状。
	out := make([]string, 0, len(req.Filters))
	for _, f := range req.Filters {
		out = append(out, fmt.Sprintf("%s|%v|%v|%s", f.Operator, f.Value, f.ValueEnd, f.Logic))
	}
	return out
}

// TestConfigDateIntentResolvesAtQueryTime 动态日期必须现算，快照被忽略。
// 断言用「区间跨度 + 上界是昨天」而不是写死日期：跨零点跑也不会闪。
func TestConfigDateIntentResolvesAtQueryTime(t *testing.T) {
	chart := &model.Chart{
		ID: 7, DatasetID: 10, ChartType: "table",
		Config: configWithDateFilter(
			`{"value":{"kind":"dynamic","preset":"last7d"},"granularity":"day","weekStart":1,"withTime":false}`,
		),
	}
	req, ok := chartDataQueryFromConfig(chart)
	if !ok {
		t.Fatal("expected v2 config to parse (ok=true), got ok=false")
	}
	if len(req.Filters) != 1 {
		t.Fatalf("期望 1 条条件，实际 %+v", req.Filters)
	}

	got := req.Filters[0]
	if got.Operator != "between" {
		t.Fatalf("operator = %q, want between（快照也是 between，故同时看下面的值）", got.Operator)
	}
	start, okStart := parseDayValue(got.Value)
	end, okEnd := parseDayValue(got.ValueEnd)
	if !okStart || !okEnd {
		t.Fatalf("值应为日期串，实际 %#v / %#v", got.Value, got.ValueEnd)
	}
	if span := int(end.Sub(start).Hours()/24) + 1; span != 7 {
		t.Errorf("「最近 7 天」应覆盖 7 个自然日，实际 %d", span)
	}
	wantEnd := time.Now().AddDate(0, 0, -1).Format(dateDayLayout)
	if end.Format(dateDayLayout) != wantEnd {
		t.Errorf("区间上界应为昨天 %s，实际 %s（说明用的是快照而不是现算）", wantEnd, end.Format(dateDayLayout))
	}
	// 快照是 2020 年：只要结果还是 2020，就说明这条路径根本没走意图解析。
	if start.Year() == 2020 {
		t.Errorf("区间落在快照年份 2020，日期意图未被解析")
	}
}

// TestConfigDateIntentUnselectedDropsCondition 未选择的日期条件整条丢弃。
// 刻意**不回落** operator/value 里的快照：未选择时快照是 `eq ”`，回落会把
// 「不过滤」变成「查出 0 行」。
func TestConfigDateIntentUnselectedDropsCondition(t *testing.T) {
	cases := map[string]string{
		"未选择":     `{"value":{"kind":"dynamic"},"granularity":"day","weekStart":1}`,
		"所有日期":    `{"value":{"kind":"special","value":"all"},"granularity":"day","weekStart":1}`,
		"起止无限制":   `{"value":{"kind":"advanced","start":{"type":"unlimited"},"end":{"type":"unlimited"}},"granularity":"day","weekStart":1}`,
		"畸形 kind": `{"value":{"kind":"nope"},"granularity":"day","weekStart":1}`,
	}
	for name, intent := range cases {
		t.Run(name, func(t *testing.T) {
			if filters := queryFromConfig(t, configWithDateFilter(intent)); len(filters) != 0 {
				t.Fatalf("期望不下发任何条件，实际 %v", filters)
			}
		})
	}
}

// TestConfigDateIntentSpecialAndSingleShapes 特殊值与单侧无限制的算子形状。
//
// ⚠️ 钉住一条契约：**单侧边界一律放在 Value**（`buildFilterPart` 对 gte/lte 只绑 Value），
// `ValueEnd` 只给 between 的第二端用。放错位置会让 lte 渲染成 `col <= NULL`（恒假，0 行）。
func TestConfigDateIntentSpecialAndSingleShapes(t *testing.T) {
	cases := []struct {
		name      string
		intent    string
		wantOp    string
		wantValue any
	}{
		{
			name:      "空日期 → isNull（Value 不参与）",
			intent:    `{"value":{"kind":"special","value":"empty"},"granularity":"day","weekStart":1}`,
			wantOp:    "isNull",
			wantValue: "",
		},
		{
			name:      "非空日期 → isNotNull",
			intent:    `{"value":{"kind":"special","value":"notEmpty"},"granularity":"day","weekStart":1}`,
			wantOp:    "isNotNull",
			wantValue: "",
		},
		{
			name:      "高级·结束无限制 → gte 且下界放 Value",
			intent:    `{"value":{"kind":"advanced","start":{"type":"fixed","value":"2023-06-14"},"end":{"type":"unlimited"}},"granularity":"day","weekStart":1}`,
			wantOp:    "gte",
			wantValue: "2023-06-14",
		},
		{
			name:      "高级·开始无限制 → lte 且上界放 Value",
			intent:    `{"value":{"kind":"advanced","start":{"type":"unlimited"},"end":{"type":"fixed","value":"2023-06-14"}},"granularity":"day","weekStart":1}`,
			wantOp:    "lte",
			wantValue: "2023-06-14",
		},
		{
			name:      "单个日期 → between，两端各就各位",
			intent:    `{"value":{"kind":"single","date":"2026-05-15"},"granularity":"day","weekStart":1}`,
			wantOp:    "between",
			wantValue: "2026-05-15",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chart := &model.Chart{ID: 7, DatasetID: 10, ChartType: "table", Config: configWithDateFilter(tc.intent)}
			req, ok := chartDataQueryFromConfig(chart)
			if !ok || len(req.Filters) != 1 {
				t.Fatalf("期望 1 条条件，实际 ok=%v filters=%v", ok, req.Filters)
			}
			got := req.Filters[0]
			if got.Operator != tc.wantOp {
				t.Errorf("operator = %q, want %q", got.Operator, tc.wantOp)
			}
			if got.Value != tc.wantValue {
				t.Errorf("Value = %#v, want %#v", got.Value, tc.wantValue)
			}
			// 除 between 外都不该有第二端：给了 ValueEnd 会让单侧算子渲染成 `col <= ? AND ?`。
			if tc.wantOp != "between" && got.ValueEnd != nil {
				t.Errorf("非 between 算子不应带 ValueEnd: %+v", got)
			}
			if tc.wantOp == "between" && got.ValueEnd == nil {
				t.Errorf("between 必须带 ValueEnd: %+v", got)
			}
		})
	}
}

// TestConfigDateIntentIncludeEmptyAppendsOrCondition 「包含空日期」追加一条 IS NULL 并用 OR 连接，
// 与前面的区间构成 `区间 OR IS NULL`。
func TestConfigDateIntentIncludeEmptyAppendsOrCondition(t *testing.T) {
	filters := queryFromConfig(t, configWithDateFilter(
		`{"value":{"kind":"fixed","start":"2026-08-01","end":"2026-08-31","includeEmpty":true},"granularity":"day","weekStart":1}`,
	))

	if len(filters) != 2 {
		t.Fatalf("期望 2 条条件（区间 + IS NULL），实际 %v", filters)
	}
	if filters[0] != "between|2026-08-01|2026-08-31|and" {
		t.Errorf("主条件形状错误: %s", filters[0])
	}
	if filters[1] != "isNull||<nil>|or" {
		t.Errorf("附加条件应为 isNull 且按 or 连接: %s", filters[1])
	}
}

// TestConfigFilterWithoutDateIntentPassesThrough 没有日期意图的条件原样透传
// （历史图表：只在 config 里存了已算好的区间）。
func TestConfigFilterWithoutDateIntentPassesThrough(t *testing.T) {
	config := `{"version":2,"chartType":"table","query":{"dimensionGroups":[{"id":"dims",` +
		`"bindings":[{"bindingId":"b-0","fieldId":"c-date"}]}],"metricGroups":[],` +
		`"filters":[{"fieldId":"c-region","operator":"eq","value":"华北","logic":"and"}]},"fieldMeta":{}}`

	filters := queryFromConfig(t, config)
	if len(filters) != 1 || filters[0] != "eq|华北|<nil>|and" {
		t.Fatalf("期望原样透传，实际 %v", filters)
	}
}

func parseDayValue(v any) (time.Time, bool) {
	text, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation(dateDayLayout, text, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
