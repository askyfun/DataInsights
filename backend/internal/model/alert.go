package model

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
)

// AlertRule is the bun model for the bi_alert_rule table (migration 00009).
//
// 绑定「图表 + 指标（数据行的输出列 alias 键）+ 算子 + 阈值」；评估由后台
// evaluator 定时轮询 enabled 规则完成，触发与否落 LastTriggeredDate（UTC 日期，
// 同日只触发一次的去重标记）。
//
// ChartID 不建 FK：图表软删后规则行保留，评估时按「取数失败 → 本周期跳过」退化，
// 读出口（List/Get）不校验图表存在性。DeletedAt json:"-" 同所有软删列。
type AlertRule struct {
	bun.BaseModel `bun:"bi_alert_rule"`

	ID        string  `bun:"id,pk" json:"id"`
	Name      string  `bun:"name,notnull" json:"name"`
	ChartID   int     `bun:"chart_id,notnull" json:"chart_id"`
	Metric    string  `bun:"metric,notnull" json:"metric"`
	Operator  string  `bun:"operator,notnull" json:"operator"`
	Threshold float64 `bun:"threshold,notnull" json:"threshold"`
	Enabled   bool    `bun:"enabled,notnull" json:"enabled"`
	// LastTriggeredDate 是「本规则最近一次触发的 UTC 日期」，NULL = 从未触发。
	LastTriggeredDate sql.NullTime  `bun:"last_triggered_date" json:"-"`
	OwnerID           sql.NullInt32 `bun:"owner_id" json:"owner_id"`
	TenantID          sql.NullInt32 `bun:"tenant_id" json:"tenant_id"`
	CreatedAt         time.Time     `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt         time.Time     `bun:"updated_at,notnull" json:"updated_at"`
	DeletedAt         sql.NullTime  `bun:"deleted_at" json:"-"`
}

// TableName satisfies bun's table naming (also covered by the bun tag).
func (*AlertRule) TableName() string { return "bi_alert_rule" }

// AlertTrigger is the bun model for the bi_alert_trigger table (migration 00009).
//
// 只追加不改写：每次触发落一行（命中的首个越线行数值、阈值、人话 message、
// 通知结果）。notified=false 且 notify_error 非空 = 通知未送达（SMTP 未配置或
// 发送失败），不算评估失败——评估与通知在记录上一次性成型，本期不做重试队列。
type AlertTrigger struct {
	bun.BaseModel `bun:"bi_alert_trigger"`

	ID          string         `bun:"id,pk" json:"id"`
	RuleID      string         `bun:"rule_id,notnull" json:"rule_id"`
	MetricValue float64        `bun:"metric_value,notnull" json:"metric_value"`
	Threshold   float64        `bun:"threshold,notnull" json:"threshold"`
	Message     string         `bun:"message,notnull" json:"message"`
	Notified    bool           `bun:"notified,notnull" json:"notified"`
	NotifyError sql.NullString `bun:"notify_error" json:"notify_error"`
	CreatedAt   time.Time      `bun:"created_at,notnull" json:"created_at"`
}

// TableName satisfies bun's table naming (also covered by the bun tag).
func (*AlertTrigger) TableName() string { return "bi_alert_trigger" }
