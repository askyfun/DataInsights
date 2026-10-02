// 预警评估器：后台 goroutine 定时轮询 enabled 规则并逐条取数评估（issue #155）。
//
// 语义（已拍板，不扩权）：
//   - 对每条规则调图表取数管道，**任一数据行**的指标值满足条件即触发，记录
//     **首个**越线行数值（行序即取数管道输出的顺序，符合「第一个越线值」直觉）；
//   - 同一 UTC 日内同一规则只触发一次：last_triggered_date 判重，触发后写当天日期；
//   - 整图无可用值（取数失败 / Data 形状不认识 / 无行可解析）→ 本周期跳过，只 log，
//     绝不报错退出——评估器是长期驻留的后台任务，单条规则的失败不能影响其余规则；
//   - 通知失败不影响触发记录落库（notified=false + notify_error 留档）。
//
// 时间由 now 缝注入（单测定死时钟）；ticker 循环只由 cmd/main.go 在 ctx 里启动，
// ctx 取消即退出。
package alert

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"data-insights/internal/database"
	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
	"data-insights/internal/query"

	"github.com/uptrace/bun"
)

// chartDataProvider 是 service/chart 的窄接口（dashboard/query.go 同款模式）：
// 评估器只需要「取一块图表的数据」，走接口避免 service→service 编译耦合，
// 单测塞假实现。
type chartDataProvider interface {
	GetData(ctx context.Context, id int) (entity.ChartDataResult, error)
}

// SetChartProvider 注入图表取数依赖（cmd/routes.go 装配期调用一次）。
// nil provider 时评估轮直接跳过（可表达「未接线」状态，不 panic）。
func (s *alertService) SetChartProvider(p chartDataProvider) { s.chartProvider = p }

// SetNotifier 注入通知实现（cmd/routes.go 装配期调用一次）。nil = 未配置通知，
// 触发照常落库，notify_error 写「未配置」文案。
func (s *alertService) SetNotifier(n Notifier) { s.notifier = n }

// RunEvaluator 阻塞运行评估循环：每 evalInterval 秒评估一轮，ctx 取消即返回。
// 启动后先等一个完整间隔再评估（进程刚起时图表与规则多半还在就绪中，不抢跑）。
func (s *alertService) RunEvaluator(ctx context.Context, evalInterval time.Duration) {
	ticker := time.NewTicker(evalInterval)
	defer ticker.Stop()
	slog.Info("Alert evaluator started", "interval", evalInterval.String())
	for {
		select {
		case <-ctx.Done():
			slog.Info("Alert evaluator stopped")
			return
		case <-ticker.C:
			s.evaluateAll(ctx)
		}
	}
}

// evaluateAll 评估一轮：拉全部 enabled 规则逐条评估。单条失败只 log，继续下一条。
func (s *alertService) evaluateAll(ctx context.Context) {
	var rules []model.AlertRule
	err := s.db.NewSelect().Model(&rules).
		Where("enabled = ?", true).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		slog.Error("Alert evaluator: failed to load rules", "error", err)
		return
	}
	for i := range rules {
		if ctx.Err() != nil {
			return
		}
		s.evaluateRule(ctx, &rules[i])
	}
}

