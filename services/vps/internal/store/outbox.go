package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type OutboxEvent struct {
	ID        int64
	EventType string
	Payload   json.RawMessage
}

var ErrPasswordChangePending = errors.New("password change already pending")

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func insertProvisionOutboxTx(ctx context.Context, tx pgx.Tx, payload json.RawMessage) (pgconn.CommandTag, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.provision_requested', $1::jsonb)
	`, payload)
	if err != nil && isUniqueViolation(err) {
		return tag, nil
	}
	return tag, err
}

func insertReinstallOutboxTx(ctx context.Context, tx pgx.Tx, payload json.RawMessage) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.reinstall_requested', $1::jsonb)
	`, payload)
	if err != nil && isUniqueViolation(err) {
		return nil
	}
	return err
}

func (s *Store) FetchPendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	return s.ClaimPendingOutbox(ctx, "worker", limit)
}

func (s *Store) ClaimPendingOutbox(ctx context.Context, workerID string, limit int) ([]OutboxEvent, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "worker"
	}
	rows, err := s.pool.Query(ctx, `
		UPDATE vps.outbox AS o
		SET worker_poll_claimed_at = now(), worker_poll_claimed_by = $2
		WHERE o.id IN (
			SELECT id FROM vps.outbox
			WHERE published = false
			  AND (worker_poll_claimed_at IS NULL OR worker_poll_claimed_at < now() - interval '10 minutes')
			ORDER BY
			  CASE
			    WHEN event_type IN ('instance.provision_requested', 'instance.reinstall_requested') THEN 1
			    ELSE 0
			  END,
			  id ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING o.id, o.event_type, o.payload
	`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OutboxEvent
	for rows.Next() {
		var item OutboxEvent
		if err := rows.Scan(&item.ID, &item.EventType, &item.Payload); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ReleaseOutboxClaim(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.outbox
		SET worker_poll_claimed_at = NULL, worker_poll_claimed_by = NULL
		WHERE id = $1 AND published = false
	`, id)
	return err
}

func (s *Store) MarkOutboxPublished(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.outbox
		SET published = true, worker_poll_claimed_at = NULL, worker_poll_claimed_by = NULL
		WHERE id = $1
	`, id)
	return err
}

func (s *Store) CompleteProvisioning(ctx context.Context, instanceID, externalID, ip, nodeID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET external_id = $2,
		    ip_address = NULLIF($3, '')::inet,
		    node_id = NULLIF($4, '')::uuid,
		    state = 'running',
		    next_billing_at = CASE
		      WHEN next_billing_at IS NULL THEN
		        now() + make_interval(days => COALESCE(
		          NULLIF((provider_meta->>'initial_prepaid_days')::int, 0),
		          NULLIF(billing_period_days, 0),
		          30
		        ))
		      ELSE next_billing_at
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, externalID, ip, nodeID)
	return err
}

func (s *Store) ListCreatingInstances(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text FROM vps.instances
		WHERE state = 'creating'
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) UpdateInstanceMetrics(ctx context.Context, instanceID string, metrics json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET metrics = $2::jsonb, metrics_updated_at = now(), updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, metrics)
	return err
}

