package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DunningConfig struct {
	GraceHours int
	DeleteDays int
}

type DunningResult struct {
	GraceExpired    int
	Deleted         int
	Reminders       int
	ReconciledStops int
}

func (s *Store) ProcessDunning(ctx context.Context, cfg DunningConfig) (*DunningResult, error) {
	if cfg.GraceHours <= 0 {
		cfg.GraceHours = 12
	}
	if cfg.DeleteDays <= 0 {
		cfg.DeleteDays = 3
	}

	result := &DunningResult{}

	expired, err := s.expireGracePeriods(ctx)
	if err != nil {
		return nil, err
	}
	result.GraceExpired = expired

	deleted, err := s.deleteExpiredSuspendedInstances(ctx, cfg.DeleteDays)
	if err != nil {
		return nil, err
	}
	result.Deleted = deleted

	reminders, err := s.sendDunningReminders(ctx)
	if err != nil {
		return nil, err
	}
	result.Reminders = reminders

	reconciled, err := s.reconcileSuspendedRunningStops(ctx)
	if err != nil {
		return nil, err
	}
	result.ReconciledStops = reconciled

	return result, nil
}

func (s *Store) expireGracePeriods(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id::text
		FROM billing.accounts
		WHERE billing_status = 'past_due'
		  AND grace_until IS NOT NULL
		  AND grace_until <= now()
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return count, err
		}
		if err := s.suspendUserForNonPayment(ctx, userID, "grace_expired"); err != nil {
			log.Printf("dunning suspend user %s: %v", userID, err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func (s *Store) suspendUserForNonPayment(ctx context.Context, userID, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts
		SET billing_status = 'suspended',
		    suspended_at = COALESCE(suspended_at, now()),
		    grace_until = NULL,
		    updated_at = now()
		WHERE user_id = $1::uuid
	`, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET billing_status = 'suspended',
		    state = CASE WHEN state IN ('running', 'starting', 'restarting', 'creating') THEN 'stopped' ELSE state END,
		    suspended_at = COALESCE(suspended_at, now()),
		    updated_at = now()
		WHERE user_id = $1::uuid
		  AND billing_status IN ('active', 'grace_period', 'past_due')
		  AND state <> 'deleted'
	`, userID); err != nil {
		return err
	}

	if err := s.enqueueInstanceStopsForUserTx(ctx, tx, userID, reason); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	deleteDays := dunningDeleteDays()
	title := "VPS остановлен"
	body := fmt.Sprintf(
		"Срок оплаты истёк. Сервер выключен и недоступен. Пополните баланс в течение %d дн. — иначе VPS будет удалён.",
		deleteDays,
	)
	if reason == "grace_expired" {
		_ = s.NotifyUser(ctx, userID, title, body, "billing", true)
	}
	return nil
}

func (s *Store) deleteExpiredSuspendedInstances(ctx context.Context, deleteDays int) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, COALESCE(hostname, ''),
			COALESCE(external_id, ''), COALESCE(product_type, 'vps')
		FROM vps.instances
		WHERE billing_status = 'suspended'
		  AND state <> 'deleted'
		  AND suspended_at IS NOT NULL
		  AND suspended_at <= now() - make_interval(days => $1)
	`, deleteDays)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var instanceID, userID, hostname, externalID, productType string
		if err := rows.Scan(&instanceID, &userID, &hostname, &externalID, &productType); err != nil {
			return count, err
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			log.Printf("dunning delete begin tx %s: %v", instanceID, err)
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET billing_status = 'cancelled',
			    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
			        || jsonb_build_object('pending_destroy', to_jsonb(now()), 'destroy_reason', 'dunning_delete'),
			    updated_at = now()
			WHERE id = $1::uuid
		`, instanceID); err != nil {
			_ = tx.Rollback(ctx)
			log.Printf("dunning delete instance %s: %v", instanceID, err)
			continue
		}
		if externalID != "" && productType != "dedicated" {
			payload, _ := json.Marshal(map[string]any{
				"instance_id": instanceID,
				"external_id": externalID,
				"user_id":     userID,
				"reason":      "dunning_delete",
			})
			if _, err := tx.Exec(ctx, `
				INSERT INTO vps.outbox (event_type, payload)
				VALUES ('instance.destroy_requested', $1::jsonb)
			`, payload); err != nil {
				_ = tx.Rollback(ctx)
				log.Printf("dunning destroy outbox %s: %v", instanceID, err)
				continue
			}
		}
		if err := tx.Commit(ctx); err != nil {
			log.Printf("dunning delete commit %s: %v", instanceID, err)
			continue
		}
		label := hostname
		if label == "" {
			label = instanceID[:8]
		}
		_ = s.NotifyUser(ctx, userID,
			"VPS удалён за неоплату",
			fmt.Sprintf("Сервер %s удалён после %d дней без оплаты.", label, deleteDays),
			"billing", true)
		count++
	}
	return count, rows.Err()
}

