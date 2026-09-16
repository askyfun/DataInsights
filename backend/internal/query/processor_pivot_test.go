package query

import (
	"testing"
)

// TestPivotProcessorV2_CrossTabClassification 验证核心交叉表组装：
// 明细行按 RowKey 合并（同一行维值的多条明细交叉进同一个 PivotRow 的 Values），
// 行小计行 IsSubtotal=true 且 Values 用哨兵键，合计行进入 GrandTotal。
func TestPivotProcessorV2_CrossTabClassification(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}}
	ast := slotAST(dims, []string{SlotRows, SlotColumns})

	rows := []map[string]any{
		{"region": "E", "product": "A", "total": 100.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "E", "product": "B", "total": 50.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "E", "product": nil, "total": 150.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(1)},
		{"region": "W", "product": "A", "total": 30.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "W", "product": nil, "total": 30.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(1)},
		{"region": nil, "product": nil, "total": 180.0, "__pivot_row_grp_0": int64(1), "__pivot_col_grp_0": int64(1)},
	}

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv, ok := resp.(*PivotResponseV2)
	if !ok {
		t.Fatalf("expected *PivotResponseV2, got %T", resp)
	}

	if len(pv.RowHeaders) != 1 || pv.RowHeaders[0] != "region" {
		t.Errorf("expected RowHeaders=[region], got %v", pv.RowHeaders)
	}
	if len(pv.ColHeaders) != 2 || pv.ColHeaders[0] != "A" || pv.ColHeaders[1] != "B" {
		t.Errorf("expected ColHeaders=[A B] in occurrence order, got %v", pv.ColHeaders)
	}
	if len(pv.MetricNames) != 1 || pv.MetricNames[0] != "total" {
		t.Errorf("expected MetricNames=[total], got %v", pv.MetricNames)
	}

	// Cells = [E 明细(交叉合并 A/B), E 小计, W 明细, W 小计]
	if len(pv.Cells) != 4 {
		t.Fatalf("expected 4 cells, got %d: %+v", len(pv.Cells), pv.Cells)
	}
	c0 := pv.Cells[0]
	if c0.IsSubtotal || len(c0.RowKey) != 1 || c0.RowKey[0] != "E" {
		t.Errorf("cell0 expected detail RowKey=[E], got %+v", c0)
	}
	if c0.Values[PivotValueKey("A", "total")] != 100 || c0.Values[PivotValueKey("B", "total")] != 50 {
		t.Errorf("cell0 expected cross-tabbed values A=100 B=50, got %v", c0.Values)
	}
	if len(c0.Values) != 2 {
		t.Errorf("cell0 expected exactly 2 value keys, got %v", c0.Values)
	}
	c1 := pv.Cells[1]
	if !c1.IsSubtotal || c1.RowKey[0] != "E" {
		t.Errorf("cell1 expected subtotal RowKey=[E], got %+v", c1)
	}
	if c1.Values[PivotValueKey(PivotSubtotalColKey, "total")] != 150 {
		t.Errorf("cell1 expected __subtotal__|total=150, got %v", c1.Values)
	}
	if len(c1.Values) != 1 {
		t.Errorf("subtotal cell expected exactly 1 value key, got %v", c1.Values)
	}
	c2 := pv.Cells[2]
	if c2.IsSubtotal || c2.RowKey[0] != "W" || c2.Values[PivotValueKey("A", "total")] != 30 {
		t.Errorf("cell2 expected detail W with A=30, got %+v", c2)
	}
	c3 := pv.Cells[3]
	if !c3.IsSubtotal || c3.RowKey[0] != "W" || c3.Values[PivotValueKey(PivotSubtotalColKey, "total")] != 30 {
		t.Errorf("cell3 expected subtotal W=30, got %+v", c3)
	}

	if pv.GrandTotal == nil {
		t.Fatal("expected GrandTotal, got nil")
	}
	if pv.GrandTotal.IsSubtotal != true {
		t.Errorf("GrandTotal.IsSubtotal expected true (aggregate-row semantics), got false")
	}
	if len(pv.GrandTotal.RowKey) != 0 {
		t.Errorf("GrandTotal.RowKey expected empty, got %v", pv.GrandTotal.RowKey)
	}
	if pv.GrandTotal.Values[PivotValueKey(PivotSubtotalColKey, "total")] != 180 {
		t.Errorf("GrandTotal expected __subtotal__|total=180, got %v", pv.GrandTotal.Values)
	}
}

