package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	IPSourceProvision           = "provision"
	IPSourceDedicatedProvision  = "dedicated_provision"
	IPSourceChangeIP            = "change_ip"
	IPSourceExtraIPv4           = "extra_ipv4"
	IPSourceInstanceDeleted     = "instance_deleted"
)

type IPAssignmentLogOpts struct {
	Source   string
	ActorID  string
	Metadata map[string]any
}

type IPAssignmentLogRow struct {
	ID         int64
	UserID     string
	InstanceID *string
	IPAddress  string
	Event      string
	Source     string
	OldIP      *string
	ActorID    *string
	Metadata   json.RawMessage
	CreatedAt  time.Time
}

func (s *Store) LogIPAssigned(ctx context.Context, userID, instanceID, ip, oldIP string, opts *IPAssignmentLogOpts) error {
	return s.insertIPAssignmentLog(ctx, userID, instanceID, ip, "assigned", oldIP, opts)
}

func (s *Store) LogIPReleased(ctx context.Context, userID, instanceID, ip string, opts *IPAssignmentLogOpts) error {
	return s.insertIPAssignmentLog(ctx, userID, instanceID, ip, "released", "", opts)
}

func (s *Store) LogIPChange(ctx context.Context, userID, instanceID, oldIP, newIP string, opts *IPAssignmentLogOpts) error {
	oldIP = strings.TrimSpace(oldIP)
	newIP = strings.TrimSpace(newIP)
	if oldIP == "" || newIP == "" || oldIP == newIP {
		return nil
	}
	if err := s.LogIPReleased(ctx, userID, instanceID, oldIP, opts); err != nil {
		return err
	}
	return s.LogIPAssigned(ctx, userID, instanceID, newIP, oldIP, opts)
}

func (s *Store) insertIPAssignmentLog(ctx context.Context, userID, instanceID, ip, event, oldIP string, opts *IPAssignmentLogOpts) error {
	ip = strings.TrimSpace(ip)
	if ip == "" || strings.TrimSpace(userID) == "" {
		return nil
	}
	source := IPSourceProvision
	var actorID string
	var meta json.RawMessage
	if opts != nil {
		if strings.TrimSpace(opts.Source) != "" {
			source = strings.TrimSpace(opts.Source)
		}
		actorID = strings.TrimSpace(opts.ActorID)
		if len(opts.Metadata) > 0 {
			meta, _ = json.Marshal(opts.Metadata)
		}
	}
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.ip_assignment_log (user_id, instance_id, ip_address, event, source, old_ip, actor_id, metadata)
		VALUES (
			$1::uuid,
			NULLIF($2, '')::uuid,
			$3::inet,
			$4,
			$5,
			NULLIF($6, '')::inet,
			NULLIF($7, '')::uuid,
			$8::jsonb
		)
	`, userID, instanceID, ip, event, source, oldIP, actorID, meta)
	return err
}

func (s *Store) ListClientIPHistory(ctx context.Context, userID string, limit int) ([]IPAssignmentLogRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id,
		       user_id::text,
		       instance_id::text,
		       host(ip_address),
		       event,
		       source,
		       host(old_ip),
		       actor_id::text,
		       metadata,
		       created_at
		FROM vps.ip_assignment_log
		WHERE user_id = $1::uuid
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []IPAssignmentLogRow
	for rows.Next() {
		var row IPAssignmentLogRow
		if err := rows.Scan(
			&row.ID,
			&row.UserID,
			&row.InstanceID,
			&row.IPAddress,
			&row.Event,
			&row.Source,
			&row.OldIP,
			&row.ActorID,
			&row.Metadata,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) logInstanceIPReleaseIfAny(ctx context.Context, instanceID, source, actorID string) {
	var userID string
	var ip *string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id::text, host(ip_address)
		FROM vps.instances
		WHERE id = $1::uuid
	`, instanceID).Scan(&userID, &ip)
	if err != nil || ip == nil || strings.TrimSpace(*ip) == "" {
		return
	}
	_ = s.LogIPReleased(ctx, userID, instanceID, *ip, &IPAssignmentLogOpts{
		Source:  source,
		ActorID: actorID,
	})
}
