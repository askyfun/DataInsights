package dashboard

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
)

// ---------------------------------------------------------------------------
// 假 provider：实现 query.go 的 chartDataProvider 窄接口，用于在不连真库、不起
// chart 服务的前提下覆盖 Query 的全部语义。
// ---------------------------------------------------------------------------

type dataCall struct {
	chartID   int
	overrides []entity.Filter
}

type fakeChartProvider struct {
	metaFn func(ctx context.Context, id int) (*entity.ChartQueryContext, error)
	dataFn func(ctx context.Context, id int, overrides []entity.Filter) (entity.ChartDataResult, error)

	mu        sync.Mutex
	metaCalls []int
	dataCalls []dataCall
}

func (f *fakeChartProvider) ChartQueryContext(ctx context.Context, id int) (*entity.ChartQueryContext, error) {
	f.mu.Lock()
	f.metaCalls = append(f.metaCalls, id)
	f.mu.Unlock()
	if f.metaFn != nil {
		return f.metaFn(ctx, id)
	}
	return &entity.ChartQueryContext{Exists: true}, nil
}

func (f *fakeChartProvider) GetDataWithFilterOverrides(
	ctx context.Context, id int, overrides []entity.Filter,
) (entity.ChartDataResult, error) {
	f.mu.Lock()
	f.dataCalls = append(f.dataCalls, dataCall{chartID: id, overrides: overrides})
	f.mu.Unlock()
	if f.dataFn != nil {
		return f.dataFn(ctx, id, overrides)
	}
	return entity.ChartDataResult{Data: []map[string]any{{"chart": id}}}, nil
}

func (f *fakeChartProvider) dataCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.dataCalls)
}

// overridesFor 返回针对某图表的最后一次取数覆盖条件（没有则返回 nil,false）。
func (f *fakeChartProvider) overridesFor(chartID int) ([]entity.Filter, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.dataCalls) - 1; i >= 0; i-- {
		if f.dataCalls[i].chartID == chartID {
			return f.dataCalls[i].overrides, true
		}
	}
	return nil, false
}

// testDashboardRead 让一次 Query 能读到一盘：Query 复用本包的 Get（未软删行）。
func expectDashboardRead(t *testing.T, mock sqlmock.Sqlmock, layout string) {
	t.Helper()
	ts := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows(dashboardColumns()).AddRow(
			testID, "月度经营总览", nil, layout, "draft", nil, nil, ts, ts, nil))
}

// layoutChart 是一块 type=chart 的 widget 字面量。
func layoutChart(widgetID string, chartID int) string {
	return fmt.Sprintf(`{"widgetId":%q,"type":"chart","x":0,"y":0,"w":6,"h":4,"chartId":%d}`, widgetID, chartID)
}

// layoutFilter 是一块 type=filter 的 widget 字面量。defaultValue 固定带上「华北」，
// 用来证伪「未下发取值时回落 layout 默认值」。
func layoutFilter(widgetID string, datasetID int, column, operator string) string {
	return fmt.Sprintf(
		`{"widgetId":%q,"type":"filter","x":0,"y":4,"w":6,"h":2,"operator":%q,"multi":true,"defaultValue":["华北"],"binding":{"datasetId":%d,"column":%q}}`,
		widgetID, operator, datasetID, column)
}

func layoutDoc(widgets ...string) string {
	return `{"version":1,"widgets":[` + strings.Join(widgets, ",") + `]}`
}

// ---------------------------------------------------------------------------
// 四态
// ---------------------------------------------------------------------------

// TestQueryBlockStatuses 覆盖 PRD §6.3 的四态。关键断言有两条：非 ok 态 Data 恒为
// nil（JSON 输出 null）；chart_deleted / chart_missing 不做取数（那两次数据源连接
// 必然是白费）。
func TestQueryBlockStatuses(t *testing.T) {
	cases := []struct {
		name          string
		meta          *entity.ChartQueryContext
		dataErr       error
		wantStatus    string
		wantDataCalls int
	}{
		{"ok", &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil, entity.DashboardBlockOK, 1},
		{"chart_deleted", &entity.ChartQueryContext{Exists: true, Deleted: true, DatasetID: 7}, nil, entity.DashboardBlockChartDeleted, 0},
		{"chart_missing", &entity.ChartQueryContext{Exists: false}, nil, entity.DashboardBlockChartMissing, 0},
		{"error", &entity.ChartQueryContext{Exists: true, DatasetID: 7}, errors.New("数据源不可达"), entity.DashboardBlockError, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42)))

			fake := &fakeChartProvider{
				metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) { return tc.meta, nil },
				dataFn: func(_ context.Context, _ int, _ []entity.Filter) (entity.ChartDataResult, error) {
					if tc.dataErr != nil {
						return entity.ChartDataResult{}, tc.dataErr
					}
					return entity.ChartDataResult{Data: []map[string]any{{"region": "华东"}}}, nil
				},
			}
			svc.chartProvider = fake

			got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
			if len(got.Results) != 1 {
				t.Fatalf("期望 1 块，实际 %d", len(got.Results))
			}
			blk := got.Results[0]
			if blk.WidgetID != "w-1" || blk.ChartID != 42 {
				t.Errorf("块身份错误: %+v", blk)
			}
			if blk.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", blk.Status, tc.wantStatus)
			}
			if tc.wantStatus == entity.DashboardBlockOK {
				if blk.Data == nil {
					t.Fatal("ok 态必须带 data")
				}
			} else if blk.Data != nil {
				t.Errorf("非 ok 态 data 必须是 nil（wire 上为 null），实际 %+v", blk.Data)
			}
			if tc.wantStatus == entity.DashboardBlockError && blk.Message != tc.dataErr.Error() {
				t.Errorf("error 态 message = %q, want %q", blk.Message, tc.dataErr.Error())
			}
			if got := fake.dataCallCount(); got != tc.wantDataCalls {
				t.Errorf("取数调用次数 = %d, want %d", got, tc.wantDataCalls)
			}
		})
	}
}

// TestQueryProviderContextErrorIsBlockError provider 在「查元信息」阶段就失败时，
// 同样收敛成块级 error，而不是让整盘失败。
func TestQueryProviderContextErrorIsBlockError(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42)))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return nil, errors.New("bi_chart 读取失败")
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got.Results[0].Status != entity.DashboardBlockError {
		t.Fatalf("status = %q, want %q", got.Results[0].Status, entity.DashboardBlockError)
	}
	if got.Results[0].Message != "bi_chart 读取失败" {
		t.Errorf("message = %q", got.Results[0].Message)
	}
	if fake.dataCallCount() != 0 {
		t.Errorf("元信息读取失败后不应再取数")
	}
}