// TestPivotProcessorV2_AvgCountDistinctSubtotalsAreDbRecomputed 是 plan §8 点名的
// 关键正确性测试：AVG/COUNT(DISTINCT) 的小计值必须由数据库在 GROUPING SETS 的每个
// 分组层级上重算，处理器原样透传，绝不能在 Go 端对子分组结果求平均/求和。
// 数据集语义：E/A 明细 price={10,20} users={u1,u2}；E/B 明细 price={60} users={u2,u3}。
// 正确小计（DB 重算）：avg=(10+20+60)/3=30，count_distinct=|{u1,u2,u3}|=3。
// 错误算法（Go 二次聚合）：avg 的平均=(15+60)/2=37.5，count_distinct 的加总=2+2=4。
func TestPivotProcessorV2_AvgCountDistinctSubtotalsAreDbRecomputed(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{
		{Field: "price", Agg: AggAvg, Alias: "avg_price"},
		{Field: "user_id", Agg: AggCountDistinct, Alias: "uniq_users"},
	}
	ast := slotAST(dims, []string{SlotRows, SlotColumns})

	// 模拟数据库 GROUPING SETS 返回：小计/合计行携带的是 DB 在对应分组层级上重算的值。
	rows := []map[string]any{
		{"region": "E", "product": "A", "avg_price": 15.0, "uniq_users": int64(2), "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "E", "product": "B", "avg_price": 60.0, "uniq_users": int64(2), "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(0)},
		{"region": "E", "product": nil, "avg_price": 30.0, "uniq_users": int64(3), "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": int64(1)},
		{"region": nil, "product": nil, "avg_price": 30.0, "uniq_users": int64(3), "__pivot_row_grp_0": int64(1), "__pivot_col_grp_0": int64(1)},
	}

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv := resp.(*PivotResponseV2)

	if len(pv.Cells) != 2 {
		t.Fatalf("expected 2 cells (E detail + E subtotal), got %d", len(pv.Cells))
	}
	subtotal := pv.Cells[1]
	if !subtotal.IsSubtotal {
		t.Fatal("expected second cell to be the row subtotal")
	}
	gotAvg := subtotal.Values[PivotValueKey(PivotSubtotalColKey, "avg_price")]
	if gotAvg != 30.0 {
		t.Errorf("subtotal avg must be DB-recomputed 30, got %v (37.5 would mean averaging sub-group averages)", gotAvg)
	}
	gotUniq := subtotal.Values[PivotValueKey(PivotSubtotalColKey, "uniq_users")]
	if gotUniq != 3 {
		t.Errorf("subtotal count_distinct must be DB-recomputed 3, got %v (4 would mean summing sub-group counts)", gotUniq)
	}
	if pv.GrandTotal == nil {
		t.Fatal("expected GrandTotal")
	}
	if pv.GrandTotal.Values[PivotValueKey(PivotSubtotalColKey, "avg_price")] != 30.0 ||
		pv.GrandTotal.Values[PivotValueKey(PivotSubtotalColKey, "uniq_users")] != 3 {
		t.Errorf("grand total must carry DB-recomputed values, got %v", pv.GrandTotal.Values)
	}
}

