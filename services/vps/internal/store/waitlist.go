package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

// IPPoolProbe checks VirtFusion for a free primary IPv4 before promoting ip_pool waitlist rows.
type IPPoolProbe func(ctx context.Context, region, hypervisorID string) (bool, error)

// PromoteWaitlisted promotes queued instances onto free capacity (FIFO by created_at).
// Returns how many were moved to creating and received a provision outbox event.
func (s *Store) PromoteWaitlisted(ctx context.Context, limit int, probe IPPoolProbe) (int, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT i.id::text,
		       i.user_id::text,
		       i.order_id::text,
		       i.plan_id::text,
		       i.region,
		       COALESCE(i.hostname, ''),
		       COALESCE(i.root_password, ''),
		       COALESCE(o.os_template_id, ''),
		       COALESCE(o.software_profile_id, ''),
		       COALESCE(i.provision_ssh_keys, '[]'::jsonb),
		       COALESCE(i.provider_meta->>'wait_reason', '')
		FROM vps.instances i
		LEFT JOIN vps.orders o ON o.id = i.order_id
		WHERE i.state = 'queued'
		  AND COALESCE(i.product_type, 'vps') <> 'dedicated'
		  AND COALESCE(i.provider, '') NOT IN ('hetzner_robot', 'hostkey')
		  AND (
		    (provider_meta->>'wait_reason') IS DISTINCT FROM 'ip_pool'
		    OR (provider_meta->>'ip_pool_retry_after')::timestamptz <= now()
		  )
		ORDER BY i.created_at ASC
		FOR UPDATE OF i SKIP LOCKED
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}

	type candidate struct {
		ID                string
		UserID            string
		OrderID           string
		PlanID            string
		Region            string
		Hostname          string
		RootPassword      string
		OSTemplateID      string
		SoftwareProfileID string
		SSHKeysRaw        []byte
		WaitReason        string
	}
	var items []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.OrderID, &c.PlanID, &c.Region,
			&c.Hostname, &c.RootPassword, &c.OSTemplateID, &c.SoftwareProfileID, &c.SSHKeysRaw,
			&c.WaitReason,
		); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	planTiers := map[string]string{}
	if len(items) > 0 {
		planIDs := make([]string, 0, len(items))
		seen := map[string]struct{}{}
		for _, item := range items {
			if item.PlanID == "" {
				continue
			}
			if _, ok := seen[item.PlanID]; ok {
				continue
			}
			seen[item.PlanID] = struct{}{}
			planIDs = append(planIDs, item.PlanID)
		}
		if len(planIDs) > 0 {
			tierRows, qErr := tx.Query(ctx, `
				SELECT id::text,
				       COALESCE(
				           NULLIF(lower(trim(tier)), ''),
				           CASE
				               WHEN lower(name) LIKE 'prosto%' THEN 'prosto'
				               WHEN lower(name) LIKE 'midrange%' THEN 'midrange'
				               WHEN lower(name) LIKE 'hustle%' THEN 'hustle'
				               WHEN lower(name) LIKE 'custom%' THEN 'custom'
				               WHEN lower(name) LIKE 'storage%' THEN 'custom'
				               ELSE ''
				           END
				       )
				FROM vps.plans
				WHERE id = ANY($1::uuid[])
			`, planIDs)
			if qErr != nil {
				return 0, qErr
			}
			for tierRows.Next() {
				var planID, tier string
				if err := tierRows.Scan(&planID, &tier); err != nil {
					tierRows.Close()
					return 0, err
				}
				planTiers[planID] = tier
			}
			tierRows.Close()
			if err := tierRows.Err(); err != nil {
				return 0, err
			}
		}
	}

	promoted := 0
	for _, item := range items {
		var nodeID string
		var nodeExternalID string
		planTier := strings.ToLower(strings.TrimSpace(planTiers[item.PlanID]))
		tierFilter := "AND ($2 = '' OR $2 = ANY(n.supported_tiers))"
		if TierAcceptsCapacityWaitlist(planTier) {
			tierFilter = "AND $2 = ANY(n.supported_tiers) AND NOT ('prosto' = ANY(n.supported_tiers))"
		}

		err := tx.QueryRow(ctx, `
			SELECT n.id::text, COALESCE(n.external_id, '')
			FROM vps.nodes n
			WHERE n.region = $1
			  AND n.status = 'online'
			  AND n.external_id IS NOT NULL AND n.external_id <> ''
			  AND COALESCE(n.vf_enabled, true) = true
			  AND COALESCE(n.maintenance_mode, false) = false
			  `+tierFilter+`
			  AND (`+activeInstancesOnNode+`) < n.capacity_instances
			ORDER BY (`+activeInstancesOnNode+`) ASC, n.name ASC
			LIMIT 1
		`, item.Region, planTier).Scan(&nodeID, &nodeExternalID)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return promoted, err
		}

		if item.WaitReason == "ip_pool" {
			if probe == nil {
				continue
			}
			ok, probeErr := probe(ctx, item.Region, nodeExternalID)
			if probeErr != nil || !ok {
				continue
			}
		}

		tag, err := tx.Exec(ctx, `
			UPDATE vps.instances
			SET state = 'creating',
			    node_id = $2::uuid,
			    updated_at = now(),
			    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
			        - 'wait_reason' - 'ip_pool_retry_after'
			WHERE id = $1::uuid AND state = 'queued'
		`, item.ID, nodeID)
		if err != nil {
			return promoted, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}

		var sshKeys []string
		_ = json.Unmarshal(item.SSHKeysRaw, &sshKeys)

		outboxPayload, _ := json.Marshal(map[string]any{
			"instance_id":         item.ID,
			"order_id":            item.OrderID,
			"user_id":             item.UserID,
			"plan_id":             item.PlanID,
			"region":              item.Region,
			"node_id":             nodeID,
			"hostname":            item.Hostname,
			"os_template_id":      item.OSTemplateID,
			"software_profile_id": item.SoftwareProfileID,
			"ssh_keys":            sshKeys,
		})
		if _, err := insertProvisionOutboxTx(ctx, tx, outboxPayload); err != nil {
			return promoted, err
		}
		promoted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return promoted, nil
}

// RequeueInstanceForIPPool moves a creating instance back to queued when the VF IP pool is empty.
func (s *Store) RequeueInstanceForIPPool(ctx context.Context, instanceID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'queued',
		    external_id = NULL,
		    ip_address = NULL,
		    updated_at = now(),
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        || jsonb_build_object(
		            'wait_reason', 'ip_pool',
		            'ip_pool_retry_after', to_jsonb(now() + interval '15 minutes')
		        )
		        - 'guest_agent_warmup_at' - 'vf_password_reset_at' - 'provision_error' - 'provision_failed_at'
		WHERE id = $1::uuid AND state = 'creating'
	`, instanceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CountQueued returns how many instances are waiting for capacity.
func (s *Store) CountQueued(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM vps.instances WHERE state = 'queued'
	`).Scan(&n)
	return n, err
}
