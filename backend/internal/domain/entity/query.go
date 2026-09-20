package entity

import "encoding/json"

// CurrentQuerySpecVersion 是 bi_query.spec_json 顶层信封的当前版本。
//
// spec_json 的结构是 {"v":1,"document":{...}}：外层 v 管「记录级」schema 演进，
// 内层 document 是前端的 ChartConfigDocument（自带 version:2 与迁移函数），
// 两套版本互不干扰——记录级将来要加兄弟字段（如字段快照）时不必动图表文档。
const CurrentQuerySpecVersion = 1

// QuerySpecEnvelope 是 bi_query.spec_json 的顶层信封。
type QuerySpecEnvelope struct {
	V int `json:"v"`
	// Document 故意用 json.RawMessage：图表文档的 schema 演进归前端
	// chartConfigSchema.migrateChartConfig 管，后端只做信封校验与透传，
	// 不复制一份会漂移的类型定义。
	Document json.RawMessage `json:"document"`
}

// QueryRecordSaveRequest 是 POST /api/queries 的业务入参。
type QueryRecordSaveRequest struct {
	DatasetID int `json:"dataset_id"`
	// ChartID 为 0 表示不关联图表（草稿查询）。
	ChartID int `json:"chart_id"`
	// Spec 是完整的 spec_json 原文（信封形态）。
	Spec json.RawMessage `json:"spec"`
	// SourceType 取值 build / share_url，仅作留痕。
	SourceType string `json:"source_type"`
	// RowCount / DurationMs 用指针区分「未上报」与「上报 0」：未上报时去重
	// 分支必须保留存量值，而不是把已有元信息覆盖成 0。
	RowCount   *int `json:"row_count"`
	DurationMs *int `json:"duration_ms"`
}

// QueryRecordSaved 是落库成功后的回执，只含地址栏需要的三件事。
type QueryRecordSaved struct {
	// QueryID 是出地址栏的形态（base58 短码），不是数据库里的 uuid 原文。
	QueryID   string `json:"query_id"`
	CreatedAt string `json:"created_at"`
	// ExpiresAt 为 nil 表示永久（NULL 语义）。
	ExpiresAt *string `json:"expires_at"`
}

// QueryRecord 是 GET /api/queries/{q} 的读数模型。
//
// 不含 ip / spec_hash / owner_id / tenant_id 这些内部列：前者是留痕字段，
// 后者是账号体系的预留列（本期无值），都不该出现在对外的分享读出口。
type QueryRecord struct {
	QueryID   string            `json:"query_id"`
	Spec      QuerySpecEnvelope `json:"spec"`
	DatasetID int               `json:"dataset_id"`
	ChartID   *int              `json:"chart_id"`
	CreatedAt string            `json:"created_at"`
	ExpiresAt *string           `json:"expires_at"`
	// Expired 是服务端按 expires_at 单字段派生的判定结果（NULL = 永久 = 未过期），
	// 客户端不重复实现这个口径。
	Expired  bool `json:"expired"`
	HitCount int  `json:"hit_count"`
}
