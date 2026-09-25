package chart

import (
	"testing"

	"data-insights/internal/domain/entity"
)

// 仪表盘筛选合并（PRD §8.3 / D5–D7）的单元测试。
//
// 规则：仪表盘有的字段 → 丢弃图表自身该字段的条件 + 追加仪表盘的条件；
// 仪表盘没有的字段 → 图表的条件原样保留。
// 另有两处不可退让的性质：输入请求不被就地修改；合并结果不回写任何持久化状态。

func filter(id, field, operator string, value any, logic string) entity.Filter {
	return entity.Filter{ID: id, Field: field, Operator: operator, Value: value, Logic: logic}
}

func requestWith(filters ...entity.Filter) *entity.ChartQueryRequest {
	return &entity.ChartQueryRequest{
		DatasetID: 1,
		ChartType: "bar",
		Dims:      []string{"region"},
		Filters:   filters,
	}
}

func fieldNames(filters []entity.Filter) []string {
	names := make([]string, 0, len(filters))
	for _, f := range filters {
		names = append(names, f.Field)
	}
	return names
}

func TestApplyFilterOverridesNilRequest(t *testing.T) {
	if got := applyFilterOverrides(nil, []entity.Filter{filter("f1", "region", "eq", "华东", "and")}); got != nil {
		t.Fatalf("nil 请求应原样返回 nil，得到 %+v", got)
	}
}

func TestApplyFilterOverridesEmptyOverridesIsNoop(t *testing.T) {
	req := requestWith(filter("c1", "region", "eq", "华东", "and"))

	got := applyFilterOverrides(req, nil)

	if got != req {
		t.Fatal("空 overrides 应短路返回入参本身（不做无谓拷贝）")
	}
	if got2 := applyFilterOverrides(req, []entity.Filter{}); got2 != req {
		t.Fatal("空切片 overrides 同样应短路")
	}
}

func TestApplyFilterOverridesNoCollisionKeepsChartFiltersAndAppends(t *testing.T) {
	req := requestWith(filter("c1", "order_date", "gte", "2026-01-01", "and"))

	got := applyFilterOverrides(req, []entity.Filter{
		filter("d1", "region", "eq", "华东", "and"),
	})

	if len(got.Filters) != 2 {
		t.Fatalf("期望 2 条条件，得到 %d 条: %+v", len(got.Filters), got.Filters)
	}
	if got.Filters[0].ID != "c1" {
		t.Errorf("图表自身条件应保留在首位，实际首位是 %q", got.Filters[0].ID)
	}
	if got.Filters[1].ID != "d1" {
		t.Errorf("仪表盘条件应追加在末位，实际末位是 %q", got.Filters[1].ID)
	}
}

func TestApplyFilterOverridesDropsChartConditionOnClaimedField(t *testing.T) {
	// 图表里 region = 华东（作者本意），仪表盘改选 region = 华南 → 覆盖。
	req := requestWith(
		filter("c-date", "order_date", "gte", "2026-01-01", "and"),
		filter("c-region", "region", "eq", "华东", "and"),
	)

	got := applyFilterOverrides(req, []entity.Filter{
		filter("d-region", "region", "eq", "华南", "and"),
	})

	if len(got.Filters) != 2 {
		t.Fatalf("期望 2 条（保留 date + 覆盖 region），得到 %d 条: %+v", len(got.Filters), got.Filters)
	}
	if names := fieldNames(got.Filters); names[0] != "order_date" || names[1] != "region" {
		t.Fatalf("期望字段顺序 [order_date region]，得到 %v", names)
	}
	if got.Filters[1].ID != "d-region" {
		t.Errorf("region 上应只剩仪表盘的条件，实际是 %q", got.Filters[1].ID)
	}
	for _, f := range got.Filters {
		if f.ID == "c-region" {
			t.Error("被仪表盘接管的字段上，图表自身条件必须整条丢弃（'仪表盘有的优先'）")
		}
	}
}

