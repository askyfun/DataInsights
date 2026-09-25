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
