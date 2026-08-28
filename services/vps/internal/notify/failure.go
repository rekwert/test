package notify

import (
	"context"
	"fmt"
	"html"
	"strings"
)

type FailKind string

const (
	FailProvision FailKind = "provision"
	FailReinstall FailKind = "reinstall"
)

// InstanceFailedEmail notifies the user when provision or OS reinstall could not finish.
func InstanceFailedEmail(ctx context.Context, userID, hostname, reason string, kind FailKind) error {
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	if kind == "" {
		kind = FailProvision
	}
	if hostname == "" {
		hostname = "server"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Не удалось завершить операцию автоматически."
	}

	panelURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/vps/servers"
	supportEmail := envOr("SUPPORT_EMAIL", "support@cloud-hustle.com")

	var subject, actionLine, retryLine, category string
	switch kind {
	case FailReinstall:
		subject = "[CLOUD HUSTLE] Переустановка ОС не удалась"
		actionLine = fmt.Sprintf(
			"Не удалось завершить переустановку операционной системы на сервере <strong>%s</strong>.",
			html.EscapeString(hostname),
		)
		retryLine = "Откройте карточку сервера в панели и запустите переустановку ОС снова. Если ошибка повторяется — напишите в поддержку."
		category = "vps"
	default:
		subject = "[CLOUD HUSTLE] Не удалось развернуть VPS"
		actionLine = fmt.Sprintf(
			"Не удалось автоматически завершить развёртывание сервера <strong>%s</strong>.",
			html.EscapeString(hostname),
		)
		retryLine = "Создайте новый заказ или обратитесь в поддержку — мы поможем вручную."
		category = "vps"
	}

	var plainAction string
	switch kind {
	case FailReinstall:
		plainAction = fmt.Sprintf("Не удалось завершить переустановку операционной системы на сервере %s.", hostname)
	default:
		plainAction = fmt.Sprintf("Не удалось автоматически завершить развёртывание сервера %s.", hostname)
	}

	plain := fmt.Sprintf(`Уважаемый клиент!

%s

Причина: %s

%s

Панель управления: %s
Техническая поддержка: %s

— CLOUD HUSTLE
`, plainAction, reason, retryLine, panelURL, supportEmail)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8" /></head>
<body style="margin:0;padding:24px;font-family:Segoe UI,Roboto,Helvetica,Arial,sans-serif;background:#fafaf9;color:#1c1917;">
  <div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #e7e5e4;border-radius:12px;padding:24px;">
    <p style="margin:0 0 12px;font-size:16px;">Уважаемый клиент!</p>
    <p style="margin:0 0 16px;line-height:1.5;">%s</p>
    <p style="margin:0 0 16px;padding:12px;background:#fef2f2;border:1px solid #fecaca;border-radius:8px;color:#991b1b;"><strong>Причина:</strong> %s</p>
    <p style="margin:0 0 16px;line-height:1.5;">%s</p>
    <p style="margin:0;"><a href="%s" style="color:#ea580c;">Открыть панель управления</a><br/>
    <a href="mailto:%s" style="color:#ea580c;">%s</a></p>
  </div>
</body>
</html>`,
		actionLine,
		html.EscapeString(reason),
		html.EscapeString(retryLine),
		html.EscapeString(panelURL),
		html.EscapeString(supportEmail), html.EscapeString(supportEmail),
	)

	return UserHTML(ctx, userID, subject, plain, htmlBody, category, true)
}
