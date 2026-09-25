package dashboard

import (
	"context"
	"encoding/json"
	"sync"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
)

// chartDataProvider 是 service/chart 的窄接口：本包只需要「查一块图表的元信息」与
// 「带覆盖条件取数」两件事。走接口而不是直接 import service/chart，一是避免
// service→service 的编译耦合（dashboard 的 CRUD 路径不该因为 chart 包变大而牵连），
// 二是单测能塞假实现，不必连真库、也不必造 chart 服务的一整套依赖。
type chartDataProvider interface {
	ChartQueryContext(ctx context.Context, id int) (*entity.ChartQueryContext, error)
	GetDataWithFilterOverrides(ctx context.Context, id int, overrides []entity.Filter) (entity.ChartDataResult, error)
}

// dashboardQueryConcurrency 是「单盘 N 块并发取数」的并发上限（PRD §6.3 / §11-6）。
// 一个盘里几十块图表各自要连数据源执行 SQL，无上限会把数据源连接池打满；6 是取
// 「一个中等盘（~10 块）两轮跑完」与「同时打满 6 个连接」之间的折中。
const dashboardQueryConcurrency = 6

// SetChartProvider 注入图表取数依赖（由 cmd/routes.go 在装配期调用一次）。
//
// 用独立 setter 而不是 NewService 的入参，是为了不动既有构造签名（12 个既有测试
// 与部署侧装配都依赖它），并让「未接线」成为一个可表达的状态：nil 时 Query 返回
// 业务错误而不是 panic。
func (s *dashboardService) SetChartProvider(p chartDataProvider) {
	s.chartProvider = p
}

// ---------------------------------------------------------------------------
// layout_json 的防御性投影
//
// ⚠️ layout 文档的权威定义与迁移都在前端（migrateDashboardLayout 版本化 + 前端迁移，
// PRD §6.2）：后端这里刻意只做「投影解析」——只声明本次取数真正要读的字段，不认识的
// 字段一律忽略，也绝不因为多出字段而报错（与 service/chart 的 chartConfigV1 /
// chartConfigV2 同款做法）。
//
// 另一处刻意的缺席：**投影里没有 defaultValue**。请求是筛选器取值的唯一真相源，
// 请求里没出现的筛选器一律视为未激活；不把 defaultValue 读进来，就从结构上排除了
// 「回落 layout 里的历史默认值」这条隐形路径（否则一打开盘就会莫名套用某个旧默认值）。
// ---------------------------------------------------------------------------

// dashboardLayoutDoc 是 layout_json 的最小投影：只要 widgets。
type dashboardLayoutDoc struct {
	Widgets []dashboardLayoutWidget `json:"widgets"`
}

// dashboardLayoutWidget 是三态 widget（chart | text | filter）的扁平投影，
// 只声明取数用得上的键；type=text 的块自然被忽略。
type dashboardLayoutWidget struct {
	WidgetID string `json:"widgetId"`
	Type     string `json:"type"`
	// ChartID：type=chart 的图表引用。
	ChartID int `json:"chartId"`
	// Binding：type=filter 的绑定字段（(datasetId, column) 二元组，PRD §11-2）。
	Binding *struct {
		DatasetID int    `json:"datasetId"`
		Column    string `json:"column"`
	} `json:"binding"`
	// Operator：type=filter 的算子（与图表过滤算子同一词表）。
	Operator string `json:"operator"`
}

// layoutChartBlock 是投影出的一块待取数图表：widgetId 用于归位与建筛选索引，
// chartId 用于取数。
type layoutChartBlock struct {
	WidgetID string
	ChartID  int
}

// layoutFilterBinding 是一块已激活的筛选器绑定：属于哪个 widget、绑到哪个数据集
// 的哪一列、用什么算子。
type layoutFilterBinding struct {
	WidgetID  string
	DatasetID int
	Column    string
	Operator  string
}

// layoutProjection 是 layout 投影解析的结果。
type layoutProjection struct {
	// Charts 只含 type=chart 的块，顺序与 layout 一致（结果切片按下标写回，
	// 因此响应顺序也与此一致）。
	Charts []layoutChartBlock
	// Filters 只含 type=filter 且 binding.column 非空的块——没有列名的绑定
	// 认领不了任何条件，等同于未绑定。
	Filters []layoutFilterBinding
}

