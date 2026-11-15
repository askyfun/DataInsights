// Package config 从环境变量装配后端运行期配置。
//
// 配置只有一个来源，优先级由低到高：
//
//	内置默认值  <  环境变量（含启动时由 LoadDotEnv 装载的 .env 文件）
//
// 刻意不提供配置文件。容器化部署下每个需要用户填写的值都必须能从外部注入
// （docker run --env-file / compose environment / K8s env / 进程环境），
// 再挂一份 TOML 只会制造"改了没生效"的歧义：同一项两处可写、优先级还得记。
//
// 变量名刻意不加前缀：本进程是唯一读者，没有命名空间要抢；而 DATABASE_URL / PORT
// 是 12-factor 的标准名，Heroku / Railway / Render / Fly 等平台自动注入的就是裸名，
// 加前缀反而逼用户手工映射一遍。
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// ListenHost 是固定的监听地址，不提供环境变量覆盖。
//
// 它不是"默认值"而是"唯一定值"：容器场景下 0.0.0.0 本就是唯一合理的选择，
// 留一个没人会改的旋钮只会让"到底在听哪个地址"变成需要排查的问题。
// 端口可以配（PORT），地址不可以。
const ListenHost = "0.0.0.0"

// defaultPort 是未设置 PORT 时的监听端口。
// Database.Url 没有默认值，必须显式提供，由调用方 fail-fast。
const defaultPort = 23352

// Config 是全部运行期配置，字段只由 Load 从环境变量填充。
type Config struct {
	// Host 恒为 ListenHost，不从环境变量读取。保留字段是为了让调用方拿到
	// 完整的监听信息，不必自己再去引用常量。
	Host     string
	Port     int
	Database DatabaseConfig
	Security SecurityConfig
	Sentry   SentryConfig
	CORS     CORSConfig
	// StaticDir 是前端构建产物的目录。留空表示本进程只提供 API，
	// 页面由别的进程提供（本地开发时的 Vite dev server）。
	StaticDir string
	// ExtractDatasourceID 指定作为「本地文件上传统一存储」的数据源 id。
	//
	// 0 = 未显式指定。这是默认形态，**不是「功能关闭」**：装配期由
	// extract.Resolve 自举出一条复用应用自身 PostgreSQL 的隐藏存储（issue #201），
	// 所以本地文件上传开箱可用。只有自举失败才会真正没有存储——那时上传入口置灰，
	// 而平台其余部分照常（抽取存储是旁路能力，不该让整个进程起不来）。
	ExtractDatasourceID int
	// HealthDetail 开启 GET /health/detail（版本、迁移水位、依赖耗时等）。
	// 默认关闭且关闭时返回 404：详情里刻意不含连接串/host/库名，但它仍是
	// 一份内部拓扑的侧面画像，不该在公网部署上默认暴露。
	HealthDetail bool
	// Alert 是指标预警（issue #155）的运行期配置：评估间隔 + SMTP 通知。
	Alert AlertConfig
	// Log 是日志运行期配置（R-96）：Level 为 LOG_LEVEL 原始值，
	// 解析交给 logx.ParseLevel（非法值由调用方回落 info 并 warn 一次）。
	Log LogConfig
	// Admin 是首管建立与恢复的逃生口（R-91）：三者都留空 = 走一次性
	// bootstrap token（启动时打印）。
	Admin AdminConfig

	// Auth 是认证端点防护（issue #203）的运行期配置。
	Auth AuthConfig
}

// LogConfig 聚合日志相关环境变量。
type LogConfig struct {
	// Level 是 LOG_LEVEL 原始值（debug/info/warn/error）。
	Level string
}

// AdminConfig 聚合首管相关的环境变量。ADMIN_USERNAME 与 ADMIN_PASSWORD
// 必须成对出现；ADMIN_RESET_TOKEN 留空即关闭恢复通道。
type AdminConfig struct {
	Username   string
	Password   string
	ResetToken string
}

// AlertConfig 聚合预警模块的环境变量。SMTP 五项 + 收件人任何一项缺失都不拦启动：
// 邮件通知未配置时评估器照常运行，触发记录的 notify_error 会说明原因。
type AlertConfig struct {
	// EvalIntervalSeconds 是评估轮询间隔（秒），默认 defaultAlertEvalInterval。
	EvalIntervalSeconds int
	SMTPHost            string
	SMTPPort            string
	SMTPUsername        string
	SMTPPassword        string
	SMTPFrom            string
	// NotifyEmails 是收件人列表（逗号分隔原样保留，拆分交给消费方）。
	NotifyEmails string
}