// TestPivotProcessorV2_MultiDimRowsAndCols 验证多行维度 + 多列维度的分类与键组装：
// RowKey 是每个行维度一个元素；colHeader 是多列维度值用 " - " 连接（与 AxisProcessor 的
// 维度组合连接符一致）；标记列按维度数量扩展（2 行 + 2 列 = 4 个标记列）。
func TestPivotProcessorV2_MultiDimRowsAndCols(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "city", "product", "channel"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}}
	ast := slotAST(dims, []string{SlotRows, SlotRows, SlotColumns, SlotColumns})

	rows := []map[string]any{
		{"region": "E", "city": "SH", "product": "A", "channel": "online", "total": 70.0,
			"__pivot_row_grp_0": int64(0), "__pivot_row_grp_1": int64(0), "__pivot_col_grp_0": int64(0), "__pivot_col_grp_1": int64(0)},
		{"region": "E", "city": "SH", "product": nil, "channel": nil, "total": 70.0,
			"__pivot_row_grp_0": int64(0), "__pivot_row_grp_1": int64(0), "__pivot_col_grp_0": int64(1), "__pivot_col_grp_1": int64(1)},
		{"region": nil, "city": nil, "product": nil, "channel": nil, "total": 70.0,
			"__pivot_row_grp_0": int64(1), "__pivot_row_grp_1": int64(1), "__pivot_col_grp_0": int64(1), "__pivot_col_grp_1": int64(1)},
	}

	resp, err := p.Process(rows, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv := resp.(*PivotResponseV2)

	if len(pv.RowHeaders) != 2 || pv.RowHeaders[0] != "region" || pv.RowHeaders[1] != "city" {
		t.Errorf("expected RowHeaders=[region city], got %v", pv.RowHeaders)
	}
	if len(pv.ColHeaders) != 1 || pv.ColHeaders[0] != "A - online" {
		t.Errorf(`expected ColHeaders=["A - online"], got %v`, pv.ColHeaders)
	}
	if len(pv.Cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(pv.Cells))
	}
	detail := pv.Cells[0]
	if len(detail.RowKey) != 2 || detail.RowKey[0] != "E" || detail.RowKey[1] != "SH" {
		t.Errorf("expected RowKey=[E SH], got %v", detail.RowKey)
	}
	if detail.Values[PivotValueKey("A - online", "total")] != 70 {
		t.Errorf("expected combined colHeader value key, got %v", detail.Values)
	}
	subtotal := pv.Cells[1]
	if !subtotal.IsSubtotal || subtotal.Values[PivotValueKey(PivotSubtotalColKey, "total")] != 70 {
		t.Errorf("expected row subtotal with sentinel key, got %+v", subtotal)
	}
	if pv.GrandTotal == nil || len(pv.GrandTotal.RowKey) != 0 {
		t.Errorf("expected GrandTotal with empty RowKey, got %+v", pv.GrandTotal)
	}
}

// TestPivotProcessorV2_PartialGroupingRejected 验证部分汇总组合（行维度被汇总但列维度
// 保留）被显式拒绝：生成的 GROUPING SETS 只有三个组合，出现该形状即契约破坏，
// 必须报错而不是静默吞掉（禁止空错误块）。
func TestPivotProcessorV2_PartialGroupingRejected(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}}
	ast := slotAST(dims, []string{SlotRows, SlotColumns})

	rows := []map[string]any{
		// 行标记=1 但列标记=0：列小计形状，本任务的 GROUPING SETS 不生成
		{"region": nil, "product": "A", "total": 100.0, "__pivot_row_grp_0": int64(1), "__pivot_col_grp_0": int64(0)},
	}

	if _, err := p.Process(rows, dims, metrics, ast); err == nil {
		t.Fatal("expected error for partial grouping row, got nil")
	}
}

// TestPivotProcessorV2_MissingMarkerRejected 验证标记列缺失/非数值时报错。
func TestPivotProcessorV2_MissingMarkerRejected(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}}
	ast := slotAST(dims, []string{SlotRows, SlotColumns})

	missing := []map[string]any{
		{"region": "E", "product": "A", "total": 100.0, "__pivot_row_grp_0": int64(0)},
	}
	if _, err := p.Process(missing, dims, metrics, ast); err == nil {
		t.Error("expected error for missing col grouping marker, got nil")
	}

	nonNumeric := []map[string]any{
		{"region": "E", "product": "A", "total": 100.0, "__pivot_row_grp_0": int64(0), "__pivot_col_grp_0": "yes"},
	}
	if _, err := p.Process(nonNumeric, dims, metrics, ast); err == nil {
		t.Error("expected error for non-numeric grouping marker, got nil")
	}
}

