package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpsAlert posts to the staff Telegram alerts channel via notification service.
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

// TelegramChat sends a direct Telegram message by chat_id (guest tickets / no linked user).
func TelegramChat(ctx context.Context, chatID int64, title, body string) error {
	baseURL := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_URL"))
	token := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	if baseURL == "" || chatID == 0 {
		log.Printf("telegram chat notify skipped (url/chat unset)")
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"telegram_chat_id": chatID,
		"title":            title,
		"body":             body,
	})
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/system/notify-telegram",
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
		return fmt.Errorf("notify-telegram status %d", resp.StatusCode)
	}
	return nil
}

func TicketAnswered(ctx context.Context, userID string, autoCloseHours int, replyBody string) error {
	if userID == "" {
		return nil
	}
	if autoCloseHours <= 0 {
		autoCloseHours = 12
	}
	title := "Ответ по тикету"
	body := strings.TrimSpace(replyBody)
	if body == "" {
		body = "По вашему обращению есть ответ в личном кабинете."
	}
	body = fmt.Sprintf(
		"%s\n\nТикет автоматически переместится в архив через %d часов.\nЕсли остались вопросы — продолжите общение в тикете.",
		body, autoCloseHours,
	)
	return User(ctx, userID, title, body, "support", true)
}
