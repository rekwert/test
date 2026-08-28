package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrUserNotFound = errors.New("user not found")

func (s *Store) ResolveUserIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM auth.users
		WHERE lower(email) = lower($1) AND deleted_at IS NULL
	`, email).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", ErrUserNotFound
	}
	return id, err
}

type InstanceDetail struct {
	ID                string
	UserID            string
	Hostname          *string
	State             string
	IPAddress         *string
	Region            string
	PlanID            string
	PlanName          string
	BillingStatus     string
	NodeID            *string
	NodeName          *string
	NodeStatus        *string
	OSTemplateID      string
	ExternalID        *string
	BillingPeriodDays int
	NextBillingAt     *time.Time
	CreatedAt         time.Time
}

func (s *Store) GetInstanceDetail(ctx context.Context, instanceID string) (*InstanceDetail, error) {
	var row InstanceDetail
	var nextBilling *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT i.id::text, i.user_id::text, i.hostname, i.state, host(i.ip_address)::text,
			i.region, i.plan_id::text, COALESCE(p.name, ''), i.billing_status,
			n.id::text, n.name, n.status, i.external_id,
			COALESCE(o.os_template_id, ''),
			COALESCE(i.billing_period_days, 30), i.next_billing_at, i.created_at
		FROM vps.instances i
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		WHERE i.id = $1::uuid
	`, instanceID).Scan(
		&row.ID, &row.UserID, &row.Hostname, &row.State, &row.IPAddress,
		&row.Region, &row.PlanID, &row.PlanName, &row.BillingStatus,
		&row.NodeID, &row.NodeName, &row.NodeStatus, &row.ExternalID,
		&row.OSTemplateID, &row.BillingPeriodDays, &nextBilling, &row.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	row.NextBillingAt = nextBilling
	return &row, nil
}

type NodeStats struct {
	Name            string
	Status          string
	Region          string
	Uptime          string
	LoadPercent     float64
	InstanceCount   int
	Capacity        int
	VFEnabled       *bool
	MaintenanceMode bool
	VFServerCount   *int
	LastSyncedAt    *time.Time
}

func (s *Store) GetNodeStats(ctx context.Context, nodeID string) (*NodeStats, error) {
	var createdAt time.Time
	var stats NodeStats
	var cpuUsed, memUsed *float64
	err := s.pool.QueryRow(ctx, `
		SELECT n.name, n.status, n.region, n.created_at, n.capacity_instances,
			(SELECT COUNT(*)::int FROM vps.instances i
			 WHERE i.node_id = n.id AND i.state <> 'deleted'),
			n.cpu_used_percent, n.memory_used_percent, n.vf_enabled, n.maintenance_mode,
			n.vf_server_count, n.last_synced_at
		FROM vps.nodes n
		WHERE n.id = $1::uuid
	`, nodeID).Scan(
		&stats.Name, &stats.Status, &stats.Region, &createdAt, &stats.Capacity, &stats.InstanceCount,
		&cpuUsed, &memUsed, &stats.VFEnabled, &stats.MaintenanceMode, &stats.VFServerCount, &stats.LastSyncedAt,
	)
	if err != nil {
		return nil, err
	}
	stats.Uptime = formatNodeDuration(time.Since(createdAt))
	if cpuUsed != nil && *cpuUsed > 0 {
		stats.LoadPercent = *cpuUsed
	} else if memUsed != nil && *memUsed > 0 {
		stats.LoadPercent = *memUsed
	} else if stats.Capacity > 0 {
		stats.LoadPercent = float64(stats.InstanceCount) / float64(stats.Capacity) * 100
	}
	return &stats, nil
}

func formatNodeDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func (s *Store) ExtendInstanceBilling(ctx context.Context, instanceID string, extendDays int, billingPeriodDays *int, staffID, reason string) (*InstanceDetail, error) {
	if extendDays == 0 && billingPeriodDays == nil {
		return nil, fmt.Errorf("extend_days or billing_period_days required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM vps.instances WHERE id = $1::uuid`, instanceID).Scan(&userID)
	if err != nil {
		return nil, err
	}

	if extendDays != 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET next_billing_at = GREATEST(
				now(),
				COALESCE(next_billing_at, now()) + make_interval(days => $2)
			),
				updated_at = now()
			WHERE id = $1::uuid
		`, instanceID, extendDays); err != nil {
			return nil, err
		}
	}
	if billingPeriodDays != nil && *billingPeriodDays > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET billing_period_days = $2, updated_at = now()
			WHERE id = $1::uuid
		`, instanceID, *billingPeriodDays); err != nil {
			return nil, err
		}
	}

	details, _ := json.Marshal(map[string]any{
		"extend_days":         extendDays,
		"billing_period_days": billingPeriodDays,
		"reason":              reason,
	})
	if err := insertAdminAction(ctx, tx, staffID, userID, instanceID, "billing_extend", details); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetInstanceDetail(ctx, instanceID)
}