func TestApplyFilterOverridesDropsEveryChartConditionSharingAField(t *testing.T) {
	// 同一字段上图表可能有两条（如 date >= X 且 date <= Y）；覆盖是**按字段**判定，
	// 因此两条都要丢弃，而不是只丢第一条。
	req := requestWith(
		filter("c1", "order_date", "gte", "2026-01-01", "and"),
		filter("c2", "order_date", "lte", "2026-01-31", "and"),
		filter("c3", "region", "eq", "华东", "and"),
	)

	got := applyFilterOverrides(req, []entity.Filter{
		filter("d1", "order_date", "between", "2026-03-01", "and"),
	})

	if names := fieldNames(got.Filters); len(names) != 2 || names[0] != "region" || names[1] != "order_date" {
		t.Fatalf("期望 [region order_date]，得到 %v", names)
	}
	for _, f := range got.Filters {
		if f.ID == "c1" || f.ID == "c2" {
			t.Errorf("同字段的图表条件应全部丢弃，仍残留 %q", f.ID)
		}
	}
}

func TestApplyFilterOverridesAppendsAndLogicByDefault(t *testing.T) {
	req := requestWith()

	got := applyFilterOverrides(req, []entity.Filter{
		filter("d1", "region", "eq", "华东", ""),
	})

	if len(got.Filters) != 1 {
		t.Fatalf("期望 1 条，得到 %d 条", len(got.Filters))
	}
	if got.Filters[0].Logic != "and" {
		t.Errorf("追加条件缺省逻辑应为 and（仪表盘只能收窄、不能放宽），实际 %q", got.Filters[0].Logic)
	}
}

func TestApplyFilterOverridesIgnoresOverridesWithoutField(t *testing.T) {
	req := requestWith(filter("c1", "region", "eq", "华东", "and"))

	// 空 field 的 overrides 声明不了任何字段 → 既不能接管，也不该被追加。
	got := applyFilterOverrides(req, []entity.Filter{
		filter("d-empty", "", "eq", "x", "and"),
	})

	if got != req {
		t.Fatal("全部 overrides 都无 field 时应短路返回入参")
	}
	if len(got.Filters) != 1 || got.Filters[0].ID != "c1" {
		t.Fatalf("图表条件应原样保留，得到 %+v", got.Filters)
	}
}

func TestApplyFilterOverridesDoesNotMutateInput(t *testing.T) {
	req := requestWith(
		filter("c1", "order_date", "gte", "2026-01-01", "and"),
		filter("c2", "region", "eq", "华东", "and"),
	)
	originalLen := len(req.Filters)
	originalFirst := req.Filters[0]

	_ = applyFilterOverrides(req, []entity.Filter{
		filter("d1", "region", "eq", "华南", "and"),
	})

	if len(req.Filters) != originalLen {
		t.Fatalf("入参 filters 长度被改动：%d → %d", originalLen, len(req.Filters))
	}
	if req.Filters[0] != originalFirst {
		t.Error("入参 filters[0] 被就地修改；合并必须返回副本")
	}
}

func TestApplyFilterOverridesKeepsOtherRequestFields(t *testing.T) {
	req := requestWith(filter("c1", "region", "eq", "华东", "and"))
	req.SpecVersion = func() *int { v := 2; return &v }()
	req.Dims = []string{"region", "city"}

	got := applyFilterOverrides(req, []entity.Filter{
		filter("d1", "city", "in", []any{"北京"}, "and"),
	})

	if got.DatasetID != req.DatasetID || got.ChartType != req.ChartType {
		t.Error("合并不应改动 DatasetID / ChartType")
	}
	if got.SpecVersion == nil || *got.SpecVersion != 2 {
		t.Error("合并不应丢失 SpecVersion（否则 v2 槽位协议会退化成 v1）")
	}
	if len(got.Dims) != 2 {
		t.Errorf("合并不应改动 Dims，得到 %v", got.Dims)
	}
}
