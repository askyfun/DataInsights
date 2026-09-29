package chart

import (
	"reflect"
	"testing"
)

// normalizePersistedQueryOptions 把持久化文档 queryOptions 小节（camelCase 惯例）
// 归一为 wire snake_case。Top N（#130）的 topN → top_n 曾因漏加映射，
// 导致「builder 里生效、分享页/仪表盘静默不生效」——本用例钉死这条回归。
func TestNormalizePersistedQueryOptions(t *testing.T) {
	if got := normalizePersistedQueryOptions(nil); got != nil {
		t.Errorf("empty section: %+v", got)
	}
	if got := normalizePersistedQueryOptions(map[string]any{}); got != nil {
		t.Errorf("empty map: %+v", got)
	}
	in := map[string]any{
		"binCount": 15,
		"binWidth": 2.5,
		"topN":     map[string]any{"limit": 10, "metric": "c2"},
		"referenceLines": []any{},
	}
	got := normalizePersistedQueryOptions(in)
	for _, key := range []string{"bin_count", "bin_width", "top_n"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing normalized key %q: %+v", key, got)
		}
	}
	if _, ok := got["topN"]; ok {
		t.Error("camelCase topN must be replaced by top_n")
	}
	// 未知键原样保留（扩展袋语义）
	if _, ok := got["referenceLines"]; !ok {
		t.Error("unknown keys must pass through")
	}
	// 已是 snake_case 的输入不重复翻译
	snake := normalizePersistedQueryOptions(map[string]any{"top_n": map[string]any{"limit": 3}})
	if !reflect.DeepEqual(snake["top_n"], map[string]any{"limit": 3}) {
		t.Errorf("snake passthrough: %+v", snake)
	}

	// 归一只改**顶层**键：top_n 内部的 camelCase mergeOther（issue #116「其余合并为
	// 其他」）必须原样留着——分享页/仪表盘这两条读持久化文档的链路不经
	// composeChartQueryRequest，没人把嵌套键翻成 snake_case。下游
	// query.parseTopNMergeOther 两个拼法都认（它的用例在 query 包的
	// TestParseTopN_MergeOther），这条与那条合起来才是「保存后分享页也生效」。
	merged := normalizePersistedQueryOptions(map[string]any{
		"topN": map[string]any{"limit": 5, "mergeOther": true},
	})
	section, ok := merged["top_n"].(map[string]any)
	if !ok {
		t.Fatalf("top_n section lost: %+v", merged)
	}
	if v, ok := section["mergeOther"].(bool); !ok || !v {
		t.Errorf("nested camelCase mergeOther must survive normalization: %+v", section)
	}
}
