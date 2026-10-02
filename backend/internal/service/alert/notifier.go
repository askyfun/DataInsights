// 预警通知抽象：评估器只认 Notifier 接口，具体通道（邮件起步）住在各自实现里。
//
// 第一期只有 SMTP 邮件：587 走 STARTTLS + PlainAuth（主流路径），465 端口在本期
// 不做隐式 TLS 直连——按端口区分 TLS 语义属于过度设计，先用最简实现跑通。
package alert

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"data-insights/internal/domain/entity"
	"data-insights/internal/model"
)

// Notifier 是一次触发的通知出口。实现必须**同步**返回（评估器按结果写
// notified / notify_error），且不重试——失败原因落库供人排查。
type Notifier interface {
	Notify(ctx context.Context, rule *ruleView, value, threshold float64) error
}

// ruleView 是 Notifier 眼中的规则最小面（避免 Notifier 反向依赖 model）。
type ruleView = model.AlertRule

// ErrNotifierNotConfigured 是 SMTP 未配置（SMTP_HOST 为空）时的哨兵错误：
// 评估器据此把 notify_error 写成「未配置」文案而不是当发送失败处理。
var ErrNotifierNotConfigured = errors.New("notifier not configured")

// SMTPConfig 是 EmailNotifier 的配置面（从 config.AlertConfig 摊平而来，便于单测）。
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	To       []string
}

// EmailNotifier 通过 SMTP 发送预警邮件。
type EmailNotifier struct {
	cfg SMTPConfig
}

// NewEmailNotifier 构造邮件通知器。cfg.SMTPHost 为空 = 未配置，Notify 直接返回
// ErrNotifierNotConfigured（不 panic、不做半成品发送）。
func NewEmailNotifier(cfg SMTPConfig) *EmailNotifier {
	return &EmailNotifier{cfg: cfg}
}

// Notify 发送一封预警邮件。未配置（Host 空）返回 ErrNotifierNotConfigured。
func (n *EmailNotifier) Notify(ctx context.Context, rule *ruleView, value, threshold float64) error {
	if n.cfg.Host == "" {
		return ErrNotifierNotConfigured
	}
	if n.cfg.From == "" || len(n.cfg.To) == 0 {
		return fmt.Errorf("smtp from/recipients not configured")
	}

	addr := net.JoinHostPort(n.cfg.Host, n.cfg.Port)
	subject := fmt.Sprintf("[Data Insights] 预警触发：%s", rule.Name)
	body := fmt.Sprintf("预警规则「%s」触发：\n\n指标 %s 的值 %s %s 阈值 %s。\n\n触发时间：%s",
		rule.Name, rule.Metric,
		strconv.FormatFloat(value, 'f', -1, 64),
		entityOperatorMessage(entity.AlertOperator(rule.Operator)),
		strconv.FormatFloat(threshold, 'f', -1, 64),
		time.Now().UTC().Format("2006-01-02 15:04:05 MST"))

	msg := buildRFC822(n.cfg.From, n.cfg.To, subject, body)

	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer conn.Close()

	if ok, _ := conn.Extension("STARTTLS"); ok {
		if err := conn.StartTLS(&tls.Config{ServerName: n.cfg.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if ok, params := conn.Extension("AUTH"); ok {
		if err := conn.Auth(smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		_ = params
	}
	if err := conn.Mail(n.cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, to := range n.cfg.To {
		if err := conn.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", to, err)
		}
	}
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	return conn.Quit()
}

// buildRFC822 拼一封最小 MIME 邮件（Subject 走 RFC 2047 编码以承载中文）。
func buildRFC822(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mimeQEncoding(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// mimeQEncoding 做 RFC 2047 Q 编码（=?UTF-8?q?...?=），避免非 ASCII 主题被网关丢弃。
func mimeQEncoding(s string) string {
	needsEncoding := false
	for i := 0; i < len(s); i++ {
		if s[i] > 126 || s[i] < 32 {
			needsEncoding = true
			break
		}
	}
	if !needsEncoding {
		return s
	}
	var out strings.Builder
	out.WriteString("=?UTF-8?q?")
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ':
			out.WriteByte('_')
		case c > 126 || c < 32 || c == '=' || c == '?' || c == '_':
			fmt.Fprintf(&out, "=%02X", c)
		default:
			out.WriteByte(c)
		}
	}
	out.WriteString("?=")
	return out.String()
}

// entityOperatorMessage 把算子渲染成人话（与 evaluator 的 message 同词表）。
func entityOperatorMessage(op entity.AlertOperator) string {
	switch op {
	case entity.AlertOperatorGT:
		return "大于"
	case entity.AlertOperatorLT:
		return "小于"
	case entity.AlertOperatorEQ:
		return "等于"
	}
	return string(op)
}
