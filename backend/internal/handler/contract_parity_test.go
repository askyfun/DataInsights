package handler

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"dataray/internal/domain/entity"
)

// jsonTagSet 收集结构体声明的全部 json 字段名（忽略无 tag、"-" 与匿名 tag）。
func jsonTagSet(v any) map[string]struct{} {
	out := map[string]struct{}{}
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

// 迁移后的 handler 用本地镜像 struct 绑定 JSON body（共享领域实体不能携带
// form:"-" 标签，见 datasetQueryIn / chartCreateIn / chartQueryIn 的注释），
// 镜像与实体靠人工同步。本测试是漂移防护：逐对断言镜像的 json tag 集合
// 与实体侧完全相等。
//
// 三个镜像当前都是实体 json 表面的精确副本（无省略字段），因此断言取"集合
// 相等"而非"子集"：任何一侧新增/删除/改名 json 字段都会使本测试失败，
// 除非同步修改另一侧或（若确属有意裁剪）在此处改为显式子集断言并注明原因。
func TestMirrorEntityJSONTagParity(t *testing.T) {
	cases := []struct {
		name        string
		mirror, ent any
	}{
		{"datasetQueryIn / entity.QueryConfig", datasetQueryIn{}, entity.QueryConfig{}},
		{"chartCreateIn / entity.Chart", chartCreateIn{}, entity.Chart{}},
		{"chartQueryIn / entity.ChartQueryRequest", chartQueryIn{}, entity.ChartQueryRequest{}},
	}
	for _, tc := range cases {
		mirrorTags := jsonTagSet(tc.mirror)
		entityTags := jsonTagSet(tc.ent)

		var missing, extra []string
		for tag := range entityTags {
			if _, ok := mirrorTags[tag]; !ok {
				missing = append(missing, tag)
			}
		}
		for tag := range mirrorTags {
			if _, ok := entityTags[tag]; !ok {
				extra = append(extra, tag)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s: json tag drift: mirror missing %v, mirror extra %v", tc.name, missing, extra)
		}
	}
}