// ---------------------------------------------------------------------------
// 顺序、布局解析、并发
// ---------------------------------------------------------------------------

// TestQueryOrderFollowsLayout 结果顺序恒等于 layout 顺序：结果按下标写回切片，
// 与各块的实际完成先后无关（这里让靠前的块故意慢，制造完成顺序反转）。
func TestQueryOrderFollowsLayout(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChart("w-1", 11), layoutChart("w-2", 22), layoutChart("w-3", 33)))

	fake := &fakeChartProvider{dataFn: func(_ context.Context, id int, _ []entity.Filter) (entity.ChartDataResult, error) {
		// 图表 id 越小越慢：若顺序依赖完成先后，结果就会反过来。
		time.Sleep(time.Duration(40-id) * time.Millisecond)
		return entity.ChartDataResult{Data: id}, nil
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	wantCharts := []int{11, 22, 33}
	wantWidgets := []string{"w-1", "w-2", "w-3"}
	for i := range wantCharts {
		if got.Results[i].ChartID != wantCharts[i] || got.Results[i].WidgetID != wantWidgets[i] {
			t.Fatalf("第 %d 块 = %+v, want chartId=%d widgetId=%q",
				i, got.Results[i], wantCharts[i], wantWidgets[i])
		}
		if got.Results[i].Status != entity.DashboardBlockOK {
			t.Fatalf("第 %d 块 status=%q", i, got.Results[i].Status)
		}
	}
}

// TestQueryBoundsConcurrency 并发上限：8 块同时取数时不得同时打满 8 个连接
// （PRD §6.3 / §11-6 的并发上限要求）。
func TestQueryBoundsConcurrency(t *testing.T) {
	svc, mock, _ := newTestService(t)
	widgets := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		widgets = append(widgets, layoutChart(fmt.Sprintf("w-%d", i), i+1))
	}
	expectDashboardRead(t, mock, layoutDoc(widgets...))

	var mu sync.Mutex
	inflight, peak := 0, 0
	fake := &fakeChartProvider{dataFn: func(_ context.Context, id int, _ []entity.Filter) (entity.ChartDataResult, error) {
		mu.Lock()
		inflight++
		if inflight > peak {
			peak = inflight
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		inflight--
		mu.Unlock()
		return entity.ChartDataResult{Data: id}, nil
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got.Results) != 8 {
		t.Fatalf("期望 8 块，实际 %d", len(got.Results))
	}
	if peak > dashboardQueryConcurrency {
		t.Errorf("并发峰值 = %d, 超过上限 %d", peak, dashboardQueryConcurrency)
	}
	if peak < 2 {
		t.Errorf("并发未生效（峰值 = %d）：本测试会失去意义", peak)
	}
}

// TestQueryMalformedLayoutYieldsEmptyResults layout 归前端所有：读不懂（空串 /
// 损坏 JSON / 顶层不是对象 / widgets 缺失）一律表现为「没有可取的块」，
// 绝不报错、更不 500。
func TestQueryMalformedLayoutYieldsEmptyResults(t *testing.T) {
	for _, layout := range []string{"", "{bad", "[]", "null", `"str"`, "{}", `{"version":1}`, `{"widgets":"x"}`} {
		svc, mock, _ := newTestService(t)
		expectDashboardRead(t, mock, layout)
		fake := &fakeChartProvider{}
		svc.chartProvider = fake

		got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
		if err != nil {
			t.Fatalf("layout %q: 不期望错误，实际 %v", layout, err)
		}
		if got == nil {
			t.Fatalf("layout %q: 结果不能为 nil", layout)
		}
		if got.Results == nil || len(got.Results) != 0 {
			t.Errorf("layout %q: 期望空 results 切片，实际 %#v", layout, got.Results)
		}
		if fake.dataCallCount() != 0 || len(fake.metaCalls) != 0 {
			t.Errorf("layout %q: 无图表块时不应触碰 provider", layout)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("layout %q: expectations: %v", layout, err)
		}
	}
}

// TestQueryIgnoresTextWidgets 文本块不参与取数，也不占结果下标。
func TestQueryIgnoresTextWidgets(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		`{"widgetId":"t-1","type":"text","x":0,"y":0,"w":12,"h":2,"markdown":"# 标题"}`,
		layoutChart("w-1", 42),
		`{"widgetId":"t-2","type":"text","x":0,"y":2,"w":12,"h":2,"markdown":"说明"}`,
	))
	fake := &fakeChartProvider{}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].WidgetID != "w-1" {
		t.Fatalf("文本块不得进入结果: %+v", got.Results)
	}
}

// ---------------------------------------------------------------------------
// 筛选合并（PRD §8.3）
// ---------------------------------------------------------------------------

// TestQueryMergesSameDatasetFilter 钉住合并的三件产出：overrides 的形状
// （field/operator/value/logic + 稳定 id）、appliedFields 与 overriddenFields。
func TestQueryMergesSameDatasetFilter(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "region", "in")))

	fake := &fakeChartProvider{
		metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
			return &entity.ChartQueryContext{
				Exists: true, DatasetID: 7, OwnFilterFields: []string{"region", "amount"},
			}, nil
		},
	}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{"华东", "华南"}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	overrides, ok := fake.overridesFor(42)
	if !ok {
		t.Fatal("未观察到取数调用")
	}
	want := []entity.Filter{{
		ID:       "dash-f-1",
		Field:    "region",
		Operator: "in",
		Value:    []any{"华东", "华南"},
		Logic:    "and",
	}}
	if !reflect.DeepEqual(overrides, want) {
		t.Errorf("overrides = %#v, want %#v", overrides, want)
	}

	blk := got.Results[0]
	if blk.Status != entity.DashboardBlockOK {
		t.Fatalf("status = %q", blk.Status)
	}
	if !reflect.DeepEqual(blk.AppliedFields, []string{"region"}) {
		t.Errorf("appliedFields = %#v, want [region]", blk.AppliedFields)
	}
	if !reflect.DeepEqual(blk.OverriddenFields, []string{"region"}) {
		t.Errorf("overriddenFields = %#v, want [region]", blk.OverriddenFields)
	}
}

