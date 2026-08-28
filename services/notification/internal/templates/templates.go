package templates

import (
	"fmt"
	"os"
	"strings"
)

var transactionalTemplates = map[string]struct{}{
	"email_verify":   {},
	"password_reset": {},
	"telegram_link":  {},
}

func IsTransactional(name string) bool {
	_, ok := transactionalTemplates[name]
	return ok
}

// Render returns subject, plain text body and optional HTML body for a named template.
// Transactional OTP templates return empty htmlBody so clients get text/plain only.
func Render(name, locale, code string) (subject, body, htmlBody string, err error) {
	ru := strings.ToLower(locale) != "en"
	code = strings.TrimSpace(code)

	switch name {
	case "email_verify":
		if ru {
			subject = "Код подтверждения email"
			body = fmt.Sprintf("Здравствуйте!\n\nКод подтверждения email: %s\n\nКод действует 15 минут. Если вы не регистрировались на cloud-hustle.com, проигнорируйте это письмо.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
			return subject, body, "", nil
		}
		subject = "Email verification code"
		body = fmt.Sprintf("Hello,\n\nYour email verification code: %s\n\nThis code expires in 15 minutes. If you did not sign up at cloud-hustle.com, you can ignore this email.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
		return subject, body, "", nil

	case "password_reset":
		if ru {
			subject = "Сброс пароля"
			body = fmt.Sprintf("Здравствуйте!\n\nКод для сброса пароля: %s\n\nКод действует 15 минут. Если вы не запрашивали сброс, проигнорируйте письмо.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
			return subject, body, "", nil
		}
		subject = "Password reset code"
		body = fmt.Sprintf("Hello,\n\nYour password reset code: %s\n\nThis code expires in 15 minutes. If you did not request a reset, ignore this email.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
		return subject, body, "", nil

	case "telegram_link":
		if ru {
			subject = "Привязка Telegram"
			body = fmt.Sprintf("Здравствуйте!\n\nКод для привязки Telegram-бота: %s\n\nКод действует 15 минут. Если вы не запрашивали привязку, проигнорируйте письмо.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
			return subject, body, "", nil
		}
		subject = "Link Telegram"
		body = fmt.Sprintf("Hello,\n\nYour code to link the Telegram bot: %s\n\nThis code expires in 15 minutes. If you did not request this, ignore the email.\n\n— CLOUD HUSTLE\ncloud-hustle.com", code)
		return subject, body, "", nil

	default:
		return "", "", "", fmt.Errorf("unknown template: %s", name)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