func (s *Store) sendDunningReminders(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id::text, grace_until
		FROM billing.accounts
		WHERE billing_status = 'past_due'
		  AND grace_until IS NOT NULL
		  AND grace_until > now()
		  AND (dunning_reminder_at IS NULL OR dunning_reminder_at <= now() - interval '3 hours')
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID string
		var graceUntil time.Time
		if err := rows.Scan(&userID, &graceUntil); err != nil {
			return count, err
		}
		hoursLeft := int(time.Until(graceUntil).Hours()) + 1
		if hoursLeft < 1 {
			hoursLeft = 1
		}
		body := fmt.Sprintf(
			"Недостаточно средств на балансе. Пополните счёт в течение ~%d ч., иначе VPS будет остановлен.",
			hoursLeft,
		)
		if err := s.NotifyUser(ctx, userID, "Напоминание об оплате", body, "billing", true); err != nil {
			log.Printf("dunning reminder user %s: %v", userID, err)
			continue
		}
		if _, err := s.pool.Exec(ctx, `
			UPDATE billing.accounts SET dunning_reminder_at = now(), updated_at = now()
			WHERE user_id = $1::uuid
		`, userID); err != nil {
			log.Printf("dunning reminder timestamp user %s: %v", userID, err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func (s *Store) enterGracePeriodTx(ctx context.Context, tx pgx.Tx, userID string, graceHours int) error {
	if graceHours <= 0 {
		graceHours = 12
	}
	_, err := tx.Exec(ctx, `
		UPDATE billing.accounts
		SET billing_status = 'past_due',
		    past_due_at = COALESCE(past_due_at, now()),
		    grace_until = COALESCE(grace_until, now() + make_interval(hours => $2)),
		    -- Record the initial past-due notice immediately so the dunning
		    -- worker does not send a second reminder in the same 24h window.
		    dunning_reminder_at = COALESCE(dunning_reminder_at, now()),
		    updated_at = now()
		WHERE user_id = $1::uuid
	`, userID, graceHours)
	return err
}

func dunningDeleteDays() int {
	raw := os.Getenv("DUNNING_DELETE_DAYS")
	if raw == "" {
		return 3
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 3
	}
	return n
}

func (s *Store) TryReactivateAfterPayment(ctx context.Context, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var balance float64
	err = tx.QueryRow(ctx, `
		SELECT billing_status, balance::float8
		FROM billing.accounts
		WHERE user_id = $1::uuid
		FOR UPDATE
	`, userID).Scan(&status, &balance)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if status != "past_due" && status != "suspended" {
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts
		SET billing_status = 'active',
		    past_due_at = NULL,
		    grace_until = NULL,
		    suspended_at = NULL,
		    dunning_reminder_at = NULL,
		    updated_at = now()
		WHERE user_id = $1::uuid
	`, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET billing_status = 'active',
		    suspended_at = NULL,
		    state = CASE
		        WHEN state = 'stopped' AND billing_status = 'suspended' THEN 'starting'
		        WHEN state = 'suspended' THEN 'starting'
		        ELSE state
		    END,
		    updated_at = now()
		WHERE user_id = $1::uuid
		  AND billing_status IN ('suspended', 'grace_period', 'past_due')
		  AND state <> 'deleted'
	`, userID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	_ = s.enqueueInstanceStartsForUser(ctx, userID)

	_ = s.NotifyUser(ctx, userID,
		"Баланс пополнен",
		"Оплата получена. VPS возобновляют работу.",
		"billing", true)

	_, _ = s.ProcessDueChargesForUser(ctx, userID)
	return nil
}

func (s *Store) afterTopupCredit(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	_ = s.TryReactivateAfterPayment(ctx, userID)
}

func (s *Store) ProcessDueChargesForUser(ctx context.Context, userID string) (*ChargeResult, error) {
	due, err := s.listDueInstancesForUser(ctx, userID)
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

func (s *Store) listDueInstancesForUser(ctx context.Context, userID string) ([]DueInstance, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, p.name,
			COALESCE(
				NULLIF((i.provider_meta->>'billing_price_rub')::float8, 0),
				p.price_monthly
			)::float8,
			i.billing_period_days, i.hostname, COALESCE(i.auto_renew, true),
			COALESCE(i.product_type, 'vps')
		FROM vps.instances i
		JOIN vps.plans p ON p.id = i.plan_id
		WHERE i.user_id = $1::uuid
		  AND i.billing_status IN ('active', 'grace_period')
		  AND i.state NOT IN ('deleted', 'creating')
		  AND i.next_billing_at IS NOT NULL
		  AND i.next_billing_at <= now()
		ORDER BY i.next_billing_at ASC
	`, userID)
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

func (s *Store) enqueueInstanceStartsForUser(ctx context.Context, userID string) error {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, COALESCE(i.external_id, ''), COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack')
		FROM vps.instances i
		WHERE i.user_id = $1::uuid
		  AND i.state = 'starting'
		  AND i.state <> 'deleted'
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instanceID, externalID, productType, provider string
		if err := rows.Scan(&instanceID, &externalID, &productType, &provider); err != nil {
			return err
		}
		if productType == "dedicated" || provider == "hetzner_robot" || provider == "hostkey" {
			continue
		}
		if externalID == "" {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"instance_id": instanceID,
			"external_id": externalID,
			"user_id":     userID,
		})
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO vps.outbox (event_type, payload)
			VALUES ('instance.start_requested', $1::jsonb)
		`, payload); err != nil {
			log.Printf("enqueue start %s: %v", instanceID, err)
		}
	}
	return rows.Err()
}

func isHypervisorVPS(productType, provider string) bool {
	if productType == "dedicated" {
		return false
	}
	switch provider {
	case "hetzner_robot", "hostkey":
		return false
	default:
		return true
	}
}

func (s *Store) enqueueInstanceStopInTx(ctx context.Context, tx outboxExecer, instanceID, userID, reason string) error {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil
	}
	var externalID, productType, provider string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(external_id, ''), COALESCE(product_type, 'vps'), COALESCE(provider, 'openstack')
		FROM vps.instances
		WHERE id = $1::uuid
	`, instanceID).Scan(&externalID, &productType, &provider)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if !isHypervisorVPS(productType, provider) || externalID == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"external_id": externalID,
		"user_id":     userID,
		"reason":      reason,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.stop_requested', $1::jsonb)
	`, payload)
	return err
}

func (s *Store) enqueueInstanceStop(ctx context.Context, instanceID, externalID, userID, reason string) error {
	instanceID = strings.TrimSpace(instanceID)
	externalID = strings.TrimSpace(externalID)
	if instanceID == "" || externalID == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"external_id": externalID,
		"user_id":     userID,
		"reason":      reason,
	})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.stop_requested', $1::jsonb)
	`, payload)
	return err
}

func (s *Store) enqueueInstanceStopsForUser(ctx context.Context, userID, reason string) error {
	return s.enqueueInstanceStopsForUserTx(ctx, s.pool, userID, reason)
}

type outboxExecer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type pendingInstanceStop struct {
	instanceID string
	externalID string
	productType string
	provider   string
}

func (s *Store) enqueueInstanceStopsForUserTx(ctx context.Context, tx outboxExecer, userID, reason string) error {
	rows, err := tx.Query(ctx, `
		SELECT i.id::text, COALESCE(i.external_id, ''), COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack')
		FROM vps.instances i
		WHERE i.user_id = $1::uuid
		  AND i.billing_status = 'suspended'
		  AND COALESCE(i.external_id, '') <> ''
		  AND i.state NOT IN ('deleted', 'queued')
	`, userID)
	if err != nil {
		return err
	}
	var pending []pendingInstanceStop
	for rows.Next() {
		var row pendingInstanceStop
		if err := rows.Scan(&row.instanceID, &row.externalID, &row.productType, &row.provider); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, row := range pending {
		if !isHypervisorVPS(row.productType, row.provider) {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"instance_id": row.instanceID,
			"external_id": row.externalID,
			"user_id":     userID,
			"reason":      reason,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO vps.outbox (event_type, payload)
			VALUES ('instance.stop_requested', $1::jsonb)
		`, payload); err != nil {
			return fmt.Errorf("enqueue stop %s: %w", row.instanceID, err)
		}
	}
	return nil
}