// TestQueryOverriddenFieldsEmptyWhenChartLacksTheField 图表自身没有该字段的条件时，
// 盘级条件仍是「追加」而非「覆盖」：appliedFields 有值、overriddenFields 为空。
func TestQueryOverriddenFieldsEmptyWhenChartLacksTheField(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "region", "in")))

	svc.chartProvider = &fakeChartProvider{
		metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
			return &entity.ChartQueryContext{Exists: true, DatasetID: 7, OwnFilterFields: []string{"amount"}}, nil
		},
	}

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{"华东"}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !reflect.DeepEqual(got.Results[0].AppliedFields, []string{"region"}) {
		t.Errorf("appliedFields = %#v, want [region]", got.Results[0].AppliedFields)
	}
	if len(got.Results[0].OverriddenFields) != 0 {
		t.Errorf("图表自身无 region 条件时 overriddenFields 必须为空，实际 %#v", got.Results[0].OverriddenFields)
	}
}

// TestQueryMultiValueSwitchesToInOperator 多选值 + 非 in 算子按 in 处理
// （布局里的算子可能是在单选语境下配的）。
func TestQueryMultiValueSwitchesToInOperator(t *testing.T) {
	cases := []struct {
		name     string
		operator string
		value    []any
		wantOp   string
	}{
		{"eq + 多值 → in", "eq", []any{"华东", "华南"}, "in"},
		{"in + 多值 → 仍是 in", "in", []any{"华东", "华南"}, "in"},
		{"notIn + 多值 → 仍是 notIn", "notIn", []any{"华东", "华南"}, "notIn"},
		{"eq + 单值 → 保持 eq", "eq", []any{"华东"}, "eq"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "region", tc.operator)))

			fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
				return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
			}}
			svc.chartProvider = fake

			if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
				Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: tc.value}},
			}); err != nil {
				t.Fatalf("Query: %v", err)
			}
			overrides, _ := fake.overridesFor(42)
			if len(overrides) != 1 || overrides[0].Operator != tc.wantOp {
				t.Fatalf("overrides = %#v, want operator=%q", overrides, tc.wantOp)
			}
		})
	}
}

// TestQueryBetweenSplitsPairValue 区间算子（日期筛选 `最近 7 天`、数值区间）的二元组
// 必须拆成 Value + ValueEnd 两个绑定参数（bun_builder 的 FilterBetween 分支就吃这两个），
// 且这条规则要排在「多值降级为 in」之前 —— 否则区间会被吃成 `IN (下界, 上界)`。
func TestQueryBetweenSplitsPairValue(t *testing.T) {
	cases := []struct {
		name         string
		value        []any
		wantOp       string
		wantValue    any
		wantValueEnd any
	}{
		{
			name:         "二元组 → between + ValueEnd",
			value:        []any{"2026-09-18", "2026-09-24"},
			wantOp:       "between",
			wantValue:    "2026-09-18",
			wantValueEnd: "2026-09-24",
		},
		{
			name:         "缺一端 → 保持 between 且 ValueEnd 为空（恒假区间，不静默放大结果集）",
			value:        []any{"2026-09-18"},
			wantOp:       "between",
			wantValue:    "2026-09-18",
			wantValueEnd: nil,
		}, {
			name:         "三个值 → 降级为 in（区间语义已不成立）",
			value:        []any{"2026-09-18", "2026-09-24", "2026-10-01"},
			wantOp:       "in",
			wantValue:    []any{"2026-09-18", "2026-09-24", "2026-10-01"},
			wantValueEnd: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "sale_date", "between")))

			fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
				return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
			}}
			svc.chartProvider = fake

			if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
				Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: tc.value}},
			}); err != nil {
				t.Fatalf("Query: %v", err)
			}
			overrides, _ := fake.overridesFor(42)
			if len(overrides) != 1 {
				t.Fatalf("overrides = %#v, want 1 条", overrides)
			}
			if overrides[0].Operator != tc.wantOp {
				t.Errorf("operator = %q, want %q", overrides[0].Operator, tc.wantOp)
			}
			if !reflect.DeepEqual(overrides[0].Value, tc.wantValue) {
				t.Errorf("value = %#v, want %#v", overrides[0].Value, tc.wantValue)
			}
			if !reflect.DeepEqual(overrides[0].ValueEnd, tc.wantValueEnd) {
				t.Errorf("value_end = %#v, want %#v", overrides[0].ValueEnd, tc.wantValueEnd)
			}
		})
	}
}

// TestQueryScalarOperatorsUnwrapSingleValue 覆盖条件的**取值形状**按算子分流（这是 SQL 正确性的前提）：
// 标量算子（eq/neq/gt/gte/lt/lte/like）必须收到单个标量 —— builder 对它们是 `append(f.Value)`
// 单参数绑定，塞数组会渲染成 `col = ARRAY[...]`（PG 42883，且现有测试只断言算子字符串，长期漏检）；
// in/notIn 相反，必须保留数组（builder 按元素展开成 `IN (?, ?, …)`）。
func TestQueryScalarOperatorsUnwrapSingleValue(t *testing.T) {
	cases := []struct {
		name      string
		operator  string
		value     []any
		wantOp    string
		wantValue any
	}{
		{"eq + 单值 → 标量", "eq", []any{"华东"}, "eq", "华东"},
		{"gte + 单值 → 标量（日期筛选「开始无限制」走这条）", "gte", []any{"2023-06-14"}, "gte", "2023-06-14"},
		{"lte + 单值 → 标量", "lte", []any{"2023-06-14"}, "lte", "2023-06-14"},
		{"like + 单值 → 标量", "like", []any{"华"}, "like", "华"},
		{"isNull 不看值：占位 null 只是「已激活」标记", "isNull", []any{nil}, "isNull", nil},
		{"in + 单值 → 保留数组", "in", []any{"华东"}, "in", []any{"华东"}},
		{"notIn + 双值 → 保留数组", "notIn", []any{"华东", "华南"}, "notIn", []any{"华东", "华南"}},
		{"eq + 双值 → 降级 in 并保留数组", "eq", []any{"华东", "华南"}, "in", []any{"华东", "华南"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "region", tc.operator)))

			fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
				return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
			}}
			svc.chartProvider = fake

			if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
				Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: tc.value}},
			}); err != nil {
				t.Fatalf("Query: %v", err)
			}
			overrides, _ := fake.overridesFor(42)
			if len(overrides) != 1 {
				t.Fatalf("overrides = %#v, want 1 条", overrides)
			}
			if overrides[0].Operator != tc.wantOp {
				t.Errorf("operator = %q, want %q", overrides[0].Operator, tc.wantOp)
			}
			if !reflect.DeepEqual(overrides[0].Value, tc.wantValue) {
				t.Errorf("value 形状错误（会被 builder 当成单个绑定参数）: %#v, want %#v",
					overrides[0].Value, tc.wantValue)
			}
		})
	}
}

