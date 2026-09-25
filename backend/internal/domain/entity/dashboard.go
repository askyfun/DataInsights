package entity

// DashboardDefaultLayoutJSON 是 layout_json 的缺省文档：合法的 v1 空文档
// （PRD §6.2）。给出它而不是留 NULL，是为了让读取侧没有「空文档」分支。
const DashboardDefaultLayoutJSON = `{"version":1,"widgets":[]}`

// DashboardStatusDraft 是新建仪表盘的缺省状态（PRD R-73）。v1 只走 draft：
// 无分享受众，UI 不暴露发布切换，status 列只为不可逆归属与将来分享预留。
const DashboardStatusDraft = "draft"

// Dashboard represents a dashboard: a 12-column free grid that composes
// EXISTING charts into one page. It is the outward entity (the wire shape),
// not the bun model.
//
// LayoutJSON is the layout document's string form (the column is JSONB, the
// API surface is a string) — same convention as Chart.Config.
type Dashboard struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	LayoutJSON  string  `json:"layout_json"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// DashboardCreateRequest is the business input of POST /api/dashboards.
// LayoutJSON / Status may be empty; the service fills in the defaults
// (an empty v1 document / "draft").
type DashboardCreateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	LayoutJSON  string  `json:"layout_json"`
	Status      string  `json:"status"`
}

// DashboardUpdateRequest is the business input of PUT /api/dashboards/{id}.
//
// Every field is a pointer on purpose: this endpoint follows the repository's
// "unprovided fields are preserved" convention (the same rule datasource
// applies to an omitted password and dataset to optional metadata). A nil
// pointer means the key was absent (or explicitly null) and must keep the
// stored value. Description has one extra reachable state: a non-nil empty
// string clears it back to NULL, because JSON cannot distinguish "absent" from
// "null" once both decode to nil.
type DashboardUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	LayoutJSON  *string `json:"layout_json"`
	Status      *string `json:"status"`
}

// ChartReference is the payload of GET /api/charts/{id}/references: how many
// dashboards embed the chart, and which ones (the delete-time hint of D2 — the
// hint never blocks the deletion).
type ChartReference struct {
	Count      int                  `json:"count"`
	Dashboards []ChartReferenceItem `json:"dashboards"`
}

// ChartReferenceItem is the minimal dashboard identity needed by that hint.
type ChartReferenceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// 逐块取数状态常量（PRD §6.3）。除 ok 外的三态都是为了让「块不消失，但明确
// 说明为什么没数据」——图表软删后 widget 保留原位渲染占位（D2），而不是静默
// 报空。
const (
	// DashboardBlockOK 该块取数成功，Data 非 nil。
	DashboardBlockOK = "ok"
	// DashboardBlockChartDeleted 图表已软删：不再取数，前端渲染「图表已删除」占位。
	DashboardBlockChartDeleted = "chart_deleted"
	// DashboardBlockChartMissing chart_id 在 bi_chart 里从未存在：同样不再取数。
	DashboardBlockChartMissing = "chart_missing"
	// DashboardBlockError 该块取数失败（数据集软删 / 数据源不可达 / SQL 报错…）。
	// 单块失败绝不让整盘失败。
	DashboardBlockError = "error"
)

// DashboardQueryFilter 是单个盘级筛选器的当前取值（POST /api/dashboards/{id}/query
// 请求体的一项）。value 空数组或该项缺失 = **未激活**：未激活的筛选器不参与合并、
// 不触发覆盖（PRD §8.3 步骤 2）。
type DashboardQueryFilter struct {
	WidgetID string `json:"widgetId"`
	Value    []any  `json:"value"`
}

// DashboardQueryRequest 是批量取数端点的请求体（PRD §6.3）。前端只下发筛选器的
// 当前值，不解析 chart config、不下发合并结果：筛选合并是后端单点逻辑（可测）。
type DashboardQueryRequest struct {
	Filters []DashboardQueryFilter `json:"filters"`
}

// DashboardQueryBlock 是单块的取数结果。Data 为 nil 时 JSON 输出 null（与
// openapi.yaml 的 DashboardQueryResult 一致：chart_deleted / chart_missing /
// error 三态都不带数据）。
//
// AppliedFields / OverriddenFields 都是「覆盖可见标识」（PRD §11-1a）：
// appliedFields 是真正生效的盘级筛选字段，overriddenFields 是其中覆盖掉了图表
// 自身同字段条件的那些。
type DashboardQueryBlock struct {
	WidgetID         string           `json:"widgetId"`
	ChartID          int              `json:"chartId"`
	Status           string           `json:"status"`
	Data             *ChartDataResult `json:"data"`
	AppliedFields    []string         `json:"appliedFields,omitempty"`
	OverriddenFields []string         `json:"overriddenFields,omitempty"`
	Message          string           `json:"message,omitempty"`
}

// DashboardQueryResult 是批量取数端点的业务负载：results 与 layout 的图表块同序
// （前端仍按 widgetId 归位，稳定顺序只为便于排查）。
type DashboardQueryResult struct {
	Results []DashboardQueryBlock `json:"results"`
}

// ChartQueryContext 汇总仪表盘取一块所需的图表元信息：行是否存在 / 是否已软删
// （决定占位状态）、所属数据集（决定盘级筛选是否适用）、以及图表自身过滤条件用到的
// 列名（决定 overriddenFields 可见标识）。见 service/chart 的 ChartQueryContext
// 方法注释。
type ChartQueryContext struct {
	Exists          bool     `json:"exists"`
	Deleted         bool     `json:"deleted"`
	DatasetID       int      `json:"dataset_id"`
	OwnFilterFields []string `json:"own_filter_fields"`
}