// AuthConfig 聚合认证端点防护（issue #203）的环境变量。
// 零值 = 未设置，消费方回退内置默认值（遵循「空串等同未设置」约定）。
type AuthConfig struct {
	// RateLimitPerMinute 是同一来源 IP 每分钟允许的认证请求数，默认 defaultAuthRateLimitPerMinute。
	RateLimitPerMinute int
	// Enforce 是资源端点强制鉴权模式（R-92 / issue #217）：off|shadow|on。
	// 显式设置优先；未设置时**按 APP_ENV 推导**：生产（含未设置的默认值 production）默认 on，
	// 本地开发（development/dev/local/test）默认 off。见 defaultAuthEnforceFor。
	Enforce string
}

const (
	// defaultAlertEvalInterval 是未设置 ALERT_EVAL_INTERVAL 时的轮询秒数（5 分钟）。
	defaultAlertEvalInterval = 300
	// defaultAlertSMTPPort 是未设置 SMTP_PORT 时的提交端口（587 = STARTTLS 语义）。
	defaultAlertSMTPPort = "587"
	// defaultAuthRateLimitPerMinute 是未设置 AUTH_RATE_LIMIT_PER_MINUTE 时的默认值。
	defaultAuthRateLimitPerMinute = 10
	// AuthEnforceOff / AuthEnforceShadow / AuthEnforceOn 是资源端点强制鉴权模式（R-92 / #217）。
	AuthEnforceOff    = "off"
	AuthEnforceShadow = "shadow"
	AuthEnforceOn     = "on"
)

// devEnvs 是「默认可关闭强制鉴权」的运行环境取值（APP_ENV，大小写与前后空格已归一）。
// 这里刻意用白名单而不是黑名单：任何**没被认出来**的取值（含未设置 → production、
// staging、私有云环境名）都按生产对待，默认开鉴权。安全默认值不能靠猜。
var devEnvs = map[string]bool{
	"development": true,
	"dev":         true,
	"local":       true,
	"test":        true,
	"testing":     true,
}

// isDevEnv 判断 APP_ENV 是否本地开发环境。
func isDevEnv(appEnv string) bool {
	return devEnvs[strings.ToLower(strings.TrimSpace(appEnv))]
}

// defaultAuthEnforceFor 给出未显式设置 AUTH_ENFORCE 时的默认档位：
// 生产默认 on（资源端点必须带身份），本地开发默认 off（改代码/跑脚本不被登录打断）。
func defaultAuthEnforceFor(appEnv string) string {
	if isDevEnv(appEnv) {
		return AuthEnforceOff
	}
	return AuthEnforceOn
}

type DatabaseConfig struct {
	Url string
}

type SecurityConfig struct {
	SecurityKey string
}

type SentryConfig struct {
	Dsn string
	// Environment 是上报事件的 environment 标签（R-98），来自 APP_ENV；
	// 缺省 defaultAppEnv（production），空值不会出现。
	Environment string
}

// defaultAppEnv 是未设置 APP_ENV 时的运行环境标识。
const defaultAppEnv = "production"

type CORSConfig struct {
	AllowedOrigins []string
}

// LoadDotEnv 按传入顺序探测候选 .env 文件，把第一个存在的加载进进程环境变量，
// 并返回实际加载的路径；全部不存在时返回空字符串且不报错。
//
// 不覆盖已存在的环境变量是刻意设计（godotenv 的默认语义），由此得到优先级：
//
//	真实系统环境变量 > .env 文件
//
// 这样 docker run --env-file、K8s env、CI 注入的同名变量都能覆盖仓库里的 .env，
// 而 .env 只承担本地开发的便利作用。
//
// 文件不存在 / 未指定路径都不是错误：生产部署本来就不该有 .env 文件。
// 文件存在但格式非法才报错，避免配置被静默忽略。
func LoadDotEnv(candidates ...string) (string, error) {
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("failed to stat env file %s: %w", path, err)
		}
		if err := godotenv.Load(path); err != nil {
			return "", fmt.Errorf("failed to load env file %s: %w", path, err)
		}
		return path, nil
	}
	return "", nil
}