// evaluateRule 评估单条规则。所有分支都不返回错误：失败语义全部落 log 或
// notify_error 留档（后台任务没有「响应给谁」）。
func (s *alertService) evaluateRule(ctx context.Context, m *model.AlertRule) {
	if s.chartProvider == nil {
		slog.Warn("Alert evaluator: chart provider not wired, skipping", "rule_id", m.ID)
		return
	}

	// 同日去重：last_triggered_date 与今天（UTC）同日 → 本周期跳过。
	today := s.now().UTC()
	if m.LastTriggeredDate.Valid && sameUTCDate(m.LastTriggeredDate.Time, today) {
		return
	}

	result, err := s.chartProvider.GetData(ctx, m.ChartID)
	if err != nil {
		slog.Warn("Alert evaluator: chart data unavailable, skipping", "rule_id", m.ID, "chart_id", m.ChartID, "error", err)
		return
	}
	var hit *float64
	switch data := result.Data.(type) {
	case []map[string]any:
		for i := range data {
			v, ok := extractMetricValue(data[i], m.Metric)
			if !ok {
				continue
			}
			if evaluateValue(entity.AlertOperator(m.Operator), v, m.Threshold) {
				hit = &v
				break
			}
		}
	case *query.KpiResponse:
		// KPI 图没有行，只有单个指标值；metric 列键在此形状下不参与取值。
		if isFinite(data.Value) && evaluateValue(entity.AlertOperator(m.Operator), data.Value, m.Threshold) {
			hit = &data.Value
		}
	default:
		// 其余聚合形状（pie 等）本期不支持：跳过并留痕，不报错。
		slog.Warn("Alert evaluator: chart data is not row-shaped, skipping",
			"rule_id", m.ID, "chart_id", m.ChartID, "data_type", fmt.Sprintf("%T", result.Data))
		return
	}
	if hit == nil {
		return
	}

	// 触发：通知结果 + 触发记录 + 去重标记在同一事务里成型。
	message := fmt.Sprintf("预警「%s」触发：指标 %s 的值 %s %s 阈值 %s",
		m.Name, m.Metric,
		strconv.FormatFloat(*hit, 'f', -1, 64),
		entity.AlertOperator(m.Operator).Message(),
		strconv.FormatFloat(m.Threshold, 'f', -1, 64))

	notified := false
	var notifyErr error
	if s.notifier == nil {
		notifyErr = ErrNotifierNotConfigured
	} else {
		notifyErr = s.notifier.Notify(ctx, m, *hit, m.Threshold)
	}
	notifyErrText := ""
	switch {
	case notifyErr == nil:
		notified = true
	case notifyErr == ErrNotifierNotConfigured:
		notifyErrText = "未配置邮件通知（SMTP_HOST 为空），本次触发未发送通知"
		slog.Warn("Alert evaluator: notifier not configured", "rule_id", m.ID)
	default:
		notifyErrText = notifyErr.Error()
		slog.Error("Alert evaluator: notification failed", "rule_id", m.ID, "error", notifyErr)
	}

	triggerID, err := s.newID()
	if err != nil {
		slog.Error("Alert evaluator: failed to generate trigger id, trigger not recorded", "rule_id", m.ID, "error", err)
		return
	}
	err = database.WithTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(&model.AlertTrigger{
			ID:          triggerID.String(),
			RuleID:      m.ID,
			MetricValue: *hit,
			Threshold:   m.Threshold,
			Message:     message,
			Notified:    notified,
			NotifyError: sql.NullString{String: notifyErrText, Valid: notifyErrText != ""},
			CreatedAt:   s.now(),
		}).Exec(ctx); err != nil {
			return fmt.Errorf("failed to insert alert trigger: %w", err)
		}
		if _, err := tx.NewUpdate().
			Model((*model.AlertRule)(nil)).
			Set("last_triggered_date = ?", today.Format("2006-01-02")).
			Where("id = ?", m.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to update alert rule last_triggered_date: %w", err)
		}
		return nil
	})
	if err != nil {
		// 触发记录落库失败不算「已触发」：去重标记同事务回滚，下轮重试。
		slog.Error("Alert evaluator: failed to record trigger", "rule_id", m.ID, "error", err)
	}
}

// sameUTCDate 判定两个时间是否同一 UTC 日。
func sameUTCDate(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

// evaluateValue 是算子求值的纯函数（单测直接打表）。
func evaluateValue(op entity.AlertOperator, value, threshold float64) bool {
	switch op {
	case entity.AlertOperatorGT:
		return value > threshold
	case entity.AlertOperatorLT:
		return value < threshold
	case entity.AlertOperatorEQ:
		return value == threshold
	}
	return false
}

// extractMetricValue 从数据行提取指标值并 coerce 成 float64。
// 数值直接接受；字符串按 strconv 解析（SQL 驱动常把 NUMERIC/DECIMAL 吐成串）；
// 缺列、NaN/Inf、不可解析一律 false（该行跳过，不是错误）。
func extractMetricValue(row map[string]any, key string) (float64, bool) {
	raw, ok := row[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, isFinite(v)
	case float32:
		f := float64(v)
		return f, isFinite(f)
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || !isFinite(f) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