// TestQueryInactiveFiltersProduceNoOverrides 未激活的筛选器不产生任何覆盖：
// 值缺失、空数组、以及「layout 里有 defaultValue 但请求没下发」三种形态都必须
// 一视同仁——请求是取值的唯一真相源，回落默认值会让盘一打开就套用历史默认值。
func TestQueryInactiveFiltersProduceNoOverrides(t *testing.T) {
	cases := []struct {
		name    string
		filters []entity.DashboardQueryFilter
	}{
		{"请求里没出现该筛选器", nil},
		{"值缺失（nil）", []entity.DashboardQueryFilter{{WidgetID: "f-1"}}},
		{"值为空数组", []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{}}}},
		{"请求里只有别的筛选器", []entity.DashboardQueryFilter{{WidgetID: "f-other", Value: []any{"华东"}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 7, "region", "in")))

			fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
				return &entity.ChartQueryContext{Exists: true, DatasetID: 7, OwnFilterFields: []string{"region"}}, nil
			}}
			svc.chartProvider = fake

			got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{Filters: tc.filters})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			overrides, ok := fake.overridesFor(42)
			if !ok {
				t.Fatal("未观察到取数调用")
			}
			if len(overrides) != 0 {
				t.Errorf("未激活的筛选器不得产生覆盖（layout 的 defaultValue 也不得回落）: %#v", overrides)
			}
			if len(got.Results[0].AppliedFields) != 0 || len(got.Results[0].OverriddenFields) != 0 {
				t.Errorf("未激活时 applied/overridden 都必须为空: %+v", got.Results[0])
			}
		})
	}
}

// TestQueryCrossDatasetFilterNotApplied 绑定到别的数据集的筛选器不适用：按
// (datasetId, column) 二元组命中（PRD §11-2），纯列名匹配会跨数据集静默误伤。
func TestQueryCrossDatasetFilterNotApplied(t *testing.T) {
	svc, mock, _ := newTestService(t)
	// 同一列名 region 在数据集 8，但图表在数据集 7。
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42), layoutFilter("f-1", 8, "region", "in")))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: 7, OwnFilterFields: []string{"region"}}, nil
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{"华东"}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	overrides, _ := fake.overridesFor(42)
	if len(overrides) != 0 {
		t.Errorf("跨数据集筛选器不得适用: %#v", overrides)
	}
	if len(got.Results[0].AppliedFields) != 0 {
		t.Errorf("appliedFields 必须为空: %#v", got.Results[0].AppliedFields)
	}
	if len(got.Results[0].OverriddenFields) != 0 {
		t.Errorf("图表自身 region 条件不应被覆盖: %#v", got.Results[0].OverriddenFields)
	}
}

// TestQueryFilterWithoutBindingIgnored 没有有效 binding.column 的筛选器块认领不了
// 任何条件，等同未绑定。
func TestQueryFilterWithoutBindingIgnored(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChart("w-1", 42),
		`{"widgetId":"f-1","type":"filter","x":0,"y":4,"w":6,"h":2,"operator":"in","binding":{"datasetId":7,"column":""}}`,
		`{"widgetId":"f-2","type":"filter","x":0,"y":6,"w":6,"h":2,"operator":"in"}`,
	))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
	}}
	svc.chartProvider = fake

	if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{
			{WidgetID: "f-1", Value: []any{"华东"}},
			{WidgetID: "f-2", Value: []any{"华东"}},
		},
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	overrides, _ := fake.overridesFor(42)
	if len(overrides) != 0 {
		t.Errorf("未绑定的筛选器不得产生覆盖: %#v", overrides)
	}
}

// TestMergeRequestFilterBindings 请求自带绑定的筛选器并入 layout 投影（issue #172）。
// 不带绑定的请求项是老写法（绑定仍来自 layout），空列名的绑定等同未绑定。
func TestMergeRequestFilterBindings(t *testing.T) {
	base := []layoutFilterBinding{{WidgetID: "f-1", DatasetID: 7, Column: "region", Operator: "in"}}
	cases := []struct {
		name string
		req  []entity.DashboardQueryFilter
		want []layoutFilterBinding
	}{
		{
			name: "不带绑定（老写法）不并入，layout 那条原样保留",
			req:  []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{"华东"}}},
			want: base,
		},
		{
			name: "空列名的绑定视为未绑定，忽略",
			req: []entity.DashboardQueryFilter{{
				WidgetID: "f-2",
				Binding:  &entity.DashboardQueryFilterBinding{DatasetID: 7, Column: ""},
			}},
			want: base,
		},
		{
			name: "新 widgetId 的绑定追加进表",
			req: []entity.DashboardQueryFilter{{
				WidgetID: "f-2",
				Binding:  &entity.DashboardQueryFilterBinding{DatasetID: 7, Column: "amount"},
				Operator: "gt",
			}},
			want: []layoutFilterBinding{
				{WidgetID: "f-1", DatasetID: 7, Column: "region", Operator: "in"},
				{WidgetID: "f-2", DatasetID: 7, Column: "amount", Operator: "gt"},
			},
		},
		{
			name: "同名 widgetId 的绑定覆盖 layout 里的那条（改了配置未保存）",
			req: []entity.DashboardQueryFilter{{
				WidgetID: "f-1",
				Binding:  &entity.DashboardQueryFilterBinding{DatasetID: 8, Column: "city"},
				Operator: "eq",
			}},
			want: []layoutFilterBinding{{WidgetID: "f-1", DatasetID: 8, Column: "city", Operator: "eq"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeRequestFilterBindings(base, tc.req)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestQueryRequestCarriedBindingAppliesUnsavedFilter 未落库的筛选器（layout 里根本没有它）
// 只要请求自带 binding，就能被认领并产生覆盖——这正是 issue #172 要修的「配好取值却没反应」。
func TestQueryRequestCarriedBindingAppliesUnsavedFilter(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42)))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{
			Exists: true, DatasetID: 7, OwnFilterFields: []string{"region"},
		}, nil
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{{
			WidgetID: "f-unsaved",
			Value:    []any{"华东"},
			Binding:  &entity.DashboardQueryFilterBinding{DatasetID: 7, Column: "region"},
			Operator: "in",
		}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	overrides, ok := fake.overridesFor(42)
	if !ok {
		t.Fatal("未观察到取数调用")
	}
	want := []entity.Filter{{
		ID:       "dash-f-unsaved",
		Field:    "region",
		Operator: "in",
		Value:    []any{"华东"},
		Logic:    "and",
	}}
	if !reflect.DeepEqual(overrides, want) {
		t.Errorf("overrides = %#v, want %#v", overrides, want)
	}
	if !reflect.DeepEqual(got.Results[0].AppliedFields, []string{"region"}) {
		t.Errorf("appliedFields = %#v, want [region]", got.Results[0].AppliedFields)
	}
}

// TestQueryRequestBindingOverridesPersistedFilter 请求自带的绑定优先于 layout：
// 改了算子/绑定但没保存的筛选器也要立刻生效（新增与改动是同一条路径）。
func TestQueryRequestBindingOverridesPersistedFilter(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChart("w-1", 42),
		layoutFilter("f-1", 7, "region", "in"),
	))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
	}}
	svc.chartProvider = fake

	if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{{
			WidgetID: "f-1",
			Value:    []any{100},
			Binding:  &entity.DashboardQueryFilterBinding{DatasetID: 7, Column: "amount"},
			Operator: "gt",
		}},
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	overrides, ok := fake.overridesFor(42)
	if !ok {
		t.Fatal("未观察到取数调用")
	}
	want := []entity.Filter{{
		ID:       "dash-f-1",
		Field:    "amount",
		Operator: "gt",
		Value:    100,
		Logic:    "and",
	}}
	if !reflect.DeepEqual(overrides, want) {
		t.Errorf("overrides = %#v, want %#v（请求绑定必须覆盖 layout）", overrides, want)
	}
}

