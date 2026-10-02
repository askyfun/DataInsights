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
	// ExtractDatasourceID 指定作为「抽取数据集统一存储」的数据源 id（issue #118 预留）。
	// 0 = 未启用：抽取存储数据源不存在，所有抽取守卫退化为放行。
	ExtractDatasourceID int
	// Alert 是指标预警（issue #155）的运行期配置：评估间隔 + SMTP 通知。
	Alert AlertConfig
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

const (
	// defaultAlertEvalInterval 是未设置 ALERT_EVAL_INTERVAL 时的轮询秒数（5 分钟）。
	defaultAlertEvalInterval = 300
	// defaultAlertSMTPPort 是未设置 SMTP_PORT 时的提交端口（587 = STARTTLS 语义）。
	defaultAlertSMTPPort = "587"
)

type DatabaseConfig struct {
	Url string
}

type SecurityConfig struct {
	SecurityKey string
}

type SentryConfig struct {
	Dsn string
}

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

	return nil
}
