package handler

import (
	"testing"

	"tablewave/internal/domain/entity"
)

// TestValidateColumnHierarchy 覆盖层级引用校验（issue #136）：
// 合法引用通过；自引用（parent_id==id / next_id==id）拒绝；引用未知列 id 拒绝；
// 空串（层级根/叶子哨兵）通过；nil hierarchy 通过。
func TestValidateColumnHierarchy(t *testing.T) {
	cols := []entity.DatasetColumn{
		{ID: "prov", Name: "省份"},
		{ID: "city", Name: "城市"},
		{ID: "district", Name: "区县"},
	}

	tests := []struct {
		name    string
		col     entity.DatasetColumn
		wantErr bool
	}{
		{"nil hierarchy ok", entity.DatasetColumn{ID: "x", Name: "X"}, false},
		{"empty parent ok (root)", entity.DatasetColumn{ID: "prov", Name: "省份", Hierarchy: &entity.ColumnHierarchy{NextID: "city"}}, false},
		{"empty next ok (leaf)", entity.DatasetColumn{ID: "district", Name: "区县", Hierarchy: &entity.ColumnHierarchy{ParentID: "city"}}, false},
		{"valid chain", entity.DatasetColumn{ID: "city", Name: "城市", Hierarchy: &entity.ColumnHierarchy{ParentID: "prov", NextID: "district"}}, false},
		{"parent self ref rejected", entity.DatasetColumn{ID: "prov", Name: "省份", Hierarchy: &entity.ColumnHierarchy{ParentID: "prov"}}, true},
		{"next self ref rejected", entity.DatasetColumn{ID: "prov", Name: "省份", Hierarchy: &entity.ColumnHierarchy{NextID: "prov"}}, true},
		{"parent unknown id rejected", entity.DatasetColumn{ID: "city", Name: "城市", Hierarchy: &entity.ColumnHierarchy{ParentID: "ghost"}}, true},
		{"next unknown id rejected", entity.DatasetColumn{ID: "city", Name: "城市", Hierarchy: &entity.ColumnHierarchy{NextID: "ghost"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateColumnHierarchy(tt.col, cols)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateColumnHierarchy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateColumnPayload_HierarchyWired 验证 validateColumnPayload 确实调用了
// 层级校验（自引用在整体 payload 校验阶段被拦截）。
func TestValidateColumnPayload_HierarchyWired(t *testing.T) {
	cols := []entity.DatasetColumn{
		{ID: "prov", Name: "省份", Hierarchy: &entity.ColumnHierarchy{NextID: "prov"}}, // 自环
		{ID: "city", Name: "城市"},
	}
	if err := validateColumnPayload(cols); err == nil {
		t.Fatal("validateColumnPayload() expected hierarchy self-ref error, got nil")
	}
}