// reconcileSuspendedRunningStops enqueues VF stop for instances marked suspended in DB
// but still running on the hypervisor (e.g. after a failed outbox insert).
func (s *Store) reconcileSuspendedRunningStops(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, COALESCE(i.external_id, ''),
			COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack')
		FROM vps.instances i
		WHERE i.billing_status = 'suspended'
		  AND i.state IN ('running', 'starting', 'restarting')
		  AND COALESCE(i.external_id, '') <> ''
		  AND i.state <> 'deleted'
		  AND NOT EXISTS (
		    SELECT 1 FROM vps.outbox o
		    WHERE o.event_type = 'instance.stop_requested'
		      AND o.published = false
		      AND o.payload->>'instance_id' = i.id::text
		  )
	`)
	if err != nil {
		return 0, err
	}
	var pending []struct {
		instanceID, userID, externalID, productType, provider string
	}
	for rows.Next() {
		var row struct {
			instanceID, userID, externalID, productType, provider string
		}
		if err := rows.Scan(&row.instanceID, &row.userID, &row.externalID, &row.productType, &row.provider); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	count := 0
	for _, row := range pending {
		if !isHypervisorVPS(row.productType, row.provider) {
			continue
		}
		if err := s.enqueueInstanceStop(ctx, row.instanceID, row.externalID, row.userID, "reconcile_suspend"); err != nil {
			log.Printf("reconcile stop %s: %v", row.instanceID, err)
			continue
		}
		count++
	}
	return count, nil
}

func (s *Store) enqueueInstanceStopByID(ctx context.Context, instanceID, userID, reason string) error {
	var externalID, productType, provider string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(external_id, ''), COALESCE(product_type, 'vps'), COALESCE(provider, 'openstack')
		FROM vps.instances
		WHERE id = $1::uuid
	`, instanceID).Scan(&externalID, &productType, &provider)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if !isHypervisorVPS(productType, provider) {
		return nil
	}
	return s.enqueueInstanceStop(ctx, instanceID, externalID, userID, reason)
}