// Load 装配配置：先落内置默认值，再用环境变量逐项覆盖。
// 每个字段独立判断，空字符串一律视为"未设置"，因此 .env 里留空占位不会抹掉默认值。
func (c *Config) Load() error {
	c.Host = ListenHost
	c.Port = defaultPort

	if v := os.Getenv("PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid PORT %d: must be between 1 and 65535", port)
		}
		c.Port = port
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Database.Url = v
	}

	if v := os.Getenv("SECURITY_KEY"); v != "" {
		c.Security.SecurityKey = v
	}

	if v := os.Getenv("SENTRY_DSN"); v != "" {
		c.Sentry.Dsn = v
	}

	// APP_ENV 只作为监控事件的 environment 标签（R-98），留空等同 production。
	c.Sentry.Environment = defaultAppEnv
	if v := os.Getenv("APP_ENV"); v != "" {
		c.Sentry.Environment = v
	}

	if v := os.Getenv("STATIC_DIR"); v != "" {
		c.StaticDir = v
	}

	if v := os.Getenv("EXTRACT_DATASOURCE_ID"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id < 0 {
			return fmt.Errorf("invalid EXTRACT_DATASOURCE_ID %q: must be a non-negative integer", v)
		}
		c.ExtractDatasourceID = id
	}

	// HEALTH_DETAIL 只认显式开启值："1" / "true" / "on"。其他任何取值（含拼写错误）
	// 都按关闭处理——这是一个暴露面的开关，错了应该少暴露而不是多暴露。
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HEALTH_DETAIL"))) {
	case "1", "true", "on":
		c.HealthDetail = true
	}

	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		origins := strings.Split(v, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		c.CORS.AllowedOrigins = origins
	}

	// 预警评估间隔：非法值不 fail-fast（预警是旁路功能，配置错误不该拦住整个服务
	// 启动），回退默认值并打日志。
	c.Alert.EvalIntervalSeconds = defaultAlertEvalInterval
	if v := os.Getenv("ALERT_EVAL_INTERVAL"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 1 {
			slog.Warn("invalid ALERT_EVAL_INTERVAL, falling back to default",
				"value", v, "default", defaultAlertEvalInterval)
		} else {
			c.Alert.EvalIntervalSeconds = secs
		}
	}

	if v := os.Getenv("SMTP_HOST"); v != "" {
		c.Alert.SMTPHost = v
	}
	c.Alert.SMTPPort = defaultAlertSMTPPort
	if v := os.Getenv("SMTP_PORT"); v != "" {
		c.Alert.SMTPPort = v
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		c.Alert.SMTPUsername = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		c.Alert.SMTPPassword = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		c.Alert.SMTPFrom = v
	}
	if v := os.Getenv("ALERT_NOTIFY_EMAIL"); v != "" {
		c.Alert.NotifyEmails = v
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		c.Admin.Password = v
	}
	if v := os.Getenv("ADMIN_RESET_TOKEN"); v != "" {
		c.Admin.ResetToken = v

	}

	// 认证端点限流（issue #203）：非法值不 fail-fast（防护是安全增强，配置错误不该
	// 拦住整个服务启动），回退默认值并打日志。
	c.Auth.RateLimitPerMinute = defaultAuthRateLimitPerMinute
	if v := os.Getenv("AUTH_RATE_LIMIT_PER_MINUTE"); v != "" {
		rpm, err := strconv.Atoi(v)
		if err != nil || rpm < 1 {
			slog.Warn("invalid AUTH_RATE_LIMIT_PER_MINUTE, falling back to default",
				"value", v, "default", defaultAuthRateLimitPerMinute)
		} else {
			c.Auth.RateLimitPerMinute = rpm
		}

	}

	// 资源端点强制鉴权模式（R-92 / issue #217）：off|shadow|on。
	//
	// 默认值按运行环境推导（见 defaultAuthEnforceFor）：生产默认 on，本地开发默认 off。
	// 显式设置 AUTH_ENFORCE 永远优先——生产确实要关就写 off，但会打一条 warn 留痕。
	// 非法值不 fail-fast（防护是增强，配置错误不该拦启动），回落环境默认并 warn。
	c.Auth.Enforce = defaultAuthEnforceFor(c.Sentry.Environment)
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_ENFORCE"))); v != "" {
		switch v {
		case AuthEnforceOff, AuthEnforceShadow, AuthEnforceOn:
			c.Auth.Enforce = v
		default:
			slog.Warn("invalid AUTH_ENFORCE, falling back to environment default",
				"value", v, "app_env", c.Sentry.Environment, "default", c.Auth.Enforce)
		}
	}
	if c.Auth.Enforce == AuthEnforceOff && !isDevEnv(c.Sentry.Environment) {
		slog.Warn("AUTH_ENFORCE=off in a production-like environment: resource endpoints are anonymously accessible",
			"app_env", c.Sentry.Environment)
	}

	return nil
}
