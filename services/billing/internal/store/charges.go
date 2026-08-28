package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type DueInstance struct {
	InstanceID   string
	UserID       string
	PlanName     string
	PriceMonthly float64
	PeriodDays   int
	Hostname     *string
	AutoRenew    bool
	ProductType  string
}

type ChargeResult struct {
	Processed int
	Charged   int
	Failed    int
	Suspended int
}

func (s *Store) ListDueInstances(ctx context.Context, limit int) ([]DueInstance, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, p.name,
			COALESCE(
				NULLIF((i.provider_meta->>'billing_price_rub')::float8, 0),
				p.price_monthly
			)::float8,
			COALESCE(NULLIF(i.billing_period_days, 0), 30), i.hostname, COALESCE(i.auto_renew, true),
			COALESCE(i.product_type, 'vps')
		FROM vps.instances i
		JOIN vps.plans p ON p.id = i.plan_id
		JOIN billing.accounts a ON a.user_id = i.user_id
		WHERE i.billing_status IN ('active', 'grace_period')
		  AND COALESCE(a.billing_status, 'active') NOT IN ('suspended')
		  AND i.state IN ('running', 'stopped', 'suspended')
		  AND i.next_billing_at IS NOT NULL
		  AND i.next_billing_at <= now()
		ORDER BY i.next_billing_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DueInstance
	for rows.Next() {
		var row DueInstance
		if err := rows.Scan(&row.InstanceID, &row.UserID, &row.PlanName, &row.PriceMonthly, &row.PeriodDays, &row.Hostname, &row.AutoRenew, &row.ProductType); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) ProcessDueCharges(ctx context.Context) (*ChargeResult, error) {
	due, err := s.ListDueInstances(ctx, 100)
	if err != nil {
		return nil, err
	}

	result := &ChargeResult{}
	for _, inst := range due {
		result.Processed++
		outcome, err := s.chargeInstance(ctx, inst)
		if err != nil {
			log.Printf("charge instance %s: %v", inst.InstanceID, err)
			continue
		}
		switch outcome {
		case "charged":
			result.Charged++
		case "failed", "past_due":
			result.Failed++
		case "suspended", "expired_no_renew":
			result.Suspended++
		}
	}
	return result, nil
}

func (s *Store) chargeInstance(ctx context.Context, inst DueInstance) (string, error) {
	if !inst.AutoRenew {
		return s.expireWithoutRenew(ctx, inst)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var billingStatus string
	var balanceKopecks int64
	var nextBillingAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance_kopecks, 0), i.next_billing_at
		FROM vps.instances i
		JOIN billing.accounts a ON a.user_id = i.user_id
		WHERE i.id = $1::uuid AND i.user_id = $2::uuid
		FOR UPDATE OF i, a
	`, inst.InstanceID, inst.UserID).Scan(&billingStatus, &balanceKopecks, &nextBillingAt)
	if err == pgx.ErrNoRows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing.accounts (user_id) VALUES ($1) ON CONFLICT DO NOTHING
		`, inst.UserID); err != nil {
			return "", err
		}
		balanceKopecks = 0
		billingStatus = "active"
	} else if err != nil {
		return "", err
	}
	balance := float64(balanceKopecks) / 100

	if nextBillingAt != nil && nextBillingAt.After(time.Now()) {
		return "skipped", nil
	}

	if billingStatus == "suspended" {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET billing_status = 'suspended', updated_at = now()
			WHERE id = $1::uuid AND user_id = $2::uuid
		`, inst.InstanceID, inst.UserID); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "failed", nil
	}

	graceHours := dunningGraceHours()

	amount := inst.PriceMonthly
	hostLabel := inst.PlanName
	if inst.Hostname != nil && *inst.Hostname != "" {
		hostLabel = *inst.Hostname
	}
	desc := fmt.Sprintf("VPS monthly charge — %s", hostLabel)
	if inst.ProductType == "dedicated" {
		desc = fmt.Sprintf("Dedicated monthly charge — %s", hostLabel)
	}

	if balance >= amount && amount >= 0 {
		discount, err := s.claimChargeDiscountTx(ctx, tx, inst.UserID, inst.InstanceID)
		if err != nil {
			return "", err
		}
		if discount > 0 {
			amount = math.Round(amount*(1-discount/100)*100) / 100
		}
		if amount < 0 {
			amount = 0
		}
		balanceAfter := balance
		if amount > 0 {
			debitKopecks := int64(math.Round(amount * 100))
			if err := tx.QueryRow(ctx, `
				UPDATE billing.accounts SET balance_kopecks = balance_kopecks - $2, updated_at = now()
				WHERE user_id = $1
				RETURNING balance_kopecks
			`, inst.UserID, debitKopecks).Scan(&balanceKopecks); err != nil {
				return "", err
			}
			balanceAfter = float64(balanceKopecks) / 100
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
			VALUES ($1, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
		`, inst.UserID, amount, desc, inst.InstanceID, balanceAfter); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET next_billing_at = COALESCE(next_billing_at, now()) + make_interval(days => $2),
			    billing_status = 'active',
			    updated_at = now()
			WHERE id = $1::uuid
		`, inst.InstanceID, inst.PeriodDays); err != nil {
			return "", err
		}
		if billingStatus == "past_due" {
			if _, err := tx.Exec(ctx, `
				UPDATE billing.accounts
				SET billing_status = 'active',
				    past_due_at = NULL,
				    grace_until = NULL,
				    dunning_reminder_at = NULL,
				    updated_at = now()
				WHERE user_id = $1::uuid
			`, inst.UserID); err != nil {
				return "", err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		_ = s.NotifyUserChargeSuccess(ctx, inst, amount, balanceAfter, nextBillingAt, inst.PeriodDays)
		return "charged", nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id)
		VALUES ($1, $2, 'failed', $3, 'balance', 'charge', $4::uuid)
	`, inst.UserID, amount, desc+" (insufficient balance)", inst.InstanceID); err != nil {
		return "", err
	}

	var graceUntil *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT grace_until FROM billing.accounts WHERE user_id = $1::uuid
	`, inst.UserID).Scan(&graceUntil); err != nil && err != pgx.ErrNoRows {
		return "", err
	}

	now := time.Now()
	inGrace := billingStatus == "past_due" && graceUntil != nil && graceUntil.After(now)

	if billingStatus == "active" {
		if err := s.enterGracePeriodTx(ctx, tx, inst.UserID, graceHours); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET billing_status = 'grace_period', updated_at = now()
			WHERE id = $1::uuid
		`, inst.InstanceID); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		_ = s.NotifyUserFirstPastDue(ctx, inst.UserID, graceHours)
		return "past_due", nil
	}

	if inGrace {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET billing_status = 'grace_period', updated_at = now()
			WHERE id = $1::uuid
		`, inst.InstanceID); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "past_due", nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET billing_status = 'suspended',
		    state = CASE WHEN state IN ('running', 'starting', 'restarting', 'creating') THEN 'stopped' ELSE state END,
		    suspended_at = COALESCE(suspended_at, now()),
		    updated_at = now()
		WHERE id = $1::uuid
	`, inst.InstanceID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts
		SET billing_status = 'suspended',
		    suspended_at = COALESCE(suspended_at, now()),
		    updated_at = now()
		WHERE user_id = $1::uuid
	`, inst.UserID); err != nil {
		return "", err
	}
	if err := s.enqueueInstanceStopInTx(ctx, tx, inst.InstanceID, inst.UserID, "charge_suspend"); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	_ = s.NotifyUser(ctx, inst.UserID,
		"VPS остановлен",
		fmt.Sprintf(
			"Недостаточно средств. Сервер выключен и недоступен. Пополните баланс в течение %d дн. — иначе VPS будет удалён.",
			dunningDeleteDays(),
		),
		"billing", true)
	return "suspended", nil
}

func (s *Store) expireWithoutRenew(ctx context.Context, inst DueInstance) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var freeOrAdmin bool
	var externalID string
	_ = tx.QueryRow(ctx, `
		SELECT
			COALESCE((provider_meta->>'free_week')::boolean, false)
			 OR COALESCE((provider_meta->>'trial')::boolean, false)
			 OR COALESCE((provider_meta->>'admin_issued')::boolean, false)
			 OR COALESCE((provider_meta->>'no_renew')::boolean, false),
			COALESCE(external_id, '')
		FROM vps.instances
		WHERE id = $1::uuid
	`, inst.InstanceID).Scan(&freeOrAdmin, &externalID)

	newState := "stopped"
	newBilling := "suspended"
	if freeOrAdmin {
		newState = "deleted"
		newBilling = "cancelled"
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET billing_status = $3,
		    state = CASE
		      WHEN $2::boolean THEN state
		      WHEN state IN ('running', 'starting', 'restarting', 'creating') THEN 'stopped'
		      ELSE state
		    END,
		    provider_meta = CASE
		      WHEN $2::boolean THEN COALESCE(provider_meta, '{}'::jsonb)
		          || jsonb_build_object('pending_destroy', to_jsonb(now()), 'destroy_reason', 'free_week_expired')
		      ELSE provider_meta
		    END,
		    suspended_at = COALESCE(suspended_at, now()),
		    updated_at = now()
		WHERE id = $1::uuid
		  AND billing_status IN ('active', 'grace_period')
	`, inst.InstanceID, freeOrAdmin, newBilling); err != nil {
		return "", err
	}
	_ = newState

	if freeOrAdmin && externalID != "" && inst.ProductType != "dedicated" {
		payload, _ := json.Marshal(map[string]any{
			"instance_id": inst.InstanceID,
			"external_id": externalID,
			"user_id":     inst.UserID,
			"reason":      "free_week_expired",
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO vps.outbox (event_type, payload)
			VALUES ('instance.destroy_requested', $1::jsonb)
		`, payload); err != nil {
			return "", err
		}
	}

	if !freeOrAdmin {
		if err := s.enqueueInstanceStopInTx(ctx, tx, inst.InstanceID, inst.UserID, "auto_renew_off"); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	if freeOrAdmin {
		_ = s.NotifyUser(ctx, inst.UserID,
			"Бесплатный период закончился",
			"Сервер остановлен и удалён. Оформите платный тариф — при переходе с бесплатного периода действует скидка 10% на первый месяц.",
			"billing", true)
	} else {
		_ = s.NotifyUser(ctx, inst.UserID,
			"Автопродление выключено",
			"Срок оплаты услуги истёк, автопродление отключено. Сервер остановлен. Продлите услугу вручную.",
			"billing", true)
	}
	return "expired_no_renew", nil
}

func dunningGraceHours() int {
	if raw := os.Getenv("DUNNING_GRACE_HOURS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	// Legacy env (days) — treat as hours only if explicitly set and GRACE_HOURS unset.
	if raw := os.Getenv("DUNNING_GRACE_DAYS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n * 24
		}
	}
	return 12
}

func (s *Store) claimChargeDiscountTx(ctx context.Context, tx pgx.Tx, userID, instanceID string) (float64, error) {
	var discount float64
	err := tx.QueryRow(ctx, `
		UPDATE billing.promo_entitlements
		SET active = false
		WHERE id = (
			SELECT id FROM billing.promo_entitlements
			WHERE user_id = $1::uuid AND active = true
			  AND discount_percent > 0
			  AND (expires_at IS NULL OR expires_at > now())
			  AND (instance_id IS NULL OR instance_id = $2::uuid)
			ORDER BY discount_percent DESC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING discount_percent::float8
	`, userID, instanceID).Scan(&discount)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return discount, err
}

func (s *Store) InitInstanceBilling(ctx context.Context, instanceID string, periodDays int) error {
	if periodDays <= 0 {
		periodDays = 30
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET next_billing_at = now() + make_interval(days => $2),
		    billing_period_days = $2,
		    updated_at = now()
		WHERE id = $1::uuid AND next_billing_at IS NULL
	`, instanceID, periodDays)
	return err
}