// TestPivotProcessorV2_RequiresSlots 验证槽位不可解析时防御性报错
// （executor 只在 resolvePivotSlots 成功后才调用本处理器，此处钉死直接误用的行为）。
func TestPivotProcessorV2_RequiresSlots(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum}}
	rows := []map[string]any{{"region": "E", "product": "A", "amount": 1.0}}

	if _, err := p.Process(rows, dims, metrics, nil); err == nil {
		t.Error("expected error when ast is nil, got nil")
	}
	// v1 形状：所有维度都是 rows 槽位，没有 columns → 不可解析
	ast := slotAST(dims, []string{SlotRows, SlotRows})
	if _, err := p.Process(rows, dims, metrics, ast); err == nil {
		t.Error("expected error when columns slot is missing, got nil")
	}
}

// TestPivotProcessorV2_EmptyRows 验证空结果集返回结构完整的空响应（GrandTotal 为 nil）。
func TestPivotProcessorV2_EmptyRows(t *testing.T) {
	p := &PivotProcessorV2{}
	dims := []string{"region", "product"}
	metrics := []MetricConfig{{Field: "amount", Agg: AggSum, Alias: "total"}}
	ast := slotAST(dims, []string{SlotRows, SlotColumns})

	resp, err := p.Process([]map[string]any{}, dims, metrics, ast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv := resp.(*PivotResponseV2)
	if len(pv.RowHeaders) != 1 || pv.RowHeaders[0] != "region" {
		t.Errorf("expected RowHeaders=[region], got %v", pv.RowHeaders)
	}
	if pv.ColHeaders == nil || len(pv.ColHeaders) != 0 {
		t.Errorf("expected empty non-nil ColHeaders, got %v", pv.ColHeaders)
	}
	if pv.Cells == nil || len(pv.Cells) != 0 {
		t.Errorf("expected empty non-nil Cells, got %v", pv.Cells)
	}
	if pv.GrandTotal != nil {
		t.Errorf("expected nil GrandTotal for empty rows, got %+v", pv.GrandTotal)
	}
}

// TestResolvePivotSlots_FallbackCases 验证回退到旧路径的判定：
// v1（GroupName 全为 rows / 全为空）、未知槽位、字段名不匹配、数量不匹配。
func TestResolvePivotSlots_FallbackCases(t *testing.T) {
	cases := []struct {
		name       string
		dims       []string
		groupNames []string
		wantOK     bool
	}{
		{"v2 rows+columns", []string{"region", "product"}, []string{SlotRows, SlotColumns}, true},
		{"v1 all rows", []string{"region", "product"}, []string{SlotRows, SlotRows}, false},
		{"empty group names", []string{"region", "product"}, []string{"", ""}, false},
		{"unknown slot", []string{"region", "product"}, []string{SlotRows, "diagonal"}, false},
		{"only columns", []string{"region", "product"}, []string{SlotColumns, SlotColumns}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast := slotAST(tc.dims, tc.groupNames)
			_, _, ok := resolvePivotSlots(tc.dims, ast)
			if ok != tc.wantOK {
				t.Errorf("resolvePivotSlots ok = %v, want %v", ok, tc.wantOK)
			}
		})
	}

	// 字段名不匹配（防御性兜底）
	ast := slotAST([]string{"region", "product"}, []string{SlotRows, SlotColumns})
	if _, _, ok := resolvePivotSlots([]string{"region", "category"}, ast); ok {
		t.Error("expected ok=false for field name mismatch")
	}
	// 数量不匹配
	if _, _, ok := resolvePivotSlots([]string{"region"}, ast); ok {
		t.Error("expected ok=false for length mismatch")
	}
	// ast 为 nil（v1 直连 qb.Build 路径）
	if _, _, ok := resolvePivotSlots([]string{"region", "product"}, nil); ok {
		t.Error("expected ok=false for nil ast")
	}
}
