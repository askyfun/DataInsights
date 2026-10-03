// Package alert 实现指标预警（issue #155）：规则的 CRUD 与后台定时评估。
//
// 三块职责各占一个文件：
//   - impl.go：规则 CRUD（软删、「未提供则保留」、chart 存在性校验）+ 触发记录读出口；
//   - evaluator.go：后台 goroutine 定时轮询 enabled 规则，逐条取数评估；
//   - notifier.go：Notifier 抽象 + SMTP 邮件实现。
//
// 与 dashboard 同款的两条红线：读路径恒带 `deleted_at IS NULL`；永不物理删除。
// chart 取数复用走 dashboard/query.go 的窄接口 + SetChartProvider 注入模式，
// 不与 service/chart 编译耦合，单测可塞假实现。
package alert

import (
	"context"
	"database/sql"
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

// Service 定义预警规则的对外接口（handler 只依赖本接口）。
type Service interface {
	// ListRules returns every not-soft-deleted rule, oldest first.
	ListRules(ctx context.Context) ([]entity.AlertRule, error)
	// GetRule returns one rule by UUID (missing/soft-deleted → 20300).
	GetRule(ctx context.Context, id string) (*entity.AlertRule, error)
	// CreateRule persists a new rule (enabled defaults to true).
	CreateRule(ctx context.Context, in entity.AlertRuleCreateRequest) (*entity.AlertRule, error)
	// UpdateRule merges fields per the "unprovided fields are preserved" convention.
	UpdateRule(ctx context.Context, id string, in entity.AlertRuleUpdateRequest) (*entity.AlertRule, error)
	// DeleteRule soft-deletes a rule (idempotent). Trigger history is kept.
	DeleteRule(ctx context.Context, id string) error
	// ListTriggers returns the rule's trigger history (newest first, capped).
	// A missing rule answers 20300.
	ListTriggers(ctx context.Context, id string) ([]entity.AlertTrigger, error)

	// SetChartProvider injects the chart data provider the evaluator needs.
	// It is a separate setter rather than a NewService parameter so existing
	// constructors/tests stay untouched and "not wired" remains expressible.
	SetChartProvider(p chartDataProvider)
	// SetNotifier injects the trigger notification channel; nil keeps the
	// "not configured" state (triggers still recorded, notify_error explains).
	SetNotifier(n Notifier)
	// RunEvaluator blocks polling enabled rules every evalInterval until ctx
	// is cancelled. cmd/main.go starts it as a goroutine.
	RunEvaluator(ctx context.Context, evalInterval time.Duration)
}

// triggerHistoryLimit 是触发历史读出口的单次上限（第一期不分页，倒序截断）。
const triggerHistoryLimit = 50

// alertService implements Service.
type alertService struct {
	db *bun.DB
	// now / newID 是与 folderService 同款的测试缝：单测里定死时钟与 id。
	now   func() time.Time
	newID func() (uuid.UUID, error)
	// chartProvider / notifier 是 evaluator.go 的依赖，由装配期 setter 注入。
	chartProvider chartDataProvider
	notifier      Notifier
}

// NewService creates a new alert Service.
func NewService(db *bun.DB) Service {
	return &alertService{
		db:    db,
		now:   func() time.Time { return time.Now().UTC() },
		newID: func() (uuid.UUID, error) { return uuid.NewV7() },
	}
}

// ListRules returns all live rules ordered `created_at ASC, id ASC`（与文件夹
// 列表同款升序：新建的规则不插到旧规则前面）。
func (s *alertService) ListRules(ctx context.Context) ([]entity.AlertRule, error) {
	var rows []model.AlertRule
	err := s.db.NewSelect().Model(&rows).
		Where("deleted_at IS NULL").
		OrderExpr("created_at ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert rules: %w", err)
	}
	out := make([]entity.AlertRule, 0, len(rows))
	for i := range rows {
		out = append(out, toAlertRuleEntity(&rows[i]))
	}
	return out, nil
}

// GetRule returns one rule by its UUID.
func (s *alertService) GetRule(ctx context.Context, id string) (*entity.AlertRule, error) {
	m, err := s.getRuleModel(ctx, id)
	if err != nil {
		return nil, err
	}
	e := toAlertRuleEntity(m)
	return &e, nil
}

// CreateRule persists a new rule. 图表存在性在这里校验（软删图表同 20300）：
// 绑一个取不到数的图表只会让规则永远沉默，那是配置错误而不是可容忍的历史态。
func (s *alertService) CreateRule(ctx context.Context, in entity.AlertRuleCreateRequest) (*entity.AlertRule, error) {
	if err := validateRuleFields(in.Name, in.Metric, in.Operator); err != nil {
		return nil, err
	}
	if in.ChartID <= 0 {
		return nil, router.NewBusinessError(response.CodeBadRequest, "chart_id 必须为正整数")
	}
	if err := s.ensureChartExists(ctx, in.ChartID); err != nil {
		return nil, err
	}

	id, err := s.newID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate alert rule id: %w", err)
	}
	now := s.now()
	m := &model.AlertRule{
		ID:        id.String(),
		Name:      in.Name,
		ChartID:   in.ChartID,
		Metric:    in.Metric,
		Operator:  string(in.Operator),
		Threshold: in.Threshold,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create alert rule: %w", err)
	}
	e := toAlertRuleEntity(m)
	return &e, nil
}