func (s *Store) CountFailedOutbox(ctx context.Context, instanceID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM vps.outbox
		WHERE published = false
		AND payload->>'instance_id' = $1
	`, instanceID).Scan(&count)
	return count, err
}

func (s *Store) TransferClient(ctx context.Context, fromUserID, toUserID, staffID string, transferBalance bool, reason string) (map[string]any, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var fromExists, toExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id = $1::uuid AND deleted_at IS NULL)`, fromUserID).Scan(&fromExists); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id = $1::uuid AND deleted_at IS NULL)`, toUserID).Scan(&toExists); err != nil {
		return nil, err
	}
	if !fromExists || !toExists {
		return nil, ErrUserNotFound
	}
	if fromUserID == toUserID {
		return nil, fmt.Errorf("cannot transfer to the same user")
	}

	var instanceCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM vps.instances
		WHERE user_id = $1::uuid AND state <> 'deleted'
	`, fromUserID).Scan(&instanceCount); err != nil {
		return nil, err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE vps.instances SET user_id = $2::uuid, updated_at = now()
		WHERE user_id = $1::uuid AND state <> 'deleted'
	`, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	instancesMoved := int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `
		UPDATE vps.orders SET user_id = $2::uuid
		WHERE user_id = $1::uuid
	`, fromUserID, toUserID); err != nil {
		return nil, err
	}
	ticketTag, err := tx.Exec(ctx, `
		UPDATE support.tickets SET user_id = $2::uuid
		WHERE user_id = $1::uuid AND status NOT IN ('closed', 'resolved')
	`, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	ticketsMoved := int(ticketTag.RowsAffected())

	var balanceMoved float64
	if transferBalance {
		var balance float64
		err = tx.QueryRow(ctx, `
			SELECT COALESCE(balance, 0)::float8 FROM billing.accounts WHERE user_id = $1::uuid
		`, fromUserID).Scan(&balance)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if balance > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO billing.accounts (user_id, balance) VALUES ($1, 0)
				ON CONFLICT (user_id) DO NOTHING
			`, toUserID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE billing.accounts SET balance = balance - $2, updated_at = now()
				WHERE user_id = $1::uuid
			`, fromUserID, balance); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE billing.accounts SET balance = balance + $2, updated_at = now()
				WHERE user_id = $1::uuid
			`, toUserID, balance); err != nil {
				return nil, err
			}
			transferReason := reason
			if transferReason == "" {
				transferReason = "account transfer"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO billing.adjustments (user_id, amount, kind, reason, staff_id)
				VALUES ($1, $2, 'debit', $3, NULLIF($4, '')::uuid)
			`, fromUserID, balance, "transfer out: "+transferReason, staffID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO billing.adjustments (user_id, amount, kind, reason, staff_id)
				VALUES ($1, $2, 'credit', $3, NULLIF($4, '')::uuid)
			`, toUserID, balance, "transfer in: "+transferReason, staffID); err != nil {
				return nil, err
			}
			balanceMoved = balance
		}
	}

	details, _ := json.Marshal(map[string]any{
		"from_user_id":      fromUserID,
		"to_user_id":        toUserID,
		"instances_moved":   instancesMoved,
		"instances_total":   instanceCount,
		"tickets_moved":     ticketsMoved,
		"balance_moved":     balanceMoved,
		"transfer_balance":  transferBalance,
		"reason":            reason,
	})
	if err := insertAdminAction(ctx, tx, staffID, fromUserID, "", "client_transfer", details); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":              true,
		"instances_moved": instancesMoved,
		"tickets_moved":   ticketsMoved,
		"balance_moved":   balanceMoved,
		"to_user_id":      toUserID,
	}, nil
}