// projectLayout 解析 layout_json 并把两类块分拣出来。
//
// 任何解析失败（空串 / 损坏 JSON / 顶层不是对象 / widgets 不是数组）都返回零值投影，
// **不返回错误**：layout 归前端所有，后端读不懂只该表现为「这一盘没有可取的块」，
// 而不该让取数端点整体 500。
func projectLayout(raw string) layoutProjection {
	var doc dashboardLayoutDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return layoutProjection{}
	}

	var out layoutProjection
	for _, w := range doc.Widgets {
		switch w.Type {
		case "chart":
			out.Charts = append(out.Charts, layoutChartBlock{WidgetID: w.WidgetID, ChartID: w.ChartID})
		case "filter":
			if w.Binding == nil || w.Binding.Column == "" {
				continue
			}
			out.Filters = append(out.Filters, layoutFilterBinding{
				WidgetID:  w.WidgetID,
				DatasetID: w.Binding.DatasetID,
				Column:    w.Binding.Column,
				Operator:  w.Operator,
			})
		}
	}
	return out
}

// Query 实现 POST /api/dashboards/{id}/query：逐块取数 + 后端筛选合并（PRD §6.3/§8.3）。
//
// 语义要点：
//   - 单块失败绝不让整盘失败：除「盘不存在」「未接线」外，一切问题都落在块级 status；
//   - 并发上限 dashboardQueryConcurrency，结果按下标写回切片以保持与 layout 同序；
//   - 图表不存在 / 已软删的块不取数（省掉一次必然失败的数据源连接）。
func (s *dashboardService) Query(
	ctx context.Context, id string, in entity.DashboardQueryRequest,
) (*entity.DashboardQueryResult, error) {
	if s.chartProvider == nil {
		// 装配期漏接线是服务端问题（不是调用方的错），但在 handler 层必须表现为
		// 结构化错误而不是 panic 掉整个进程。
		return nil, router.NewBusinessError(response.CodeInternalError, "图表取数服务未接线")
	}

	// 盘不存在 / 已软删 → Get 已经翻译成 404 业务错误，这里原样上抛。
	dash, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	projection := projectLayout(dash.LayoutJSON)
	result := &entity.DashboardQueryResult{
		Results: make([]entity.DashboardQueryBlock, len(projection.Charts)),
	}
	if len(projection.Charts) == 0 {
		return result, nil
	}

	// 请求是取值的唯一真相源：按 widgetId 建索引，请求里没出现的筛选器就是未激活。
	values := make(map[string][]any, len(in.Filters))
	for _, f := range in.Filters {
		values[f.WidgetID] = f.Value
	}

	sem := make(chan struct{}, dashboardQueryConcurrency)
	var wg sync.WaitGroup
	for i := range projection.Charts {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			// 按下标写回（每块只写自己那一格），因此无需额外加锁，结果顺序
			// 恒等于 layout 顺序。
			result.Results[i] = s.queryBlock(ctx, projection.Filters, projection.Charts[i], values)
		}(i)
	}
	wg.Wait()

	return result, nil
}

// queryBlock 取一块的数据：先判三态，再算合并条件，最后取数。任何失败都收敛成块级
// status/message，绝不返回 error。
func (s *dashboardService) queryBlock(
	ctx context.Context, filters []layoutFilterBinding, block layoutChartBlock, values map[string][]any,
) entity.DashboardQueryBlock {
	out := entity.DashboardQueryBlock{WidgetID: block.WidgetID, ChartID: block.ChartID}

	meta, err := s.chartProvider.ChartQueryContext(ctx, block.ChartID)
	if err != nil {
		return blockError(out, err)
	}
	// provider 契约上不该返回 (nil, nil)，但这一层是进程内可替换依赖的边界：
	// 与其让一次实现失误把整个盘带崩，不如照「图表不存在」处理。
	if meta == nil || !meta.Exists {
		out.Status = entity.DashboardBlockChartMissing
		return out
	}
	if meta.Deleted {
		out.Status = entity.DashboardBlockChartDeleted
		return out
	}

	overrides, appliedFields := buildOverrides(filters, values, meta.DatasetID)

	var data entity.ChartDataResult
	if len(overrides) == 0 {
		// 无适用筛选器时传 nil：让 chart 侧走「不做任何覆盖」的短路，语义更明确。
		data, err = s.chartProvider.GetDataWithFilterOverrides(ctx, block.ChartID, nil)
	} else {
		data, err = s.chartProvider.GetDataWithFilterOverrides(ctx, block.ChartID, overrides)
	}
	if err != nil {
		return blockError(out, err)
	}

	out.Status = entity.DashboardBlockOK
	out.Data = &data
	out.AppliedFields = appliedFields
	out.OverriddenFields = intersectFields(appliedFields, meta.OwnFilterFields)
	return out
}

