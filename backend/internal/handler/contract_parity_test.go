package handler

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"data-insights/internal/domain/entity"
)

// jsonTagTypes 收集结构体声明的全部 json 字段名 → 声明类型（忽略无 tag、
// "-" 与匿名 tag）。记录前剥离指针层：实体侧可选元数据用 *string（区分
// null 与 ""），body 镜像侧用裸 string（"" 即未提供、handler 保留存量），
// 两者序列化同一 wire 类型，不算漂移。
func jsonTagTypes(v any) map[string]reflect.Type {
	out := map[string]reflect.Type{}
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
		ft := t.Field(i).Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		out[name] = ft
	}
	return out
}

// 迁移后的 handler 用本地镜像 struct 绑定 JSON body（共享领域实体不能携带
// form:"-" 标签，见 datasetQueryIn / chartCreateIn / chartQueryIn /
// dashboardCreateIn / dashboardUpdateIn / dashboardQueryIn / datasetCreateIn /
// datasetUpdateIn 的注释），镜像与实体靠人工同步。本测试是漂移防护，按镜像意图
// 分两档：
//
//  1. subset=false（精确副本）：datasetQueryIn / chartCreateIn /
//     chartQueryIn / dashboardCreateIn / dashboardUpdateIn / dashboardQueryIn
//     当前都是实体 json 表面的精确副本（无省略字段），断言 "集合相等"：任何一侧
//     新增/删除/改名 json 字段都会使本测试失败，除非同步修改另一侧或（若确属有意
//     裁剪）转为第 2 档并注明裁剪原因。
//  2. subset=true（有意裁剪）：datasetCreateIn / datasetUpdateIn 只承载
//     entity.Dataset 的客户端可编辑面——id/created_at/updated_at 为服务端
//     所有，quality_rules 由服务端默认 "[]"，accelerate_config /
//     refresh_strategy / preview_data 不对外开放编辑；PUT 还叠加"未提供的
//     可选元数据保留存量"的 merge 约定（见 handler.Update 注释）。故只断言
//     显式子集：镜像的每个 json tag 必须存在于实体且类型一致（按 jsonTagTypes
//     的指针剥离口径比较），实体多出的字段是设计而非漂移。
func TestMirrorEntityJSONTagParity(t *testing.T) {
	cases := []struct {
		name        string
		mirror, ent any
		subset      bool
	}{
		{"datasetQueryIn / entity.QueryConfig", datasetQueryIn{}, entity.QueryConfig{}, false},
		{"chartCreateIn / entity.Chart", chartCreateIn{}, entity.Chart{}, false},
		{"chartQueryIn / entity.ChartQueryRequest", chartQueryIn{}, entity.ChartQueryRequest{}, false},
		{"querySaveIn / entity.QueryRecordSaveRequest", querySaveIn{}, entity.QueryRecordSaveRequest{}, false},
		{"dashboardCreateIn / entity.DashboardCreateRequest", dashboardCreateIn{}, entity.DashboardCreateRequest{}, false},
		{"dashboardUpdateIn / entity.DashboardUpdateRequest", dashboardUpdateIn{}, entity.DashboardUpdateRequest{}, false},
		{"dashboardQueryIn / entity.DashboardQueryRequest", dashboardQueryIn{}, entity.DashboardQueryRequest{}, false},
		{"dashboardFolderCreateIn / entity.DashboardFolderCreateRequest", dashboardFolderCreateIn{}, entity.DashboardFolderCreateRequest{}, false},
		{"dashboardFolderUpdateIn / entity.DashboardFolderUpdateRequest", dashboardFolderUpdateIn{}, entity.DashboardFolderUpdateRequest{}, false},
		{"datasetCreateIn / entity.Dataset", datasetCreateIn{}, entity.Dataset{}, true},
		{"datasetUpdateIn / entity.Dataset", datasetUpdateIn{}, entity.Dataset{}, true},
	}
	for _, tc := range cases {
		mirrorTypes := jsonTagTypes(tc.mirror)
		entityTypes := jsonTagTypes(tc.ent)

		var missing, extra, mismatched []string
		for tag, mt := range mirrorTypes {
			et, ok := entityTypes[tag]
			if !ok {
				extra = append(extra, tag)
				continue
			}
			if tc.subset && et != mt {
				mismatched = append(mismatched, fmt.Sprintf("%s(mirror=%s entity=%s)", tag, mt, et))
			}
		}
		if !tc.subset {
			for tag := range entityTypes {
				if _, ok := mirrorTypes[tag]; !ok {
					missing = append(missing, tag)
				}
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		sort.Strings(mismatched)
		if len(missing) > 0 || len(extra) > 0 || len(mismatched) > 0 {
			t.Errorf("%s: json tag drift: mirror missing %v, mirror extra %v, type mismatch %v", tc.name, missing, extra, mismatched)
		}
	}
}
