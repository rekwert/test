package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpsAlert sends a message to the ops/alerts Telegram channel via notification service.
// Used for dedicated provision failures and other staff-facing events.
func OpsAlert(ctx context.Context, title, body string) error {
	baseURL := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_URL"))
	token := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	if baseURL == "" {
		log.Printf("ops alert: NOTIFICATION_SERVICE_URL not set (%s)", title)
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
	})
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/system/ops-alert",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Service-Token", token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ops alert status %d", resp.StatusCode)
	}
	return nil
}

func User(ctx context.Context, userID, title, body, category string, sendEmail bool) error {
	return UserHTML(ctx, userID, title, body, "", category, sendEmail)
}

func UserHTML(ctx context.Context, userID, title, body, htmlBody, category string, sendEmail bool) error {
	baseURL := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_URL"))
	token := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	if baseURL == "" {
		log.Printf("notify user %s: NOTIFICATION_SERVICE_URL not set", userID)
		return nil
	}

	payload, _ := json.Marshal(map[string]any{
		"user_id":    userID,
		"title":      title,
		"body":       body,
		"html_body":  htmlBody,
		"category":   category,
		"send_email": sendEmail,
	})
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/system/notify",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Service-Token", token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification service status %d", resp.StatusCode)
	}
	return nil
}

type ReadyKind string

const (
	ReadyProvision ReadyKind = "provision"
	ReadyReinstall ReadyKind = "reinstall"
	ReadyDedicated ReadyKind = "dedicated"
)

// InstanceReadyEmail sends inbox + branded HTML email when a server is ready
// after first provision, dedicated provision, or OS reinstall.
// includeSSH adds the "ssh root@IP" connection block (omit for Windows guests).
func InstanceReadyEmail(ctx context.Context, userID, hostname, ip, password string, orderNumber int64, kind ReadyKind, includeSSH bool) error {
	if kind == "" {
		kind = ReadyProvision
	}
	if hostname == "" {
		if kind == ReadyDedicated {
			hostname = "server"
		} else {
			hostname = "vps"
		}
	}

	orderSuffix := ""
	if orderNumber > 0 {
		orderSuffix = fmt.Sprintf(" №%d", orderNumber)
	}

	var subject, actionLine, category string
	switch kind {
	case ReadyReinstall:
		subject = "[CLOUD HUSTLE] Переустановка ОС завершена" + orderSuffix
		actionLine = fmt.Sprintf(
			"Переустановка операционной системы на сервере <strong>%s</strong> успешно завершена. Сервер снова доступен.",
			html.EscapeString(hostname),
		)
		category = "vps"
	case ReadyDedicated:
		subject = "[CLOUD HUSTLE] Выделенный сервер готов к использованию" + orderSuffix
		actionLine = fmt.Sprintf(
			"Ваш выделенный сервер <strong>%s</strong> подготовлен и готов к использованию.",
			html.EscapeString(hostname),
		)
		category = "dedicated"
	default:
		subject = "[CLOUD HUSTLE] Ваша VPS готова к использованию" + orderSuffix
		actionLine = fmt.Sprintf(
			"Ваша VPS <strong>%s</strong> развёрнута и готова к использованию.",
			html.EscapeString(hostname),
		)
		category = "vps"
	}

	panelURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/vps/servers"
	supportEmail := envOr("SUPPORT_EMAIL", "support@cloud-hustle.com")
	// PNG: most email clients do not render SVG (broken image icon).
	logoURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/email-logo.png"

	tagline := "Аренда виртуальных серверов"
	if kind == ReadyDedicated {
		tagline = "Аренда выделенных серверов"
	}

	plain := buildReadyPlain(hostname, ip, password, panelURL, supportEmail, kind, includeSSH)
	htmlBody := buildReadyHTML(hostname, ip, password, panelURL, supportEmail, logoURL, actionLine, tagline, includeSSH)

	return UserHTML(ctx, userID, subject, plain, htmlBody, category, true)
}

