package store

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

func (s *Store) NotifyUser(ctx context.Context, userID, title, body, category string, sendEmail bool) error {
	baseURL := os.Getenv("NOTIFICATION_SERVICE_URL")
	token := os.Getenv("NOTIFICATION_SERVICE_TOKEN")
	if baseURL != "" {
		payload, _ := json.Marshal(map[string]any{
			"user_id":    userID,
			"title":      title,
			"body":       body,
			"category":   category,
			"send_email": sendEmail,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/system/notify", bytes.NewReader(payload))
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
			log.Printf("notify user %s: %v", userID, err)
			return s.insertInbox(ctx, userID, title, body, category)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("notify user %s: status %d", userID, resp.StatusCode)
			return s.insertInbox(ctx, userID, title, body, category)
		}
		return nil
	}
	return s.insertInbox(ctx, userID, title, body, category)
}

func (s *Store) insertInbox(ctx context.Context, userID, title, body, category string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification.inbox (user_id, title, body, category)
		VALUES ($1::uuid, $2, $3, $4)
	`, userID, title, body, category)
	return err
}

func (s *Store) NotifyUserFirstPastDue(ctx context.Context, userID string, graceHours int) error {
	if graceHours <= 0 {
		graceHours = 12
	}
	body := fmt.Sprintf(
		"На балансе недостаточно средств для продления VPS. У вас %d ч., чтобы пополнить баланс — после этого сервер будет остановлен.",
		graceHours,
	)
	return s.NotifyUser(ctx, userID, "Баланс закончился", body, "billing", true)
}

// NotifyUserChargeSuccess informs the client that auto-renewal charge succeeded.
func (s *Store) NotifyUserChargeSuccess(ctx context.Context, inst DueInstance, amount, balanceAfter float64, prevBillingAt *time.Time, periodDays int) error {
	hostLabel := inst.PlanName
	if inst.Hostname != nil && strings.TrimSpace(*inst.Hostname) != "" {
		hostLabel = strings.TrimSpace(*inst.Hostname)
	}
	base := time.Now().UTC()
	if prevBillingAt != nil {
		base = prevBillingAt.UTC()
	}
	if periodDays <= 0 {
		periodDays = 30
	}
	nextDue := base.AddDate(0, 0, periodDays)
	msk := time.FixedZone("MSK", 3*60*60)
	nextDueMSK := nextDue.In(msk).Format("02.01.2006 15:04") + " МСК"

	kind := "сервер"
	if inst.ProductType == "dedicated" {
		kind = "dedicated"
	}
	body := fmt.Sprintf(
		"Списано %.0f ₽ за %s %s. Следующее списание: %s. Баланс: %.0f ₽.",
		amount, kind, hostLabel, nextDueMSK, balanceAfter,
	)
	return s.NotifyUser(ctx, inst.UserID, "Оплата прошла, сервер продлён", body, "billing", true)
}