// TestQueryAppliedFieldsDedupeKeepsOrder 同一列被两块筛选器绑定时，overrides 保留
// 两条（各自是一条件），但 appliedFields 去重保序。
func TestQueryAppliedFieldsDedupeKeepsOrder(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChart("w-1", 42),
		layoutFilter("f-1", 7, "region", "in"),
		layoutFilter("f-2", 7, "region", "in"),
		layoutFilter("f-3", 7, "channel", "in"),
	))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
	}}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters: []entity.DashboardQueryFilter{
			{WidgetID: "f-1", Value: []any{"华东"}},
			{WidgetID: "f-2", Value: []any{"华南"}},
			{WidgetID: "f-3", Value: []any{"线上"}},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	overrides, _ := fake.overridesFor(42)
	if len(overrides) != 3 {
		t.Fatalf("每条激活的筛选器各生成一条覆盖，实际 %#v", overrides)
	}
	if ids := []string{overrides[0].ID, overrides[1].ID, overrides[2].ID}; !reflect.DeepEqual(ids, []string{"dash-f-1", "dash-f-2", "dash-f-3"}) {
		t.Errorf("覆盖条件顺序应与 layout 一致: %#v", ids)
	}
	if !reflect.DeepEqual(got.Results[0].AppliedFields, []string{"region", "channel"}) {
		t.Errorf("appliedFields = %#v, want [region channel]（去重保序）", got.Results[0].AppliedFields)
	}
}

// TestQueryWithoutOverridesPassesNil 没有适用筛选器时传给 chart 的是 nil 而不是
// 空切片：让「不做任何覆盖」在 chart 侧走明确的短路分支。
func TestQueryWithoutOverridesPassesNil(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(layoutChart("w-1", 42)))

	fake := &fakeChartProvider{metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
	}}
	svc.chartProvider = fake

	if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	overrides, ok := fake.overridesFor(42)
	if !ok {
		t.Fatal("未观察到取数调用")
	}
	if overrides != nil {
		t.Errorf("无适用筛选器时应传 nil，实际 %#v", overrides)
	}
}

// TestQueryFailedBlockDoesNotAffectOthers 单块失败不整盘失败：好的块照常 ok，
// 坏的块只有自己变成 error。
func TestQueryFailedBlockDoesNotAffectOthers(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChart("w-1", 11), layoutChart("w-2", 22), layoutChart("w-3", 33)))

	svc.chartProvider = &fakeChartProvider{
		metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
			return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
		},
		dataFn: func(_ context.Context, id int, _ []entity.Filter) (entity.ChartDataResult, error) {
			if id == 22 {
				return entity.ChartDataResult{}, errors.New("SQL 执行失败：permission denied")
			}
			return entity.ChartDataResult{Data: id}, nil
		},
	}

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("单块失败不得让整盘失败，实际: %v", err)
	}
	if len(got.Results) != 3 {
		t.Fatalf("期望 3 块，实际 %d", len(got.Results))
	}
	for i, want := range []string{entity.DashboardBlockOK, entity.DashboardBlockError, entity.DashboardBlockOK} {
		if got.Results[i].Status != want {
			t.Errorf("第 %d 块 status = %q, want %q", i, got.Results[i].Status, want)
		}
	}
	if got.Results[1].Message == "" {
		t.Error("error 块必须带 message")
	}
	if got.Results[0].Data == nil || got.Results[2].Data == nil {
		t.Error("未失败的块必须带 data")
	}
}

// ---------------------------------------------------------------------------
// 接线与入口错误
// ---------------------------------------------------------------------------

// TestQueryWithoutProviderIsBusinessError 未接线时必须返回业务错误（结构化 50000），
// 绝不能 panic。
func TestQueryWithoutProviderIsBusinessError(t *testing.T) {
	svc, _, _ := newTestService(t)

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if got != nil {
		t.Errorf("未接线时不应返回结果: %+v", got)
	}
	if code := bizCode(t, err); code != response.CodeInternalError {
		t.Fatalf("code = %d, want %d", code, response.CodeInternalError)
	}
}

// TestQueryOnMissingDashboardIsNotFound 盘不存在（或已软删）时走既有 Get 的 404
// 语义，且一块都不取。
func TestQueryOnMissingDashboardIsNotFound(t *testing.T) {
	svc, mock, _ := newTestService(t)
	mock.ExpectQuery(`SELECT .* FROM "bi_dashboard"`).
		WillReturnRows(sqlmock.NewRows(dashboardColumns()))

	fake := &fakeChartProvider{}
	svc.chartProvider = fake

	_, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if code := bizCode(t, err); code != response.CodeNotFound {
		t.Fatalf("code = %d, want %d (%v)", code, response.CodeNotFound, err)
	}
	if fake.dataCallCount() != 0 || len(fake.metaCalls) != 0 {
		t.Errorf("盘不存在时不应触碰 provider")
	}
}