func (s *Store) ListRunningInstanceIDs(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text FROM vps.instances
		WHERE state = 'running' AND billing_status IN ('active', 'grace_period')
		ORDER BY metrics_updated_at NULLS FIRST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type SnapshotRow struct {
	ID        string
	Name      string
	Status    string
	SizeGB    *int
	CreatedAt time.Time
}

func (s *Store) ListSnapshots(ctx context.Context, instanceID string) ([]SnapshotRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, status, size_gb, created_at
		FROM vps.instance_snapshots
		WHERE instance_id = $1::uuid
		ORDER BY created_at DESC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SnapshotRow
	for rows.Next() {
		var row SnapshotRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Status, &row.SizeGB, &row.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) InsertSnapshot(ctx context.Context, instanceID, name, status, externalID string, sizeGB int) (*SnapshotRow, error) {
	var row SnapshotRow
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vps.instance_snapshots (instance_id, name, status, external_id, size_gb)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text, name, status, size_gb, created_at
	`, instanceID, name, status, externalID, sizeGB).Scan(&row.ID, &row.Name, &row.Status, &row.SizeGB, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) MarkProvisionOutboxPublished(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.outbox
		SET published = true, worker_poll_claimed_at = NULL, worker_poll_claimed_by = NULL
		WHERE published = false
		  AND event_type = 'instance.provision_requested'
		  AND payload->>'instance_id' = $1
	`, instanceID)
	return err
}

func (s *Store) MarkReinstallOutboxPublished(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.outbox
		SET published = true, worker_poll_claimed_at = NULL, worker_poll_claimed_by = NULL
		WHERE published = false
		  AND event_type = 'instance.reinstall_requested'
		  AND payload->>'instance_id' = $1
	`, instanceID)
	return err
}

func (s *Store) GetSnapshotExternalID(ctx context.Context, instanceID, snapshotID string) (string, error) {
	var externalID *string
	err := s.pool.QueryRow(ctx, `
		SELECT external_id FROM vps.instance_snapshots
		WHERE id = $1::uuid AND instance_id = $2::uuid
	`, snapshotID, instanceID).Scan(&externalID)
	if err != nil {
		return "", err
	}
	if externalID == nil {
		return "", nil
	}
	return strings.TrimSpace(*externalID), nil
}

func (s *Store) DeleteSnapshot(ctx context.Context, instanceID, snapshotID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM vps.instance_snapshots
		WHERE id = $1::uuid AND instance_id = $2::uuid
	`, snapshotID, instanceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

type BackupRow struct {
	ID        string
	Name      string
	Status    string
	Schedule  *string
	CreatedAt time.Time
}

func (s *Store) ListBackups(ctx context.Context, instanceID string) ([]BackupRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, status, schedule, created_at
		FROM vps.instance_backups
		WHERE instance_id = $1::uuid
		ORDER BY created_at DESC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []BackupRow
	for rows.Next() {
		var row BackupRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Status, &row.Schedule, &row.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) InsertBackup(ctx context.Context, instanceID, name, status, externalID, schedule string) (*BackupRow, error) {
	var row BackupRow
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vps.instance_backups (instance_id, name, status, external_id, schedule)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5, ''))
		RETURNING id::text, name, status, schedule, created_at
	`, instanceID, name, status, externalID, schedule).Scan(
		&row.ID, &row.Name, &row.Status, &row.Schedule, &row.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) GetInstanceExternalID(ctx context.Context, instanceID string) (string, error) {
	var externalID *string
	err := s.pool.QueryRow(ctx, `
		SELECT external_id FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&externalID)
	if err != nil {
		return "", err
	}
	if externalID == nil {
		return "", nil
	}
	return strings.TrimSpace(*externalID), nil
}

func (s *Store) GetInstanceMetricsRaw(ctx context.Context, instanceID string) (json.RawMessage, *time.Time, error) {
	var metrics json.RawMessage
	var updatedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT metrics, metrics_updated_at FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&metrics, &updatedAt)
	return metrics, updatedAt, err
}

func (s *Store) SetInstanceRunning(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances SET state = 'running', updated_at = now() WHERE id = $1::uuid
	`, instanceID)
	return err
}

func (s *Store) EnqueueReinstall(ctx context.Context, instanceID, osTemplateID, rootPassword string, sshKeys []string) error {
	payload, _ := json.Marshal(map[string]any{
		"instance_id":    instanceID,
		"os_template_id": osTemplateID,
		"ssh_keys":       sshKeys,
	})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.reinstall_requested', $1::jsonb)
	`, payload)
	return err
}

