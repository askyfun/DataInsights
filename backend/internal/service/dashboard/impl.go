// Package dashboard 实现仪表盘的持久化与读取（PRD prd-dashboard-v1-2026-09-25.md
// 的 M1：数据契约与骨架）。
//
// 职责边界：bi_dashboard 这一张表的 CRUD、 「谁引用了某张图表」的反查，以及
// POST /api/dashboards/{id}/query 的批量取数（query.go；筛选合并算法见那里）。
// layout_json 的权威解析仍归前端 migrateDashboardLayout，后端只做防御性投影读取。
//
// 两条与 PRD 一致的生命周期语义：
//   - 永不物理删除，Delete 只打 deleted_at，且所有读路径恒带 deleted_at IS NULL；
//   - 仪表盘对图表是引用关系而非父子行关系，所以本包不做任何级联——图表软删时
//     bi_dashboard 一行都不动，由前端按 chart_deleted 渲染占位块（PRD §6.4）。
package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/response"
	"data-insights/internal/router"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// defaultListLimit 是 List 的缺省页大小：调用方给不出正数时兜底，避免无界扫描。
const defaultListLimit = 100

// referencingChartPath 是「某盘是否嵌入了图表 N」的 jsonpath。图表 id 以 %d 注入
// （整型，无注入面），整个路径再作为绑定参数交给 PG 的 @? 存在性运算符。
const referencingChartPath = `$.widgets[*] ? (@.chartId == %d)`

// Service defines the interface for dashboard operations
type Service interface {
	// CRUD operations
	List(ctx context.Context, limit, offset int) ([]entity.Dashboard, error)
	Get(ctx context.Context, id string) (*entity.Dashboard, error)
	Create(ctx context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error)
	Update(ctx context.Context, id string, in entity.DashboardUpdateRequest) (*entity.Dashboard, error)
	Delete(ctx context.Context, id string) error

	// CountChartReferences returns the not-soft-deleted dashboards whose layout
	// embeds the given chart. Used by GET /api/charts/{id}/references.
	CountChartReferences(ctx context.Context, chartID int) ([]entity.Dashboard, error)

	// Query 是 POST /api/dashboards/{id}/query 的业务实现：按 layout 逐块取数，
	// 并在后端完成盘级筛选与图表自身筛选的合并（PRD §8.3）。筛选合并算法与
	// 逐块取数的细节见 query.go。
	Query(ctx context.Context, id string, in entity.DashboardQueryRequest) (*entity.DashboardQueryResult, error)

	// SetChartProvider injects the chart data provider Query needs. It is a
	// separate setter (rather than a NewService parameter) for the same reason
	// datasource.SetSecurityKey is: the dependency is optional at construction
	// time and nil means "not wired", which Query reports as a business error
	// instead of panicking.
	SetChartProvider(p chartDataProvider)
}

// dashboardService implements the Service interface
type dashboardService struct {
	db *bun.DB
	// chartProvider 是 service/chart 的窄接口替身（见 query.go 的 chartDataProvider）：
	// 走接口而非具体包，既避免 service→service 的编译耦合，也让单测能塞假实现。
	// nil 表示尚未接线。
	chartProvider chartDataProvider
	// now / newID 是注入点：让单测能固定时间与 id，不依赖真实时钟与随机源
	// （与 service/queryrecord 同款写法）。
	now   func() time.Time
	newID func() (uuid.UUID, error)
}

// NewService creates a new dashboard service
func NewService(db *bun.DB) Service {
	return &dashboardService{
		db:    db,
		now:   func() time.Time { return time.Now().UTC() },
		newID: func() (uuid.UUID, error) { return uuid.NewV7() },
	}
}

// List returns the not-soft-deleted dashboards, newest first.
//
// The ordering (created_at DESC, id DESC) matches the partial index
// idx_dashboard_not_deleted defined in migration 00005; id is the tiebreaker so
// the page stays stable when several rows share a timestamp.
func (s *dashboardService) List(ctx context.Context, limit, offset int) ([]entity.Dashboard, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	var rows []model.Dashboard
	q := s.db.NewSelect().Model(&rows).
		Where("deleted_at IS NULL").
		OrderExpr("created_at DESC, id DESC").
		Limit(limit)
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list dashboards: %w", err)
	}
	return toDashboardEntityList(rows), nil
}

// Get returns a dashboard by its UUID.
//
// A missing (or soft-deleted) row is a NotFound business error; any other
// failure stays an internal error so a database outage is not reported as 404.
func (s *dashboardService) Get(ctx context.Context, id string) (*entity.Dashboard, error) {
	m, err := s.getModel(ctx, id)
	if err != nil {
		return nil, err
	}
	return toDashboardEntity(m), nil
}

// Create persists a new dashboard. The id is a UUIDv7 generated here (never an
// auto-increment integer, PRD §6.5); status defaults to "draft" and layout_json
// to the valid empty v1 document so no reader ever has to handle an empty one.
func (s *dashboardService) Create(ctx context.Context, in entity.DashboardCreateRequest) (*entity.Dashboard, error) {
	if err := validateLayoutJSON(in.LayoutJSON); err != nil {
		return nil, err
	}

	id, err := s.newID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate dashboard id: %w", err)
	}
	now := s.now()
	m := &model.Dashboard{
		ID:          id.String(),
		Name:        in.Name,
		Description: nullableDescription(in.Description),
		LayoutJSON:  defaultLayoutJSON(in.LayoutJSON),
		Status:      defaultStatus(in.Status),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create dashboard: %w", err)
	}
	return toDashboardEntity(m), nil
}