// TestQueryEmptyDashboardReturnsEmptySlice 空盘（合法 v1 空文档）返回空 results
// 切片而非 null：前端不必处理 null。
func TestQueryEmptyDashboardReturnsEmptySlice(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, entity.DashboardDefaultLayoutJSON)
	svc.chartProvider = &fakeChartProvider{}

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got.Results == nil || len(got.Results) != 0 {
		t.Fatalf("期望空切片，实际 %#v", got.Results)
	}
}

// ---------------------------------------------------------------------------
// 多页面（layout v2）：按页收窄
//
// layout 文档的归属用**扁平** widgets[].pageId 表达，所以收窄逻辑就是投影时的
// 一个谓词（chartInPage / filterInPage），不需要动 jsonpath 或响应结构。
// ---------------------------------------------------------------------------

// layoutChartOnPage 是带页面归属的图表块（v2 文档的块多一个 pageId）。
func layoutChartOnPage(widgetID, pageID string, chartID int) string {
	return fmt.Sprintf(
		`{"widgetId":%q,"pageId":%q,"type":"chart","x":0,"y":0,"w":6,"h":4,"chartId":%d}`,
		widgetID, pageID, chartID)
}

// layoutScopedFilter 是带页面归属与作用范围的筛选器块；scope 传空串表示不写该键
// （缺省 = 只作用本页）。刻意不带 defaultValue，与 layoutFilter 同样用来证伪
// 「未下发取值时回落 layout 默认值」。
func layoutScopedFilter(widgetID, pageID string, datasetID int, column, operator, scope string) string {
	scopeJSON := ""
	if scope != "" {
		scopeJSON = fmt.Sprintf(`"scope":%q,`, scope)
	}
	return fmt.Sprintf(
		`{"widgetId":%q,"pageId":%q,"type":"filter",%s"x":0,"y":4,"w":6,"h":2,"operator":%q,"multi":true,"binding":{"datasetId":%d,"column":%q}}`,
		widgetID, pageID, scopeJSON, operator, datasetID, column)
}

func projectedChartIDs(blocks []layoutChartBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.WidgetID)
	}
	return out
}

func projectedFilterIDs(bindings []layoutFilterBinding) []string {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.WidgetID)
	}
	return out
}

// TestProjectLayoutScopesByPage 覆盖三条谓词语义：
//   - page_id 空 = 不按页收窄（单页时代的调用方）；
//   - 该页的块 + **缺 pageId 的旧块**都要取（旧块若被排除，打开旧盘会整盘空白）；
//   - 筛选器额外纳入 scope=all 的（它作用于所有页）。
func TestProjectLayoutScopesByPage(t *testing.T) {
	doc := `{"version":2,"grid":{"cols":12},"pages":[{"id":"p-1","name":"总览"},{"id":"p-2","name":"明细"}],"widgets":[` +
		strings.Join([]string{
			layoutChartOnPage("c-p1", "p-1", 11),
			layoutChartOnPage("c-p2", "p-2", 22),
			layoutChart("c-legacy", 33),
			layoutScopedFilter("f-p1", "p-1", 7, "region", "in", ""),
			layoutScopedFilter("f-p2", "p-2", 7, "city", "in", ""),
			layoutScopedFilter("f-global", "p-2", 7, "channel", "in", "all"),
			layoutFilter("f-legacy", 7, "shop", "in"),
		}, ",") + `]}`

	cases := []struct {
		name        string
		pageID      string
		wantCharts  []string
		wantFilters []string
	}{
		{
			name:        "空 page_id = 不按页收窄",
			pageID:      "",
			wantCharts:  []string{"c-p1", "c-p2", "c-legacy"},
			wantFilters: []string{"f-p1", "f-p2", "f-global", "f-legacy"},
		},
		{
			name:        "p-1",
			pageID:      "p-1",
			wantCharts:  []string{"c-p1", "c-legacy"},
			wantFilters: []string{"f-p1", "f-global", "f-legacy"},
		},
		{
			name:        "p-2",
			pageID:      "p-2",
			wantCharts:  []string{"c-p2", "c-legacy"},
			wantFilters: []string{"f-p2", "f-global", "f-legacy"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectLayout(doc, tc.pageID)
			if charts := projectedChartIDs(got.Charts); !reflect.DeepEqual(charts, tc.wantCharts) {
				t.Errorf("charts = %v, want %v", charts, tc.wantCharts)
			}
			if filters := projectedFilterIDs(got.Filters); !reflect.DeepEqual(filters, tc.wantFilters) {
				t.Errorf("filters = %v, want %v", filters, tc.wantFilters)
			}
		})
	}
}

// TestProjectLayoutUnknownPageKeepsLegacyBlocks 页面 id 认不出（跨版本错配 / 手改坏）
// 时只剩缺 pageId 的旧块：不返回错误，也不把整盘判空——与其它解析失败同一口径。
func TestProjectLayoutUnknownPageKeepsLegacyBlocks(t *testing.T) {
	doc := `{"version":2,"pages":[{"id":"p-1","name":"A"}],"widgets":[` +
		layoutChartOnPage("c-p1", "p-1", 11) + `,` + layoutChart("c-legacy", 33) + `]}`

	got := projectLayout(doc, "p-gone")

	if charts := projectedChartIDs(got.Charts); !reflect.DeepEqual(charts, []string{"c-legacy"}) {
		t.Fatalf("charts = %v, want [c-legacy]", charts)
	}
}