// BeginReinstall sets reinstalling state and enqueues worker event atomically.
func (s *Store) BeginReinstall(ctx context.Context, instanceID, osTemplateID, rootPassword string, sshKeys []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'reinstalling', updated_at = now(),
		    worker_poll_claimed_at = NULL,
		    worker_poll_claimed_by = NULL,
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        - 'provision_error' - 'provision_failed_at' - 'guest_agent_warmup_at' - 'vf_password_reset_at' - 'reinstall_build_started'
		WHERE id = $1::uuid
		  AND state IN ('running', 'stopped', 'error')
	`, instanceID); err != nil {
		return err
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM vps.instances WHERE id = $1::uuid`, instanceID).Scan(&state); err != nil {
		return err
	}
	if state != "reinstalling" {
		return fmt.Errorf("instance not available for reinstall")
	}
	if osTemplateID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.orders o
			SET os_template_id = $2
			FROM vps.instances i
			WHERE i.order_id = o.id AND i.id = $1::uuid
		`, instanceID, osTemplateID); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"instance_id":    instanceID,
		"os_template_id": osTemplateID,
		"ssh_keys":       sshKeys,
	})
	if err := insertReinstallOutboxTx(ctx, tx, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) EnqueueInstanceStart(ctx context.Context, instanceID, externalID, userID string) error {
	payload, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"external_id": externalID,
		"user_id":     userID,
	})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('instance.start_requested', $1::jsonb)
	`, payload)
	return err
}

func (s *Store) EnqueueInstanceStop(ctx context.Context, instanceID, externalID, userID, reason string) error {
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

func (s *Store) EnqueuePasswordChange(ctx context.Context, instanceID, userID string) error {
	instanceID = strings.TrimSpace(instanceID)
	userID = strings.TrimSpace(userID)
	if instanceID == "" || userID == "" {
		return fmt.Errorf("instance_id and user_id required")
	}
	payload, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"user_id":     userID,
	})
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		SELECT 'instance.password_change_requested', $1::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM vps.outbox
			WHERE published = false
			  AND event_type = 'instance.password_change_requested'
			  AND payload->>'instance_id' = $2
		)
	`, payload, instanceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPasswordChangePending
	}
	return nil
}

func (s *Store) HasPendingPasswordChange(ctx context.Context, instanceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM vps.outbox
			WHERE published = false
			  AND event_type = 'instance.password_change_requested'
			  AND payload->>'instance_id' = $1
		)
	`, instanceID).Scan(&exists)
	return exists, err
}

func (s *Store) EnqueueInstanceDestroy(ctx context.Context, instanceID, externalID, userID string) error {
	instanceID = strings.TrimSpace(instanceID)
	externalID = strings.TrimSpace(externalID)
	userID = strings.TrimSpace(userID)
	if instanceID == "" {
		return fmt.Errorf("instance_id required")
	}
	payload, _ := json.Marshal(map[string]any{
		"instance_id": instanceID,
		"external_id": externalID,
		"user_id":     userID,
	})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		SELECT 'instance.destroy_requested', $1::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM vps.outbox
			WHERE published = false
			  AND event_type = 'instance.destroy_requested'
			  AND payload->>'instance_id' = $2
		)
	`, payload, instanceID)
	return err
}

func (s *Store) SetInstanceReinstalling(ctx context.Context, instanceID, osTemplateID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'reinstalling', updated_at = now(),
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        - 'provision_error' - 'provision_failed_at' - 'guest_agent_warmup_at'
		WHERE id = $1::uuid
		  AND state IN ('running', 'stopped', 'error')
	`, instanceID); err != nil {
		return err
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM vps.instances WHERE id = $1::uuid`, instanceID).Scan(&state); err != nil {
		return err
	}
	if state != "reinstalling" {
		return fmt.Errorf("instance not available for reinstall")
	}
	if osTemplateID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE vps.orders o
			SET os_template_id = $2
			FROM vps.instances i
			WHERE i.order_id = o.id AND i.id = $1::uuid
		`, instanceID, osTemplateID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
