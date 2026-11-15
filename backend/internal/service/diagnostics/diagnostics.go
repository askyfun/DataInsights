// Package diagnostics 组装诊断包（R-104 / issue #229）：版本/迁移/健康/配置键快照/
// 最近错误。配置只列「键名 + 是否已设置」，绝不出值；错误串已在 diag 入队时经 logx.Redact。
package diagnostics

import (
	"context"
	"os"

	"github.com/uptrace/bun"

	"tablewave/internal/database"
	"tablewave/internal/diag"
)

// Info 是装配期注入的静态运行信息。
type Info struct {
	Version       string
	Commit        string
	BuildTime     string
	SentryEnabled bool
	AuthEnforce   string
}

type ConfigItem struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}

type Bundle struct {
	Version            string          `json:"version"`
	Commit             string          `json:"commit"`
	BuildTime          string          `json:"build_time"`
	MigrationVersion   int64           `json:"migration_version"`
	MigrationsUpToDate bool            `json:"migrations_up_to_date"`
	DatabaseConnected  bool            `json:"database_connected"`
	SentryEnabled      bool            `json:"sentry_enabled"`
	AuthEnforce        string          `json:"auth_enforce"`
	Checks             map[string]bool `json:"checks"`
	Config             []ConfigItem    `json:"config"`
	RecentErrors       []diag.Entry    `json:"recent_errors"`
}

// configKeys 是配置快照的键名白名单（只报是否已设置，不报值）。
var configKeys = []string{
	"DATABASE_URL", "SECURITY_KEY", "SENTRY_DSN", "STATIC_DIR", "PORT",
	"CORS_ALLOWED_ORIGINS", "APP_ENV", "AUTH_ENFORCE", "AUTH_RATE_LIMIT_PER_MINUTE",
	"HOST_BIND", "ADMIN_USERNAME", "ADMIN_PASSWORD", "ADMIN_RESET_TOKEN",
	"LOG_LEVEL", "HEALTH_DETAIL", "EXTRACT_DATASOURCE_ID",
	"ALERT_EVAL_INTERVAL", "ALERT_NOTIFY_EMAIL",
	"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM",
}

type Service struct {
	db   *bun.DB
	info Info
}

// ConfigSnapshot 只返回键名与「是否已设置」，绝不返回值（脱敏红线）。
func ConfigSnapshot() []ConfigItem {
	out := make([]ConfigItem, 0, len(configKeys))
	for _, k := range configKeys {
		out = append(out, ConfigItem{Name: k, Set: os.Getenv(k) != ""})
	}
	return out
}

func NewService(db *bun.DB, info Info) *Service {
	return &Service{db: db, info: info}
}

func (s *Service) Build(ctx context.Context) (*Bundle, error) {
	mv, upToDate, _ := database.MigrationsUpToDate(ctx, s.db.DB)
	pingErr := s.db.DB.PingContext(ctx)
	dbOK := pingErr == nil

	cfg := ConfigSnapshot()

	return &Bundle{
		Version:            s.info.Version,
		Commit:             s.info.Commit,
		BuildTime:          s.info.BuildTime,
		MigrationVersion:   mv,
		MigrationsUpToDate: upToDate,
		DatabaseConnected:  dbOK,
		SentryEnabled:      s.info.SentryEnabled,
		AuthEnforce:        s.info.AuthEnforce,
		Checks: map[string]bool{
			"database":   dbOK,
			"migrations": upToDate,
		},
		Config:       cfg,
		RecentErrors: diag.Snapshot(100, 0, true),
	}, nil
}
