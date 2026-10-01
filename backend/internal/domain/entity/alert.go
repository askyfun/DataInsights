package entity

// AlertOperator 是预警算子的 typed 常量（gt/lt/eq）。
type AlertOperator string

const (
	AlertOperatorGT AlertOperator = "gt"
	AlertOperatorLT AlertOperator = "lt"
	AlertOperatorEQ AlertOperator = "eq"
)

// Valid 判定算子是否在词表内。
func (op AlertOperator) Valid() bool {
	switch op {
	case AlertOperatorGT, AlertOperatorLT, AlertOperatorEQ:
		return true
	}
	return false
}

// Message 渲染触发时的人话描述。
func (op AlertOperator) Message() string {
	switch op {
	case AlertOperatorGT:
		return "大于"
	case AlertOperatorLT:
		return "小于"
	case AlertOperatorEQ:
		return "等于"
	}
	return string(op)
}

// AlertRule 是预警规则的出口实体（bi_alert_rule，migration 00009）。
// LastTriggeredDate 是「最近触发的 UTC 日期」（YYYY-MM-DD，空串 = 从未触发）：
// 它同时承担评估器的去重标记与前端「最近触发日」展示，出口为展示格式。
type AlertRule struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	ChartID           int           `json:"chart_id"`
	Metric            string        `json:"metric"`
	Operator          AlertOperator `json:"operator"`
	Threshold         float64       `json:"threshold"`
	Enabled           bool          `json:"enabled"`
	LastTriggeredDate string        `json:"last_triggered_date,omitempty"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
}

// AlertTrigger 是一次触发记录的出口实体（bi_alert_trigger）。
type AlertTrigger struct {
	ID          string  `json:"id"`
	RuleID      string  `json:"rule_id"`
	MetricValue float64 `json:"metric_value"`
	Threshold   float64 `json:"threshold"`
	Message     string  `json:"message"`
	Notified    bool    `json:"notified"`
	// NotifyError 非空 = 通知未送达的原因；成功通知为 null。
	NotifyError *string `json:"notify_error"`
	CreatedAt   string  `json:"created_at"`
}

// AlertRuleCreateRequest 是 POST /api/alerts 的业务输入。
// chart 存在性（含软删过滤）由 service 查 bi_chart 校验。
type AlertRuleCreateRequest struct {
	Name      string        `json:"name"`
	ChartID   int           `json:"chart_id"`
	Metric    string        `json:"metric"`
	Operator  AlertOperator `json:"operator"`
	Threshold float64       `json:"threshold"`
}

// AlertRuleUpdateRequest 是 PUT /api/alerts/{id} 的业务输入。
// 全指针，未提供则保留（本仓库 PUT 约定）；name/metric 显式空串报 20100，
// chart_id 非零才校验图表存在性。
type AlertRuleUpdateRequest struct {
	Name      *string        `json:"name"`
	ChartID   *int           `json:"chart_id"`
	Metric    *string        `json:"metric"`
	Operator  *AlertOperator `json:"operator"`
	Threshold *float64       `json:"threshold"`
	Enabled   *bool          `json:"enabled"`
}