// blockError 把一次失败折叠成该块的 error 状态：Data 恒为 nil，原因进 Message。
func blockError(out entity.DashboardQueryBlock, err error) entity.DashboardQueryBlock {
	out.Status = entity.DashboardBlockError
	out.Message = err.Error()
	out.Data = nil
	return out
}

// buildOverrides 实现 PRD §8.3 的输入侧：从盘级筛选器里挑出对该图表数据集适用的、
// 且**已激活**的那些，翻成 chart 侧认识的覆盖条件。
//
// 请求里的值恒为数组（`DashboardQueryFilter.value: []`），但**下发到 builder 的形状必须按算子分流**，
// 否则 SQL 形状是错的：
//   - `in` / `notIn` 要数组：builder 按元素展开成 `IN (?, ?, …)`；
//   - `between` 要**两个标量**（`Value` + `ValueEnd`）；
//   - 其余标量算子（`eq`/`neq`/`gt`/`gte`/`lt`/`lte`/`like`）要**单个标量**：builder 直接
//     `append(f.Value)`，塞数组进去会渲染成 `col = ARRAY[...]` → PG 报 42883。
//     ⚠️ 这条曾长期是缺陷：既有测试只断言算子字符串，取值形状零覆盖。
//   - `isNull` / `isNotNull` 不看值，只靠「数组非空」表达「已激活」。
//
// 四条规则：
//  1. 适用条件是 (binding.datasetId == 图表数据集) —— 产品负责人确认的二元组口径
//     （PRD §11-2）：纯按列名匹配会跨数据集误伤同名同义字段（比如两个数据集都有
//     region），而盘允许混搭多数据集（D12）。
//  2. 值为空数组或请求里没出现 = 未激活，跳过，不产生覆盖、也不计入 appliedFields。
//     否则拖入一个筛选器就会莫名抹掉图表的默认条件（PRD §8.3 步骤 2 的警告）。
//  3. `between` 的拆分必须排在规则 4 之前：否则二元组会被「多值降级为 in」吃掉，区间变成 IN 两元素。
//  4. 多选（值长度 > 1）且算子是标量算子时按 in 处理：布局里的算子可能是在单选语境下配的，
//     而请求给了多个值，语义只能是「属于其中之一」。
func buildOverrides(
	filters []layoutFilterBinding, values map[string][]any, datasetID int,
) ([]entity.Filter, []string) {
	var overrides []entity.Filter
	var applied []string
	seen := make(map[string]struct{}, len(filters))

	for _, f := range filters {
		if f.DatasetID != datasetID {
			continue
		}
		value := values[f.WidgetID]
		if len(value) == 0 {
			continue
		}

		operator := f.Operator
		// 默认（标量算子 + 单值）取第一个元素；其余分支各自覆盖。
		var valueArg, valueEndArg any = value[0], nil
		switch {
		case operator == "between" && len(value) == 2:
			valueArg, valueEndArg = value[0], value[1]
		case operator == "in" || operator == "notIn":
			valueArg = value
		case len(value) > 1:
			operator = "in"
			valueArg = value
		}

		overrides = append(overrides, entity.Filter{
			// 稳定的 id 只为排查方便（该 Filter 不落库、只活一次查询）。
			ID:       "dash-" + f.WidgetID,
			Field:    f.Column,
			Operator: operator,
			Value:    valueArg,
			ValueEnd: valueEndArg,
			Logic:    "and",
		})

		if _, dup := seen[f.Column]; !dup {
			seen[f.Column] = struct{}{}
			applied = append(applied, f.Column)
		}
	}
	return overrides, applied
}

// intersectFields 返回 appliedFields 中同时出现在图表自身过滤字段里的那些（去重保序）
// —— 即 PRD §11-1a 的「覆盖可见标识」：这些字段的图表原始条件已被盘级条件整条丢弃。
func intersectFields(applied, own []string) []string {
	if len(applied) == 0 || len(own) == 0 {
		return nil
	}
	ownSet := make(map[string]struct{}, len(own))
	for _, f := range own {
		ownSet[f] = struct{}{}
	}
	var out []string
	for _, f := range applied {
		if _, hit := ownSet[f]; hit {
			out = append(out, f)
		}
	}
	return out
}