// InstanceReady keeps compatibility for callers that do not pass order/kind.
func InstanceReady(ctx context.Context, userID, hostname, ip, password string) error {
	return InstanceReadyEmail(ctx, userID, hostname, ip, password, 0, ReadyProvision, true)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func buildReadyPlain(hostname, ip, password, panelURL, supportEmail string, kind ReadyKind, includeSSH bool) string {
	var intro string
	switch kind {
	case ReadyReinstall:
		intro = fmt.Sprintf("Уважаемый клиент!\n\nПереустановка ОС на сервере %s завершена.", hostname)
	case ReadyDedicated:
		intro = fmt.Sprintf("Уважаемый клиент!\n\nВаш выделенный сервер %s готов к использованию.", hostname)
	default:
		intro = fmt.Sprintf("Уважаемый клиент!\n\nВаша VPS %s готова к использованию.", hostname)
	}
	sshBlock := ""
	if includeSSH {
		sshBlock = fmt.Sprintf("\nПодключение:\nssh root@%s\n", ip)
	}
	return fmt.Sprintf(`%s

IP: %s
Логин: root
Пароль: %s
%s
Дайте нам знать, если имеются какие-либо проблемы с сервером и требуется наше участие.

Панель управления: %s
Техническая поддержка: %s

— CLOUD HUSTLE
`, intro, ip, password, sshBlock, panelURL, supportEmail)
}

func buildReadyHTML(hostname, ip, password, panelURL, supportEmail, logoURL, actionLine, tagline string, includeSSH bool) string {
	esc := html.EscapeString
	sshRow := ""
	passwordPadding := "12px 16px 16px"
	if includeSSH {
		passwordPadding = "12px 16px"
		sshRow = fmt.Sprintf(
			`<tr><td style="padding:12px 16px 16px;color:#1c1917;"><strong style="color:#78716c;">Подключение:</strong><br/><code style="color:#c2410c;">ssh root@%s</code></td></tr>`,
			esc(ip),
		)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>CLOUD HUSTLE</title>
</head>
<body style="margin:0;padding:0;background:#fafaf9;font-family:Segoe UI,Roboto,Helvetica,Arial,sans-serif;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#fafaf9;padding:24px 12px;">
    <tr>
      <td align="center">
        <table role="presentation" width="600" cellspacing="0" cellpadding="0" style="max-width:600px;width:100%%;background:#ffffff;border-radius:16px;overflow:hidden;border:1px solid #e7e5e4;">
          <tr>
            <td style="background:linear-gradient(128deg,#f97316 0%%,#ea580c 58%%,#c2410c 100%%);padding:28px 32px;text-align:center;">
              <img src="%s" width="56" height="56" alt="" style="display:block;margin:0 auto 12px;border:0;border-radius:14px;" />
              <div style="font-size:22px;font-weight:700;letter-spacing:0.06em;color:#ffffff;">CLOUD HUSTLE</div>
              <div style="margin-top:6px;font-size:13px;color:#ffedd5;">%s</div>
            </td>
          </tr>
          <tr>
            <td style="padding:28px 32px 8px;color:#44403c;font-size:15px;line-height:1.55;">
              <p style="margin:0 0 16px;font-size:16px;color:#1c1917;">Уважаемый клиент!</p>
              <p style="margin:0 0 20px;">%s</p>
              <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#fafaf9;border:1px solid #e7e5e4;border-radius:12px;">
                <tr><td style="padding:14px 16px;border-bottom:1px solid #e7e5e4;color:#ea580c;font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;">Данные для доступа</td></tr>
                <tr><td style="padding:12px 16px;color:#1c1917;"><strong style="color:#78716c;">Сервер:</strong> %s</td></tr>
                <tr><td style="padding:12px 16px;color:#1c1917;"><strong style="color:#78716c;">IP:</strong> %s</td></tr>
                <tr><td style="padding:12px 16px;color:#1c1917;"><strong style="color:#78716c;">Логин:</strong> root</td></tr>
                <tr><td style="padding:%s;color:#1c1917;"><strong style="color:#78716c;">Пароль:</strong> <code style="color:#1c1917;background:#ffedd5;padding:2px 6px;border-radius:4px;">%s</code></td></tr>
                %s
              </table>
              <p style="margin:22px 0 8px;">Дайте нам знать, если имеются какие-либо проблемы с сервером и требуется наше участие.</p>
              <p style="margin:0 0 6px;"><a href="%s" style="display:inline-block;margin:8px 0 12px;padding:12px 20px;background:#ea580c;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;">Открыть панель управления</a></p>
              <p style="margin:0 0 20px;">Техническая поддержка: <a href="mailto:%s" style="color:#ea580c;text-decoration:none;">%s</a></p>
            </td>
          </tr>
          <tr>
            <td style="padding:16px 32px 24px;border-top:1px solid #e7e5e4;color:#78716c;font-size:12px;line-height:1.5;text-align:center;">
              С уважением,<br/>команда CLOUD HUSTLE<br/>
              <a href="https://cloud-hustle.com" style="color:#78716c;text-decoration:none;">cloud-hustle.com</a>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		esc(logoURL),
		esc(tagline),
		actionLine,
		esc(hostname),
		esc(ip),
		passwordPadding,
		esc(password),
		sshRow,
		esc(panelURL),
		esc(supportEmail), esc(supportEmail),
	)
}