// TestQueryScopesByPage 端到端：请求带 page_id 时只取该页的块，且筛选器口径同步收窄
// —— 别页的筛选器即使前端把取值发过来了也不生效，作用所有页的照常生效。
func TestQueryScopesByPage(t *testing.T) {
	svc, mock, _ := newTestService(t)
	doc := `{"version":2,"grid":{"cols":12},"pages":[{"id":"p-1","name":"总览"},{"id":"p-2","name":"明细"}],"widgets":[` +
		strings.Join([]string{
			layoutChartOnPage("c-p1", "p-1", 11),
			layoutChartOnPage("c-p2", "p-2", 22),
			layoutScopedFilter("f-p1", "p-1", 7, "region", "in", ""),
			layoutScopedFilter("f-global", "p-1", 7, "channel", "in", "all"),
		}, ",") + `]}`
	expectDashboardRead(t, mock, doc)

	fake := &fakeChartProvider{
		metaFn: func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
			return &entity.ChartQueryContext{Exists: true, DatasetID: 7}, nil
		},
	}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		PageID: "p-2",
		Filters: []entity.DashboardQueryFilter{
			{WidgetID: "f-p1", Value: []any{"华东"}},
			{WidgetID: "f-global", Value: []any{"线上"}},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if n := fake.dataCallCount(); n != 1 {
		t.Fatalf("取数次数 = %d, want 1（只取当前页的块）", n)
	}
	if len(got.Results) != 1 || got.Results[0].WidgetID != "c-p2" {
		t.Fatalf("results = %+v, want 只有 c-p2", got.Results)
	}

	overrides, ok := fake.overridesFor(22)
	if !ok {
		t.Fatal("chart 22 未取数")
	}
	fields := make([]string, 0, len(overrides))
	for _, o := range overrides {
		fields = append(fields, o.Field)
	}
	if !reflect.DeepEqual(fields, []string{"channel"}) {
		t.Fatalf("生效字段 = %v, want [channel]（region 属于别页，不该生效）", fields)
	}
	if !reflect.DeepEqual(got.Results[0].AppliedFields, []string{"channel"}) {
		t.Fatalf("appliedFields = %v, want [channel]", got.Results[0].AppliedFields)
	}
}

// TestQueryWithoutPageIDCoversEveryPage 不传 page_id 时保持单页时代的语义：整盘所有
// 块都取。这条是旧客户端 / 未升级调用方的兼容底线。
func TestQueryWithoutPageIDCoversEveryPage(t *testing.T) {
	svc, mock, _ := newTestService(t)
	doc := `{"version":2,"pages":[{"id":"p-1","name":"A"},{"id":"p-2","name":"B"}],"widgets":[` +
		layoutChartOnPage("c-p1", "p-1", 11) + `,` + layoutChartOnPage("c-p2", "p-2", 22) + `]}`
	expectDashboardRead(t, mock, doc)

	fake := &fakeChartProvider{}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got.Results) != 2 || fake.dataCallCount() != 2 {
		t.Fatalf("results=%d 取数=%d, want 2/2", len(got.Results), fake.dataCallCount())
	}
}

// 图表联动（issue #143）
// ---------------------------------------------------------------------------

// layoutChartLinked 是一块带 linkage.targets 的 type=chart widget 字面量。
func layoutChartLinked(widgetID string, chartID int, targets ...string) string {
	return fmt.Sprintf(
		`{"widgetId":%q,"type":"chart","x":0,"y":0,"w":6,"h":4,"chartId":%d,"linkage":{"targets":[%s]}}`,
		widgetID, chartID, strings.Join(targets, ","))
}

// linkageTarget 是一条联动去向的字面量（column 为目标数据集的列 ID）。
func linkageTarget(widgetID, column string) string {
	return fmt.Sprintf(`{"widgetId":%q,"column":%q}`, widgetID, column)
}

// blockByWidget 按 widgetId 取结果块（结果顺序跟随 layout，断言不该依赖下标）。
func blockByWidget(blocks []entity.DashboardQueryBlock, widgetID string) entity.DashboardQueryBlock {
	for _, b := range blocks {
		if b.WidgetID == widgetID {
			return b
		}
	}
	return entity.DashboardQueryBlock{}
}

// linkageMeta 让每块图表都返回同一份元信息（数据集相同，便于把断言收敛在条件形状上）。
func linkageMeta(datasetID int) func(context.Context, int) (*entity.ChartQueryContext, error) {
	return func(_ context.Context, _ int) (*entity.ChartQueryContext, error) {
		return &entity.ChartQueryContext{Exists: true, DatasetID: datasetID}, nil
	}
}

// TestQueryAppliesLinkageToTargetBlock 联动只落在 layout 声明的目标块上：来源自己
// 不带任何条件（点击它不该反过来筛它自己）。
func TestQueryAppliesLinkageToTargetBlock(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChartLinked("w-1", 42, linkageTarget("w-2", "region")),
		layoutChart("w-2", 43),
	))
	fake := &fakeChartProvider{}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Linkages: []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1", Value: []any{"华东"}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	target, ok := fake.overridesFor(43)
	if !ok {
		t.Fatal("未观察到对目标块的取数")
	}
	want := []entity.Filter{{
		ID:       "link-w-1",
		Field:    "region",
		Operator: "eq",
		Value:    "华东",
		Logic:    "and",
	}}
	if !reflect.DeepEqual(target, want) {
		t.Errorf("目标块 overrides = %#v, want %#v", target, want)
	}

	source, ok := fake.overridesFor(42)
	if !ok {
		t.Fatal("未观察到对来源块的取数")
	}
	if source != nil {
		t.Errorf("来源块不该被自己的联动筛到，实际 overrides = %#v", source)
	}
	if !reflect.DeepEqual(blockByWidget(got.Results, "w-2").AppliedFields, []string{"region"}) {
		t.Errorf("目标块 appliedFields = %#v, want [region]", blockByWidget(got.Results, "w-2").AppliedFields)
	}
}

// TestQueryLinkageMultiValueUsesIn 联动取值形状按个数分流：多值 → in。
func TestQueryLinkageMultiValueUsesIn(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChartLinked("w-1", 42, linkageTarget("w-2", "region")),
		layoutChart("w-2", 43),
	))
	fake := &fakeChartProvider{}
	svc.chartProvider = fake

	if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Linkages: []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1", Value: []any{"华东", "华南"}}},
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	target, _ := fake.overridesFor(43)
	if len(target) != 1 || target[0].Operator != "in" {
		t.Fatalf("多值联动应为 in 单条，实际 %#v", target)
	}
	if !reflect.DeepEqual(target[0].Value, []any{"华东", "华南"}) {
		t.Errorf("value = %#v, want 数组两元素", target[0].Value)
	}
}