// Update rewrites an existing dashboard following the repository's
// "unprovided fields are preserved" convention: a nil pointer keeps the stored
// value instead of zeroing it. updated_at is stamped explicitly because the
// table has no trigger and the column default only applies on INSERT.
func (s *dashboardService) Update(ctx context.Context, id string, in entity.DashboardUpdateRequest) (*entity.Dashboard, error) {
	m, err := s.getModel(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.LayoutJSON != nil {
		if err := validateLayoutJSON(*in.LayoutJSON); err != nil {
			return nil, err
		}
		m.LayoutJSON = *in.LayoutJSON
	}
	if in.Name != nil {
		m.Name = *in.Name
	}
	if in.Description != nil {
		m.Description = nullableDescription(in.Description)
	}
	if in.Status != nil {
		m.Status = *in.Status
	}
	m.UpdatedAt = s.now()

	// ⚠️ 整行 WherePK 更新必须 ExcludeColumn("deleted_at")：模型里的 DeletedAt 是
	// 零值（本就未读/未设置），让它进 SET 子句会写出 NULL，把已软删的行当场复活
	// （docs/pitfalls.md T-9）。WHERE 上的 deleted_at IS NULL 同时兜住「更新已删行」。
	if _, err := s.db.NewUpdate().Model(m).
		WherePK().
		Where("deleted_at IS NULL").
		ExcludeColumn("deleted_at").
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update dashboard: %w", err)
	}
	return toDashboardEntity(m), nil
}

// Delete soft-deletes a dashboard: it stamps deleted_at and never removes the
// row.
//
// It deliberately does not go through a full-row UPDATE — an explicit
// `SET deleted_at = now()` cannot resurrect an already deleted row with zero
// values, which is the failure mode documented as T-9 in docs/pitfalls.md.
// Re-deleting (0 rows affected) is idempotent and answers no error.
func (s *dashboardService) Delete(ctx context.Context, id string) error {
	if _, err := s.db.NewUpdate().
		Model((*model.Dashboard)(nil)).
		Set("deleted_at = now()").
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete dashboard: %w", err)
	}
	return nil
}

// CountChartReferences returns every not-soft-deleted dashboard that embeds the
// given chart, newest first.
//
// The lookup is a jsonpath existence test (`layout_json @? …`) so the
// jsonb_path_ops GIN index from migration 00005 applies; a `layout_json::text
// LIKE '%"chartId":N%'` scan is forbidden because reformatting or a substring
// elsewhere in the document would silently produce false hits (PRD §6.4).
func (s *dashboardService) CountChartReferences(ctx context.Context, chartID int) ([]entity.Dashboard, error) {
	// ⚠️ bun 把所有 `?` 都当占位符（连单引号字符串里的也不放过），所以不能把
	// `@?` 或 jsonpath 的过滤问号直接写进 SQL 文本：bun 会把它们替换成实参。
	// 正解是 `@\?`（bun 认识的反斜杠转义，输出仍是 `@?`）＋把整条 jsonpath 作为
	// 绑定参数传入。sentinel 测试 TestCountChartReferencesUsesJSONPath 钉死这条 SQL。
	path := fmt.Sprintf(referencingChartPath, chartID)

	var rows []model.Dashboard
	if err := s.db.NewSelect().Model(&rows).
		Where("deleted_at IS NULL").
		Where(`layout_json @\? ?::jsonpath`, path).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to find dashboards referencing chart: %w", err)
	}
	return toDashboardEntityList(rows), nil
}

// getModel reads one not-soft-deleted row, translating "no rows" into a
// NotFound business error.
func (s *dashboardService) getModel(ctx context.Context, id string) (*model.Dashboard, error) {
	m := &model.Dashboard{ID: id}
	err := s.db.NewSelect().Model(m).WherePK().Where("deleted_at IS NULL").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeNotFound, "dashboard not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard: %w", err)
	}
	return m, nil
}

// validateLayoutJSON rejects text that is not valid JSON. The column is JSONB,
// so malformed input would otherwise surface as an opaque 500 from PostgreSQL.
// An empty string is allowed: it means "not provided" and the default document
// is used.
func validateLayoutJSON(raw string) error {
	if raw == "" {
		return nil
	}
	if !json.Valid([]byte(raw)) {
		return router.NewBusinessError(response.CodeBadRequest, "layout_json 必须是合法 JSON")
	}
	return nil
}

func defaultLayoutJSON(raw string) string {
	if raw == "" {
		return entity.DashboardDefaultLayoutJSON
	}
	return raw
}

func defaultStatus(raw string) string {
	if raw == "" {
		return entity.DashboardStatusDraft
	}
	return raw
}

// nullableDescription maps nil/empty to NULL. An empty description carries no
// meaning, so storing "" would only create a second way to say "nothing".
func nullableDescription(value *string) sql.NullString {
	if value == nil || *value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

// Conversion functions

func toDashboardEntity(m *model.Dashboard) *entity.Dashboard {
	e := &entity.Dashboard{
		ID:         m.ID,
		Name:       m.Name,
		LayoutJSON: m.LayoutJSON,
		Status:     m.Status,
		CreatedAt:  m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  m.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if m.Description.Valid {
		e.Description = &m.Description.String
	}
	return e
}

func toDashboardEntityList(models []model.Dashboard) []entity.Dashboard {
	result := make([]entity.Dashboard, len(models))
	for i, m := range models {
		result[i] = *toDashboardEntity(&m)
	}
	return result
}
