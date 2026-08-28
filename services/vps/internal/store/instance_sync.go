package store

import (
	"context"
	"strings"
)

type HypervisorInstanceRow struct {
	ID                string
	ExternalID        string
	State             string
	BillingStatus     string
	IPAddress         *string
	HasProvisionError bool
}

func (s *Store) ListInstancesForHypervisorSync(ctx context.Context, limit int) ([]HypervisorInstanceRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, external_id, state, COALESCE(billing_status, 'active'), NULLIF(host(ip_address)::text, ''),
		       COALESCE(NULLIF(TRIM(provider_meta->>'provision_error'), ''), '') <> ''
		FROM vps.instances
		WHERE external_id IS NOT NULL AND TRIM(external_id) <> ''
		  AND state NOT IN ('deleted', 'creating', 'queued', 'reinstalling', 'error')
		ORDER BY updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []HypervisorInstanceRow
	for rows.Next() {
		var row HypervisorInstanceRow
		if err := rows.Scan(&row.ID, &row.ExternalID, &row.State, &row.BillingStatus, &row.IPAddress, &row.HasProvisionError); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

// MarkInstanceOrphaned marks a live instance as error when VirtFusion has no VM.
// ok=false when the row was already finalized (e.g. provision failure refund done).
func (s *Store) MarkInstanceOrphaned(ctx context.Context, instanceID, reason string) (ok bool, err error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "hypervisor server missing"
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'error',
		    ip_address = NULL,
		    external_id = NULL,
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        || jsonb_build_object('provision_error', $2::text, 'provision_failed_at', to_jsonb(now())),
		    updated_at = now()
		WHERE id = $1::uuid
		  AND state IN ('running', 'stopped', 'creating', 'reinstalling')
	`, instanceID, reason)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) IncrementInstanceSyncMiss(ctx context.Context, instanceID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object(
		        'sync_miss_count',
		        COALESCE((provider_meta->>'sync_miss_count')::int, 0) + 1
		    ),
		    updated_at = now()
		WHERE id = $1::uuid
		RETURNING COALESCE((provider_meta->>'sync_miss_count')::int, 0)
	`, instanceID).Scan(&count)
	return count, err
}

func (s *Store) ClearInstanceSyncMiss(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'sync_miss_count',
		    updated_at = now()
		WHERE id = $1::uuid
		  AND provider_meta ? 'sync_miss_count'
	`, instanceID)
	return err
}

func (s *Store) ClearInstanceProvisionError(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    - 'provision_error' - 'provision_failed_at',
		    updated_at = now()
		WHERE id = $1::uuid
		  AND NULLIF(TRIM(provider_meta->>'provision_error'), '') IS NOT NULL
	`, instanceID)
	return err
}

func (s *Store) MarkInstanceStopped(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'stopped', updated_at = now()
		WHERE id = $1::uuid
		  AND state NOT IN ('deleted', 'stopped')
	`, instanceID)
	return err
}

func (s *Store) UpdateInstanceFromHypervisor(ctx context.Context, instanceID, state, ip string) error {
	if state == "running" && strings.TrimSpace(ip) == "" {
		state = ""
	}
	if state == "" && ip == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances SET
			state = CASE WHEN $2 <> '' THEN $2 ELSE state END,
			ip_address = CASE
				WHEN $3 <> '' AND (
					ip_address IS NULL OR host(ip_address) = ''
					OR host(ip_address) IS DISTINCT FROM $3
				) THEN NULLIF($3, '')::inet
				ELSE ip_address
			END,
			updated_at = now()
		WHERE id = $1::uuid
		  AND (
			($2 <> '' AND state IS DISTINCT FROM $2)
			OR ($3 <> '' AND (
				ip_address IS NULL OR host(ip_address) = ''
				OR host(ip_address) IS DISTINCT FROM $3
			))
		  )
	`, instanceID, state, ip)
	return err
}
