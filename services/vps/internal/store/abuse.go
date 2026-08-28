package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type AbuseSignalRow struct {
	ID         int64
	InstanceID string
	UserID     string
	SignalType string
	Weight     int
	Evidence   json.RawMessage
	CreatedAt  time.Time
}

type AbuseCaseRow struct {
	ID             string
	InstanceID     string
	UserID         string
	Status         string
	TotalScore     int
	TriggerReason  string
	TriggerSignals json.RawMessage
	AutoStoppedAt  *time.Time
	CreatedAt      time.Time
}

type AbuseScanInstance struct {
	ID         string
	UserID     string
	Hostname   *string
	IPAddress  *string
	State      string
	AbuseHold  bool
	AbuseState json.RawMessage
	Metrics    json.RawMessage
	CreatedAt  time.Time
	ExternalID *string
}

func (s *Store) HasOpenAbuseCase(ctx context.Context, instanceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM vps.abuse_cases
			WHERE instance_id = $1::uuid
			  AND status IN ('open', 'auto_stopped', 'confirmed')
		)
	`, instanceID).Scan(&exists)
	return exists, err
}

func (s *Store) InstanceAbuseHold(ctx context.Context, instanceID string) (bool, error) {
	var hold bool
	err := s.pool.QueryRow(ctx, `
		SELECT abuse_hold FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&hold)
	return hold, err
}

func (s *Store) RecentAbuseSignalExists(ctx context.Context, instanceID, signalType string, since time.Time) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM vps.abuse_signals
			WHERE instance_id = $1::uuid
			  AND signal_type = $2
			  AND created_at >= $3
		)
	`, instanceID, signalType, since).Scan(&exists)
	return exists, err
}

func (s *Store) InsertAbuseSignal(ctx context.Context, instanceID, userID, signalType string, weight int, evidence json.RawMessage) (int64, error) {
	if len(evidence) == 0 {
		evidence = json.RawMessage(`{}`)
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vps.abuse_signals (instance_id, user_id, signal_type, weight, evidence)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
		RETURNING id
	`, instanceID, userID, signalType, weight, evidence).Scan(&id)
	return id, err
}

func (s *Store) ListRecentAbuseSignals(ctx context.Context, instanceID string, since time.Time) ([]AbuseSignalRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, instance_id::text, user_id::text, signal_type, weight, evidence, created_at
		FROM vps.abuse_signals
		WHERE instance_id = $1::uuid AND created_at >= $2
		ORDER BY created_at ASC
	`, instanceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AbuseSignalRow, 0)
	for rows.Next() {
		var row AbuseSignalRow
		if err := rows.Scan(&row.ID, &row.InstanceID, &row.UserID, &row.SignalType, &row.Weight, &row.Evidence, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) CreateAbuseCase(ctx context.Context, instanceID, userID, status, reason string, totalScore int, triggerSignals json.RawMessage) (string, error) {
	if len(triggerSignals) == 0 {
		triggerSignals = json.RawMessage(`[]`)
	}
	var id string
	var autoStopped *time.Time
	if status == "auto_stopped" {
		now := time.Now().UTC()
		autoStopped = &now
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vps.abuse_cases (
			instance_id, user_id, status, total_score, trigger_reason, trigger_signals, auto_stopped_at
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::jsonb, $7)
		RETURNING id::text
	`, instanceID, userID, status, totalScore, reason, triggerSignals, autoStopped).Scan(&id)
	return id, err
}

func (s *Store) SetInstanceAbuseHold(ctx context.Context, instanceID string, hold bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances SET abuse_hold = $2, updated_at = now() WHERE id = $1::uuid
	`, instanceID, hold)
	return err
}

func (s *Store) UpdateInstanceAbuseState(ctx context.Context, instanceID string, state json.RawMessage) error {
	if len(state) == 0 {
		state = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances SET abuse_state = $2::jsonb, updated_at = now() WHERE id = $1::uuid
	`, instanceID, state)
	return err
}

func (s *Store) ListInstancesForAbuseScan(ctx context.Context, limit int) ([]AbuseScanInstance, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, i.hostname, host(i.ip_address)::text,
			i.state, i.abuse_hold, i.abuse_state, i.metrics, i.created_at,
			i.external_id
		FROM vps.instances i
		WHERE i.state = 'running'
		  AND i.ip_address IS NOT NULL
		  AND COALESCE(i.product_type, 'vps') = 'vps'
		  AND COALESCE(i.provider, 'openstack') NOT IN ('hetzner_robot', 'hostkey')
		ORDER BY i.metrics_updated_at NULLS FIRST, i.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AbuseScanInstance, 0)
	for rows.Next() {
		var row AbuseScanInstance
		var ip *string
		var ext *string
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.Hostname, &ip, &row.State, &row.AbuseHold,
			&row.AbuseState, &row.Metrics, &row.CreatedAt, &ext,
		); err != nil {
			return nil, err
		}
		row.IPAddress = ip
		row.ExternalID = ext
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ApplyAbuseAutoStop(ctx context.Context, instanceID, userID, caseID, reason string, totalScore int, triggerSignals json.RawMessage) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'stopped', abuse_hold = true, updated_at = now()
		WHERE id = $1::uuid AND state <> 'deleted'
	`, instanceID); err != nil {
		return err
	}

	details, _ := json.Marshal(map[string]any{
		"case_id":      caseID,
		"reason":       reason,
		"total_score":  totalScore,
		"auto":         true,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.admin_actions (staff_id, user_id, instance_id, action, details)
		VALUES (NULL, $1::uuid, $2::uuid, 'abuse_auto_stop', $3::jsonb)
	`, userID, instanceID, details); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) ResolveAbuseCaseFalsePositive(ctx context.Context, caseID, staffID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var instanceID string
	err = tx.QueryRow(ctx, `
		UPDATE vps.abuse_cases
		SET status = 'false_positive', resolved_at = now(), resolved_by = $2::uuid, updated_at = now()
		WHERE id = $1::uuid AND status IN ('auto_stopped', 'open', 'confirmed')
		RETURNING instance_id::text
	`, caseID, staffID).Scan(&instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("case not found")
		}
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances SET abuse_hold = false, updated_at = now() WHERE id = $1::uuid
	`, instanceID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
