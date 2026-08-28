package store

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// ProcessFreeWeekReminders keeps the worker entrypoint name stable.
// It now runs the 7-day daily lease/balance reminder job (inbox + email + Telegram).
func (s *Store) ProcessFreeWeekReminders(ctx context.Context) (int, error) {
	return s.ProcessLeaseEndingReminders(ctx)
}

// ProcessLeaseEndingReminders sends daily reminders for the last 7 days before
// next_billing_at when the lease is ending without auto-renew, or auto-renew
// is on but balance is too low to cover the next charge.
//
// Channels (via notification /system/notify): site inbox, email, Telegram if linked.
func (s *Store) ProcessLeaseEndingReminders(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text,
		       i.user_id::text,
		       COALESCE(i.hostname, p.name, ''),
		       COALESCE(
		         NULLIF((i.provider_meta->>'billing_price_rub')::float8, 0),
		         p.price_monthly
		       )::float8,
		       COALESCE(i.auto_renew, true),
		       i.next_billing_at,
		       COALESCE((i.provider_meta->>'free_week')::boolean, false)
		         OR COALESCE((i.provider_meta->>'trial')::boolean, false) AS is_free_week,
		       COALESCE(a.balance, 0)::float8
		FROM vps.instances i
		JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN billing.accounts a ON a.user_id = i.user_id
		WHERE i.billing_status IN ('active', 'grace_period')
		  AND i.state NOT IN ('deleted', 'creating', 'queued')
		  AND i.next_billing_at IS NOT NULL
		  AND i.next_billing_at > now()
		  AND i.next_billing_at <= now() + interval '7 days'
		  AND COALESCE((i.provider_meta->>'admin_issued')::boolean, false) = false
		  AND (
		    i.provider_meta->>'lease_remind_last_at' IS NULL
		    OR (i.provider_meta->>'lease_remind_last_at')::timestamptz <= now() - interval '24 hours'
		  )
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			instanceID, userID, hostname string
			price                        float64
			autoRenew                    bool
			nextBilling                  time.Time
			isFreeWeek                   bool
			balance                      float64
		)
		if err := rows.Scan(&instanceID, &userID, &hostname, &price, &autoRenew, &nextBilling, &isFreeWeek, &balance); err != nil {
			return count, err
		}

		needsNotify := false
		if !autoRenew || isFreeWeek {
			needsNotify = true
		} else if price > 0 && balance+1e-9 < price {
			needsNotify = true
		}
		if !needsNotify {
			continue
		}

		if isFreeWeek {
			if err := s.grantFreeWeekPromo(ctx, userID); err != nil {
				log.Printf("lease remind promo user=%s: %v", userID, err)
			}
		}

		label := strings.TrimSpace(hostname)
		if label == "" {
			label = instanceID
			if len(label) > 8 {
				label = label[:8]
			}
		}
		daysLeft := int(math.Ceil(time.Until(nextBilling).Hours() / 24))
		if daysLeft < 1 {
			daysLeft = 1
		}
		if daysLeft > 7 {
			daysLeft = 7
		}
		msk := time.FixedZone("MSK", 3*60*60)
		until := nextBilling.In(msk).Format("02.01.2006 15:04") + " МСК"

		var title, body string
		switch {
		case !autoRenew || isFreeWeek:
			title = "Срок сервера заканчивается"
			body = fmt.Sprintf(
				"Через %d дн. (до %s) заканчивается период сервера %s. Включите автопродление или продлите в личном кабинете / боте. Иначе сервер будет остановлен и удалён.",
				daysLeft, until, label,
			)
			if isFreeWeek {
				body += " На первый месяц любого тарифа действует скидка 10% (промо после бесплатной недели)."
			}
		default:
			title = "Не хватает баланса для продления"
			need := price - balance
			if need < 0 {
				need = 0
			}
			body = fmt.Sprintf(
				"Через %d дн. (до %s) списание за сервер %s: %.0f ₽. На балансе %.0f ₽ (не хватает ещё %.0f ₽). Пополните баланс, иначе сервер уйдёт в отсрочку и может быть остановлен.",
				daysLeft, until, label, price, balance, need,
			)
		}

		if err := s.NotifyUser(ctx, userID, title, body, "billing", true); err != nil {
			log.Printf("lease remind notify user=%s instance=%s: %v", userID, instanceID, err)
			continue
		}
		if _, err := s.pool.Exec(ctx, `
			UPDATE vps.instances
			SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
			  || jsonb_build_object('lease_remind_last_at', to_jsonb(now())),
			    updated_at = now()
			WHERE id = $1::uuid
		`, instanceID); err != nil {
			log.Printf("lease remind mark %s: %v", instanceID, err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func (s *Store) grantFreeWeekPromo(ctx context.Context, userID string) error {
	var promoID string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM billing.promo_codes
		WHERE lower(code) = 'freeweek10' AND active = true
		LIMIT 1
	`).Scan(&promoID)
	if err != nil {
		return err
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM billing.promo_entitlements
			WHERE user_id = $1::uuid AND promo_id = $2::uuid AND active = true
			  AND (expires_at IS NULL OR expires_at > now())
		)
	`, userID, promoID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO billing.promo_entitlements (user_id, promo_id, discount_percent, instance_id, expires_at)
		VALUES ($1::uuid, $2::uuid, 10, NULL, now() + interval '30 days')
	`, userID, promoID)
	return err
}