// UpdateRule merges provided fields; everything else keeps its stored value
// （本仓库 PUT「未提供则保留」约定）。整行读出再写回（ExcludeColumn("deleted_at")），
// 与 folderService.UpdateFolder 同例（troubleshooting T-9）。
func (s *alertService) UpdateRule(ctx context.Context, id string, in entity.AlertRuleUpdateRequest) (*entity.AlertRule, error) {
	m, err := s.getRuleModel(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		if *in.Name == "" {
			return nil, router.NewBusinessError(response.CodeBadRequest, "预警名称不能为空")
		}
		m.Name = *in.Name
	}
	if in.ChartID != nil {
		if *in.ChartID <= 0 {
			return nil, router.NewBusinessError(response.CodeBadRequest, "chart_id 必须为正整数")
		}
		if err := s.ensureChartExists(ctx, *in.ChartID); err != nil {
			return nil, err
		}
		m.ChartID = *in.ChartID
	}
	if in.Metric != nil {
		if *in.Metric == "" {
			return nil, router.NewBusinessError(response.CodeBadRequest, "metric 不能为空")
		}
		m.Metric = *in.Metric
	}
	if in.Operator != nil {
		if !in.Operator.Valid() {
			return nil, router.NewBusinessError(response.CodeBadRequest, "operator 必须是 gt/lt/eq 之一")
		}
		m.Operator = string(*in.Operator)
	}
	if in.Threshold != nil {
		m.Threshold = *in.Threshold
	}
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
	m.UpdatedAt = s.now()

	if _, err := s.db.NewUpdate().Model(m).
		WherePK().
		Where("deleted_at IS NULL").
		ExcludeColumn("deleted_at").
		ExcludeColumn("created_at").
		ExcludeColumn("last_triggered_date").
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to update alert rule: %w", err)
	}
	e := toAlertRuleEntity(m)
	return &e, nil
}

// DeleteRule soft-deletes a rule; idempotent via the deleted_at IS NULL guard
// （重复删除影响 0 行不报错）。触发历史不删：规则删了，历史记录仍是事实。
func (s *alertService) DeleteRule(ctx context.Context, id string) error {
	if _, err := s.getRuleModel(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.NewUpdate().
		Model((*model.AlertRule)(nil)).
		Set("deleted_at = now()").
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete alert rule: %w", err)
	}
	return nil
}

// ListTriggers returns the rule's trigger history, newest first, capped at
// triggerHistoryLimit. 规则必须存在（软删同 20300）——按不存在处理防止软删规则
// 仍可通过 triggers 端点被遍历。
func (s *alertService) ListTriggers(ctx context.Context, id string) ([]entity.AlertTrigger, error) {
	if _, err := s.getRuleModel(ctx, id); err != nil {
		return nil, err
	}
	var rows []model.AlertTrigger
	err := s.db.NewSelect().Model(&rows).
		Where("rule_id = ?", id).
		OrderExpr("created_at DESC").
		Limit(triggerHistoryLimit).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert triggers: %w", err)
	}
	out := make([]entity.AlertTrigger, 0, len(rows))
	for i := range rows {
		out = append(out, toAlertTriggerEntity(&rows[i]))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 内部
// ---------------------------------------------------------------------------

// getRuleModel loads one live rule row.
func (s *alertService) getRuleModel(ctx context.Context, id string) (*model.AlertRule, error) {
	m := &model.AlertRule{ID: id}
	err := s.db.NewSelect().Model(m).WherePK().Where("deleted_at IS NULL").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, router.NewBusinessError(response.CodeNotFound, "alert rule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alert rule: %w", err)
	}
	return m, nil
}

// ensureChartExists 校验图表存在且未软删（20300），报警规则与 dashboard 文件夹
// 的父级校验同例：树/绑定语义依赖引用可解析。
func (s *alertService) ensureChartExists(ctx context.Context, chartID int) error {
	count, err := s.db.NewSelect().
		Model((*model.Chart)(nil)).
		Where("id = ?", chartID).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to check chart existence: %w", err)
	}
	if count == 0 {
		return router.NewBusinessError(response.CodeNotFound, "绑定的图表不存在")
	}
	return nil
}

// validateRuleFields 是 Create 与 Update 共用的字段校验（Update 侧逐字段调用）。
func validateRuleFields(name, metric string, operator entity.AlertOperator) error {
	if name == "" {
		return router.NewBusinessError(response.CodeBadRequest, "预警名称不能为空")
	}
	if metric == "" {
		return router.NewBusinessError(response.CodeBadRequest, "metric 不能为空")
	}
	if !operator.Valid() {
		return router.NewBusinessError(response.CodeBadRequest, "operator 必须是 gt/lt/eq 之一")
	}
	return nil
}

func toAlertRuleEntity(m *model.AlertRule) entity.AlertRule {
	lastDate := ""
	if m.LastTriggeredDate.Valid {
		lastDate = m.LastTriggeredDate.Time.UTC().Format("2006-01-02")
	}
	return entity.AlertRule{
		ID:                m.ID,
		Name:              m.Name,
		ChartID:           m.ChartID,
		Metric:            m.Metric,
		Operator:          entity.AlertOperator(m.Operator),
		Threshold:         m.Threshold,
		Enabled:           m.Enabled,
		LastTriggeredDate: lastDate,
		CreatedAt:         m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toAlertTriggerEntity(m *model.AlertTrigger) entity.AlertTrigger {
	e := entity.AlertTrigger{
		ID:          m.ID,
		RuleID:      m.RuleID,
		MetricValue: m.MetricValue,
		Threshold:   m.Threshold,
		Message:     m.Message,
		Notified:    m.Notified,
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.NotifyError.Valid {
		e.NotifyError = &m.NotifyError.String
	}
	return e
}