// TestQueryInactiveLinkageProducesNoOverrides 空值 / 未声明的来源都视为未激活：
// 一次取数都不该被注入条件（nil 短路路径）。
func TestQueryInactiveLinkageProducesNoOverrides(t *testing.T) {
	cases := []struct {
		name    string
		request []entity.DashboardQueryLinkage
	}{
		{"空值", []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1", Value: []any{}}}},
		{"nil 值", []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1"}}},
		{"来源不在布局里", []entity.DashboardQueryLinkage{{SourceWidgetID: "w-nope", Value: []any{"华东"}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, _ := newTestService(t)
			expectDashboardRead(t, mock, layoutDoc(
				layoutChartLinked("w-1", 42, linkageTarget("w-2", "region")),
				layoutChart("w-2", 43),
			))
			fake := &fakeChartProvider{}
			svc.chartProvider = fake

			got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
				Linkages: tc.request,
			})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			target, _ := fake.overridesFor(43)
			if target != nil {
				t.Errorf("未激活的联动不该产生条件，实际 %#v", target)
			}
			if len(blockByWidget(got.Results, "w-2").AppliedFields) != 0 {
				t.Errorf("appliedFields 应为空，实际 %#v", blockByWidget(got.Results, "w-2").AppliedFields)
			}
		})
	}
}

// TestQueryLinkageWinsOverFilterOnSameField 同字段冲突时联动优先，且只留一条条件：
// 两条同字段条件 AND 在一起会互相排斥成空集。
func TestQueryLinkageWinsOverFilterOnSameField(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChartLinked("w-1", 42, linkageTarget("w-2", "region")),
		layoutChart("w-2", 43),
		layoutFilter("f-1", 7, "region", "in"),
	))
	fake := &fakeChartProvider{metaFn: linkageMeta(7)}
	svc.chartProvider = fake

	got, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Filters:  []entity.DashboardQueryFilter{{WidgetID: "f-1", Value: []any{"华东", "华南"}}},
		Linkages: []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1", Value: []any{"华南"}}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	target, _ := fake.overridesFor(43)
	want := []entity.Filter{{
		ID:       "link-w-1",
		Field:    "region",
		Operator: "eq",
		Value:    "华南",
		Logic:    "and",
	}}
	if !reflect.DeepEqual(target, want) {
		t.Fatalf("同字段冲突应只留联动那条，实际 %#v", target)
	}

	// 来源块（42）没有联动去向，仍吃盘级筛选器。
	source, _ := fake.overridesFor(42)
	if len(source) != 1 || source[0].ID != "dash-f-1" {
		t.Errorf("来源块应保留盘级筛选条件，实际 %#v", source)
	}

	applied := blockByWidget(got.Results, "w-2").AppliedFields
	if !reflect.DeepEqual(applied, []string{"region"}) {
		t.Errorf("appliedFields 不该出现重复字段，实际 %#v", applied)
	}
}

// TestQueryLinkageAcrossDatasets 联动不受「目标数据集必须等于来源数据集」约束：
// 目标列来自目标自己的数据集（跨数据集靠用户在联动设置里选列，而不是靠列名撞名）。
func TestQueryLinkageAcrossDatasets(t *testing.T) {
	svc, mock, _ := newTestService(t)
	expectDashboardRead(t, mock, layoutDoc(
		layoutChartLinked("w-1", 42, linkageTarget("w-2", "city")),
		layoutChart("w-2", 43),
	))
	fake := &fakeChartProvider{metaFn: func(_ context.Context, id int) (*entity.ChartQueryContext, error) {
		datasetID := 7
		if id == 43 {
			datasetID = 9
		}
		return &entity.ChartQueryContext{Exists: true, DatasetID: datasetID}, nil
	}}
	svc.chartProvider = fake

	if _, err := svc.Query(context.Background(), testID, entity.DashboardQueryRequest{
		Linkages: []entity.DashboardQueryLinkage{{SourceWidgetID: "w-1", Value: []any{"杭州"}}},
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	target, _ := fake.overridesFor(43)
	if len(target) != 1 || target[0].Field != "city" || target[0].Value != "杭州" {
		t.Fatalf("跨数据集联动应落在目标列上，实际 %#v", target)
	}
	source, _ := fake.overridesFor(42)
	if source != nil {
		t.Errorf("来源块不该被联动筛到，实际 %#v", source)
	}
}

// TestProjectLinkageTargetsDropsUnusableAndDedupes 投影层丢弃「声明不了条件」的去向
// （缺 widgetId / 缺列 ID），并让同一目标的重复声明只留首条。
func TestProjectLinkageTargetsDropsUnusableAndDedupes(t *testing.T) {
	layout := layoutDoc(layoutChartLinked("w-1", 42,
		linkageTarget("", "region"),
		linkageTarget("w-2", ""),
		linkageTarget("w-2", "region"),
		linkageTarget("w-2", "city"),
	))
	got := projectLayout(layout, "")

	if len(got.Charts) != 1 {
		t.Fatalf("期望 1 块图表，实际 %d", len(got.Charts))
	}
	want := []layoutLinkageTarget{{WidgetID: "w-2", Column: "region"}}
	if !reflect.DeepEqual(got.Charts[0].LinkageTargets, want) {
		t.Errorf("linkageTargets = %#v, want %#v", got.Charts[0].LinkageTargets, want)
	}
}

// TestMergeOverridesKeepsFilterPosition filter 与联动打在不同字段时两条都留，
// 且盘级筛选器那条保持原位置（前端「盘级条件在前」的阅读顺序不变）。
func TestMergeOverridesKeepsFilterPosition(t *testing.T) {
	filters := []entity.Filter{{ID: "dash-f-1", Field: "region", Operator: "eq", Value: "华东", Logic: "and"}}
	linkages := []entity.Filter{{ID: "link-w-1", Field: "city", Operator: "eq", Value: "杭州", Logic: "and"}}

	merged, applied := mergeOverrides(filters, linkages)
	want := append(append([]entity.Filter{}, filters...), linkages...)
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged = %#v, want %#v", merged, want)
	}
	if !reflect.DeepEqual(applied, []string{"region", "city"}) {
		t.Errorf("applied = %#v, want [region city]", applied)
	}
}

// TestMergeOverridesEmptyLinkagesReturnsFiltersAsIs 没有联动时原样返回盘级条件
// （含 nil 短路：无条件的块仍走 provider 的 nil 语义）。
func TestMergeOverridesEmptyLinkagesReturnsFiltersAsIs(t *testing.T) {
	merged, applied := mergeOverrides(nil, nil)
	if merged != nil || applied != nil {
		t.Fatalf("空输入应原样返回 nil，实际 %#v / %#v", merged, applied)
	}

	filters := []entity.Filter{{ID: "dash-f-1", Field: "region", Operator: "eq", Value: "华东", Logic: "and"}}
	merged, applied = mergeOverrides(filters, nil)
	if !reflect.DeepEqual(merged, filters) {
		t.Errorf("merged = %#v, want 入参本身", merged)
	}
	if !reflect.DeepEqual(applied, []string{"region"}) {
		t.Errorf("applied = %#v, want [region]", applied)
	}
}
