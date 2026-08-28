package notify

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// AbuseAutoStopEmail notifies the customer that their server was automatically stopped due to suspected abuse.
func AbuseAutoStopEmail(ctx context.Context, userID, hostname, ip, caseID string) error {
	if hostname == "" {
		hostname = "server"
	}
	supportURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/vps/support"
	panelURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/vps/servers"
	supportEmail := envOr("SUPPORT_EMAIL", "support@cloud-hustle.com")
	logoURL := strings.TrimRight(envOr("SITE_URL", "https://cloud-hustle.com"), "/") + "/email-logo.png"

	subject := "[CLOUD HUSTLE] Сервер временно остановлен — требуется проверка"
	plain := fmt.Sprintf(`Здравствуйте!

Мы временно остановили ваш сервер %s (%s), потому что система зафиксировала активность, похожую на нарушение правил использования (возможный спам, сканирование сети или иной abusable traffic).

Это превентивная мера: сервер не удалён, данные сохранены. Чтобы разобраться в ситуации, откройте тикет в поддержке: %s

Укажите в обращении номер сервера и опишите, какие сервисы на нём работают. Если это ошибка, мы возобновим работу после проверки.

Панель управления: %s
Служба поддержки: %s

— Команда Cloud-hustle
`, hostname, ip, supportURL, panelURL, supportEmail)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;background:#0f0d14;color:#e8e6ed;padding:24px">
<div style="max-width:560px;margin:0 auto;background:#1a1625;border-radius:12px;padding:28px;border:1px solid #2a2438">
<img src="%s" alt="Cloud-hustle" width="140" style="margin-bottom:20px"/>
<h1 style="font-size:18px;margin:0 0 12px">Сервер временно остановлен</h1>
<p style="line-height:1.6;color:#b8b4c4">Здравствуйте!</p>
<p style="line-height:1.6;color:#b8b4c4">Мы временно <strong>остановили</strong> ваш сервер <strong>%s</strong> (%s), потому что автоматическая система мониторинга зафиксировала активность, похожую на <strong>нарушение правил использования</strong> (возможный спам, сканирование сети или иной abusable traffic).</p>
<p style="line-height:1.6;color:#b8b4c4">Это <strong>превентивная мера</strong> — сервер не удалён, ваши данные сохранены.</p>
<p style="line-height:1.6;color:#b8b4c4"><strong>Что делать:</strong></p>
<ol style="line-height:1.8;color:#b8b4c4">
<li>Откройте тикет в поддержке: <a href="%s" style="color:#a78bfa">Создать обращение</a></li>
<li>Укажите сервер и опишите, какие сервисы на нём работают</li>
<li>Дождитесь ответа — обычно в течение нескольких часов</li>
</ol>
<p style="line-height:1.6;color:#b8b4c4">Если это <strong>ошибка</strong>, мы возобновим работу сервера после проверки.</p>
<p style="margin-top:24px"><a href="%s" style="display:inline-block;background:#7c3aed;color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px">Открыть поддержку</a></p>
<p style="font-size:12px;color:#6b6578;margin-top:24px">ID инцидента: %s · <a href="%s" style="color:#a78bfa">Панель серверов</a></p>
</div></body></html>`,
		logoURL,
		html.EscapeString(hostname),
		html.EscapeString(ip),
		html.EscapeString(supportURL),
		html.EscapeString(supportURL),
		html.EscapeString(caseID),
		html.EscapeString(panelURL),
	)

	return UserHTML(ctx, userID, subject, plain, htmlBody, "abuse", true)
}
