package store

import (
	"context"
	"encoding/json"
)

type SoftwareInstallRetry struct {
	ID                string
	IP                string
	ExternalID        string
	OSTemplateID      string
	SoftwareProfileID string
}

// ListSoftwareInstallRetries returns running VirtFusion VPS with a failed software preinstall.
func (s *Store) ListSoftwareInstallRetries(ctx context.Context, limit int) ([]SoftwareInstallRetry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id,
		       host(i.ip_address)::text,
		       COALESCE(i.external_id, ''),
		       COALESCE(o.os_template_id, ''),
		       COALESCE(NULLIF(o.software_profile_id, ''), 'clean')
		FROM vps.instances i
		JOIN vps.orders o ON o.id = i.order_id
		WHERE i.state = 'running'
		  AND NULLIF(TRIM(i.provider_meta->>'software_install_error'), '') IS NOT NULL
		  AND COALESCE(NULLIF(o.software_profile_id, ''), 'clean') NOT IN ('', 'clean')
		  AND COALESCE(i.provider, 'openstack') = 'openstack'
		  AND i.ip_address IS NOT NULL
		  AND (
		    i.provider_meta->>'software_install_retry_at' IS NULL
		    OR (i.provider_meta->>'software_install_retry_at')::timestamptz <= now() - interval '3 minutes'
		  )
		ORDER BY i.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SoftwareInstallRetry
	for rows.Next() {
		var item SoftwareInstallRetry
		if err := rows.Scan(&item.ID, &item.IP, &item.ExternalID, &item.OSTemplateID, &item.SoftwareProfileID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListClaudeCodeHealthChecks returns running Claude Code VPS that should expose the web terminal.
func (s *Store) ListClaudeCodeHealthChecks(ctx context.Context, limit int) ([]SoftwareInstallRetry, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id,
		       host(i.ip_address)::text,
		       COALESCE(i.external_id, ''),
		       COALESCE(o.os_template_id, ''),
		       COALESCE(NULLIF(o.software_profile_id, ''), 'clean')
		FROM vps.instances i
		JOIN vps.orders o ON o.id = i.order_id
		WHERE i.state = 'running'
		  AND COALESCE(NULLIF(o.software_profile_id, ''), 'clean') = 'claude-code'
		  AND COALESCE(i.provider, 'openstack') = 'openstack'
		  AND i.ip_address IS NOT NULL
		  AND (
		    NULLIF(TRIM(i.provider_meta->>'software_install_error'), '') IS NOT NULL
		    OR i.provider_meta->'software_bundle'->'panel'->>'url' IS NOT NULL
		  )
		  AND (
		    i.provider_meta->>'software_install_retry_at' IS NULL
		    OR (i.provider_meta->>'software_install_retry_at')::timestamptz <= now() - interval '3 minutes'
		  )
		ORDER BY i.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SoftwareInstallRetry
	for rows.Next() {
		var item SoftwareInstallRetry
		if err := rows.Scan(&item.ID, &item.IP, &item.ExternalID, &item.OSTemplateID, &item.SoftwareProfileID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkSoftwareInstallRetry(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('software_install_retry_at', to_jsonb(now()::text)),
		    updated_at = now()
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

// CompleteSoftwareInstallMeta merges bundle meta and clears install error markers.
func (s *Store) CompleteSoftwareInstallMeta(ctx context.Context, instanceID string, meta map[string]any) error {
	b, _ := json.Marshal(meta)
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = (COALESCE(provider_meta, '{}'::jsonb) || $2::jsonb)
		    - 'software_install_error'
		    - 'software_install_retry_at',
		    updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, b)
	return err
}
