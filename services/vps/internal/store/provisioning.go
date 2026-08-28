package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CreatingInstance struct {
	ID                string
	ExternalID        string
	UserID            string
	Hostname          string
	RootPassword      string
	NodeID            string
	PlanID            string
	Region            string
	OSTemplateID      string
	SoftwareProfileID string
	UpdatedAt         time.Time
}

func (s *Store) SetInstanceExternalID(ctx context.Context, instanceID, externalID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET external_id = $2, updated_at = now()
		WHERE id = $1::uuid AND state = 'creating'
	`, instanceID, externalID)
	return err
}

func (s *Store) SetInstanceIP(ctx context.Context, instanceID, ip string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET ip_address = NULLIF($2, '')::inet,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND state IN ('creating', 'reinstalling', 'running')
		  AND ip_address IS DISTINCT FROM NULLIF($2, '')::inet
		  AND (
		    NULLIF($2, '') IS NULL
		    OR NOT EXISTS (
		      SELECT 1 FROM vps.instances o
		      WHERE o.id <> $1::uuid
		        AND o.state NOT IN ('deleted', 'error')
		        AND o.ip_address IS NOT NULL
		        AND o.ip_address = NULLIF($2, '')::inet
		    )
		  )
	`, instanceID, ip)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 && strings.TrimSpace(ip) != "" {
		var taken bool
		_ = s.pool.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM vps.instances o
			  WHERE o.id <> $1::uuid
			    AND o.state NOT IN ('deleted', 'error')
			    AND o.ip_address = NULLIF($2, '')::inet
			)
		`, instanceID, ip).Scan(&taken)
		if taken {
			return fmt.Errorf("ip %s already assigned to another instance", ip)
		}
	}
	return nil
}

// GuestAgentWarmupAt returns when guest-agent warmup started for the current create/reinstall cycle.
// On first call while creating or reinstalling, the timestamp is persisted in provider_meta.
func (s *Store) GuestAgentWarmupAt(ctx context.Context, instanceID string) (time.Time, error) {
	var raw *string
	err := s.pool.QueryRow(ctx, `
		SELECT provider_meta->>'guest_agent_warmup_at'
		FROM vps.instances
		WHERE id = $1::uuid
	`, instanceID).Scan(&raw)
	if err != nil {
		return time.Time{}, err
	}
	if raw != nil && strings.TrimSpace(*raw) != "" {
		if t, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(*raw)); parseErr == nil {
			return t, nil
		}
		if t, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(*raw)); parseErr == nil {
			return t, nil
		}
	}
	var started time.Time
	err = s.pool.QueryRow(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('guest_agent_warmup_at', to_jsonb(now())),
		    updated_at = updated_at
		WHERE id = $1::uuid AND state IN ('creating', 'reinstalling')
		RETURNING (provider_meta->>'guest_agent_warmup_at')::timestamptz
	`, instanceID).Scan(&started)
	if err != nil {
		return time.Time{}, err
	}
	return started, nil
}

func (s *Store) ClearGuestAgentWarmupMeta(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'guest_agent_warmup_at'
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

func (s *Store) LastVFPasswordResetAt(ctx context.Context, instanceID string) (time.Time, bool, error) {
	var raw *string
	err := s.pool.QueryRow(ctx, `
		SELECT provider_meta->>'vf_password_reset_at'
		FROM vps.instances
		WHERE id = $1::uuid
	`, instanceID).Scan(&raw)
	if err != nil {
		return time.Time{}, false, err
	}
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*raw)); err == nil {
		return t, true, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func (s *Store) VFPasswordResetAt(ctx context.Context, instanceID string) (time.Time, error) {
	if t, ok, err := s.LastVFPasswordResetAt(ctx, instanceID); err != nil {
		return time.Time{}, err
	} else if ok {
		return t, nil
	}
	var started time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('vf_password_reset_at', to_jsonb(now())),
		    updated_at = updated_at
		WHERE id = $1::uuid AND state IN ('creating', 'reinstalling', 'running')
		RETURNING (provider_meta->>'vf_password_reset_at')::timestamptz
	`, instanceID).Scan(&started)
	if err != nil {
		return time.Time{}, err
	}
	return started, nil
}

func (s *Store) ClearVFPasswordResetMeta(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'vf_password_reset_at'
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

func (s *Store) GetInstancePlanID(ctx context.Context, instanceID string) (string, error) {
	var planID string
	err := s.pool.QueryRow(ctx, `
		SELECT plan_id::text FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&planID)
	return planID, err
}

func (s *Store) ClearInstanceExternalID(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET external_id = NULL, updated_at = now()
		WHERE id = $1::uuid AND state = 'creating'
	`, instanceID)
	return err
}

// ReleaseCreatingPollClaim clears the worker poll lease so the next tick can re-poll VF.
func (s *Store) ReleaseCreatingPollClaim(ctx context.Context, instanceID string) error {
	return s.releaseInstancePollClaim(ctx, instanceID)
}

func (s *Store) releaseInstancePollClaim(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET worker_poll_claimed_at = NULL, worker_poll_claimed_by = NULL
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

const provisionAllocateClaimStaleAfter = "5 minutes"

// TryClaimProvisionAllocate grants one worker exclusive rights to call VirtFusion AllocateServer.
// Prevents duplicate VF servers when outbox handlers and creating poll overlap.
func (s *Store) TryClaimProvisionAllocate(ctx context.Context, instanceID, workerID string) (bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	workerID = strings.TrimSpace(workerID)
	if instanceID == "" {
		return false, nil
	}
	if workerID == "" {
		workerID = "worker"
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('provision_allocate_claim', jsonb_build_object(
		        'at', to_jsonb(now()),
		        'by', to_jsonb($2::text)
		    )),
		    updated_at = updated_at
		WHERE id = $1::uuid AND state = 'creating'
		  AND (external_id IS NULL OR TRIM(external_id) = '')
		  AND (
		    provider_meta->'provision_allocate_claim' IS NULL
		    OR (provider_meta->'provision_allocate_claim'->>'at')::timestamptz
		       < now() - '`+provisionAllocateClaimStaleAfter+`'::interval
		  )
	`, instanceID, workerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ReleaseProvisionAllocateClaim clears a stale or finished allocate lock.
func (s *Store) ReleaseProvisionAllocateClaim(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'provision_allocate_claim'
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

// SetInstanceExternalIDIfEmpty persists VF server id only when still unassigned.
func (s *Store) SetInstanceExternalIDIfEmpty(ctx context.Context, instanceID, externalID string) (won bool, err error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET external_id = $2,
		    updated_at = now(),
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'provision_allocate_claim'
		WHERE id = $1::uuid AND state = 'creating'
		  AND (external_id IS NULL OR TRIM(external_id) = '')
	`, instanceID, externalID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ClaimCreatingForPoll(ctx context.Context, workerID string, limit int) ([]CreatingInstance, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "worker"
	}
	rows, err := s.pool.Query(ctx, `
		WITH picked AS (
			SELECT id FROM vps.instances
			WHERE (state = 'creating' OR (state = 'running' AND ip_address IS NULL))
			  AND COALESCE(provider, 'openstack') NOT IN ('hetzner_robot', 'hostkey')
			  AND (
			    worker_poll_claimed_at IS NULL
			    OR worker_poll_claimed_at < now() - interval '30 seconds'
			  )
			-- Claiming updates updated_at, so old slow builds rotate behind
			-- untouched instances instead of occupying every LIMIT batch.
			ORDER BY updated_at ASC, created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE vps.instances AS i
		SET worker_poll_claimed_at = now(),
		    worker_poll_claimed_by = $2,
		    updated_at = now()
		FROM picked p
		WHERE i.id = p.id
		RETURNING i.id::text,
		          COALESCE(i.external_id, ''),
		          i.user_id::text,
		          COALESCE(i.hostname, ''),
		          COALESCE(i.root_password, ''),
		          COALESCE(i.node_id::text, ''),
		          i.plan_id::text,
		          i.region,
		          i.order_id::text,
		          i.updated_at,
		          COALESCE((SELECT os_template_id FROM vps.orders WHERE id = i.order_id), ''),
		          COALESCE(NULLIF((SELECT software_profile_id FROM vps.orders WHERE id = i.order_id), ''), 'clean')
	`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CreatingInstance
	for rows.Next() {
		var item CreatingInstance
		var orderID string
		if err := rows.Scan(
			&item.ID, &item.ExternalID, &item.UserID, &item.Hostname, &item.RootPassword, &item.NodeID,
			&item.PlanID, &item.Region, &orderID, &item.UpdatedAt,
			&item.OSTemplateID, &item.SoftwareProfileID,
		); err != nil {
			return nil, err
		}
		item.RootPassword, err = s.openSecret(item.RootPassword)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListCreatingForPoll(ctx context.Context, limit int) ([]CreatingInstance, error) {
	return s.ClaimCreatingForPoll(ctx, "worker", limit)
}

type InstanceCredentials struct {
	Hostname     string
	IPAddress    string
	AllIPs       []string
	RootPassword string
	State        string
}

func (s *Store) GetInstanceCredentials(ctx context.Context, userID, instanceID string) (*InstanceCredentials, error) {
	var creds InstanceCredentials
	var meta []byte
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(hostname, ''), COALESCE(host(ip_address)::text, ''), COALESCE(root_password, ''), state,
			COALESCE(provider_meta, '{}'::jsonb)
		FROM vps.instances
		WHERE id = $1::uuid AND user_id = $2::uuid
	`, instanceID, userID).Scan(&creds.Hostname, &creds.IPAddress, &creds.RootPassword, &creds.State, &meta)
	if err != nil {
		return nil, err
	}
	creds.RootPassword, err = s.openSecret(creds.RootPassword)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	_ = json.Unmarshal(meta, &m)
	if m != nil {
		if raw, ok := m["all_ips"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok && s != "" {
					creds.AllIPs = append(creds.AllIPs, s)
				}
			}
		}
	}
	if len(creds.AllIPs) == 0 && creds.IPAddress != "" {
		creds.AllIPs = []string{creds.IPAddress}
	}
	return &creds, nil
}

func (s *Store) IsInstanceCreating(ctx context.Context, instanceID string) (bool, error) {
	var state string
	err := s.pool.QueryRow(ctx, `
		SELECT state FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&state)
	if err != nil {
		return false, err
	}
	return state == "creating", nil
}

func (s *Store) CompleteProvisioningIfCreating(ctx context.Context, instanceID, externalID, ip, nodeID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET external_id = $2,
		    ip_address = NULLIF($3, '')::inet,
		    node_id = NULLIF($4, '')::uuid,
		    state = 'running',
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        - 'guest_agent_warmup_at' - 'provision_error' - 'provision_failed_at',
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
		WHERE id = $1::uuid AND (
			state = 'creating'
			OR (state = 'running' AND ip_address IS NULL)
		)
	`, instanceID, externalID, ip, nodeID)
	if err != nil {
		return false, err
	}
	changed := tag.RowsAffected() > 0
	if changed && strings.TrimSpace(ip) != "" {
		userID, ownerErr := s.GetInstanceOwner(ctx, instanceID)
		if ownerErr == nil {
			_ = s.LogIPAssigned(ctx, userID, instanceID, ip, "", &IPAssignmentLogOpts{Source: IPSourceProvision})
		}
	}
	return changed, nil
}

func (s *Store) GetInstanceOwner(ctx context.Context, instanceID string) (userID string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT user_id::text FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&userID)
	if err == pgx.ErrNoRows {
		return "", err
	}
	return userID, err
}

func (s *Store) ClaimReinstallingForPoll(ctx context.Context, workerID string, limit int) ([]CreatingInstance, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "worker"
	}
	rows, err := s.pool.Query(ctx, `
		WITH picked AS (
			SELECT id FROM vps.instances
			WHERE state = 'reinstalling'
			  AND external_id IS NOT NULL AND TRIM(external_id) <> ''
			  AND COALESCE(provider, 'openstack') NOT IN ('hetzner_robot', 'hostkey')
			  AND (
			    worker_poll_claimed_at IS NULL
			    OR worker_poll_claimed_at < now() - interval '30 seconds'
			  )
			ORDER BY updated_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE vps.instances AS i
		SET worker_poll_claimed_at = now(),
		    worker_poll_claimed_by = $2,
		    updated_at = now()
		FROM picked p
		WHERE i.id = p.id
		RETURNING i.id::text,
		          COALESCE(i.external_id, ''),
		          i.user_id::text,
		          COALESCE(i.hostname, ''),
		          COALESCE(i.root_password, ''),
		          COALESCE(i.node_id::text, ''),
		          i.plan_id::text,
		          i.region,
		          COALESCE((SELECT os_template_id FROM vps.orders WHERE id = i.order_id), ''),
		          COALESCE(NULLIF((SELECT software_profile_id FROM vps.orders WHERE id = i.order_id), ''), 'clean'),
		          i.updated_at
	`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CreatingInstance
	for rows.Next() {
		var item CreatingInstance
		if err := rows.Scan(
			&item.ID, &item.ExternalID, &item.UserID, &item.Hostname, &item.RootPassword, &item.NodeID,
			&item.PlanID, &item.Region, &item.OSTemplateID, &item.SoftwareProfileID, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.RootPassword, err = s.openSecret(item.RootPassword)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListReinstallingForPoll(ctx context.Context, limit int) ([]CreatingInstance, error) {
	return s.ClaimReinstallingForPoll(ctx, "worker", limit)
}

func (s *Store) CompleteReinstall(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'running',
		    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		        - 'guest_agent_warmup_at' - 'reinstall_build_started'
		        - 'provision_error' - 'provision_failed_at',
		    updated_at = now()
		WHERE id = $1::uuid AND state = 'reinstalling'
	`, instanceID)
	return err
}

// TryMarkReinstallBuildStarted ensures ReinstallServer runs at most once per reinstall cycle.
func (s *Store) TryMarkReinstallBuildStarted(ctx context.Context, instanceID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('reinstall_build_started', to_jsonb(now())),
		    updated_at = updated_at
		WHERE id = $1::uuid AND state = 'reinstalling'
		  AND (provider_meta->>'reinstall_build_started') IS NULL
	`, instanceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ClearReinstallBuildStarted(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'reinstall_build_started',
		    updated_at = updated_at
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

func (s *Store) GetInstanceOSTemplateID(ctx context.Context, instanceID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(o.os_template_id, '')
		FROM vps.instances i
		JOIN vps.orders o ON o.id = i.order_id
		WHERE i.id = $1::uuid
	`, instanceID).Scan(&id)
	return id, err
}

func (s *Store) FailProvisioning(ctx context.Context, instanceID string, reason ...string) error {
	_, err := s.FailProvisioningIfCreating(ctx, instanceID, reason...)
	return err
}

// FailProvisioningIfCreating sets state=error only when still creating/reinstalling.
// ok=false means the instance was already finalized (no second refund/alert).
func (s *Store) FailProvisioningIfCreating(ctx context.Context, instanceID string, reason ...string) (ok bool, err error) {
	msg := ""
	if len(reason) > 0 {
		msg = strings.TrimSpace(reason[0])
	}
	if msg != "" {
		tag, err := s.pool.Exec(ctx, `
			UPDATE vps.instances
			SET state = 'error', updated_at = now(),
			    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
			        || jsonb_build_object('provision_error', $2::text, 'provision_failed_at', to_jsonb(now()))
			WHERE id = $1::uuid AND state IN ('creating', 'reinstalling')
		`, instanceID, msg)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'error', updated_at = now()
		WHERE id = $1::uuid AND state IN ('creating', 'reinstalling')
	`, instanceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// GetInstanceState returns current instance state.
func (s *Store) GetInstanceState(ctx context.Context, instanceID string) (string, error) {
	var state string
	err := s.pool.QueryRow(ctx, `SELECT state FROM vps.instances WHERE id = $1::uuid`, instanceID).Scan(&state)
	return state, err
}

// HasDedicatedFailureRefund reports whether a provision-failure refund invoice already exists.
func (s *Store) HasDedicatedFailureRefund(ctx context.Context, instanceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM billing.invoices
			WHERE invoice_type = 'refund'
			  AND status = 'paid'
			  AND description = $1
		)
	`, fmt.Sprintf("Refund dedicated provision failure %s", instanceID)).Scan(&exists)
	return exists, err
}

// HasVPSFailureRefund reports whether a VPS provision-failure refund invoice already exists.
func (s *Store) HasVPSFailureRefund(ctx context.Context, instanceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM billing.invoices
			WHERE invoice_type = 'refund'
			  AND status = 'paid'
			  AND description = $1
		)
	`, fmt.Sprintf("Refund VPS provision failure %s", instanceID)).Scan(&exists)
	return exists, err
}

// HasVPSOrphanRefund reports whether a hypervisor-orphan refund invoice already exists.
func (s *Store) HasVPSOrphanRefund(ctx context.Context, instanceID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM billing.invoices
			WHERE invoice_type = 'refund'
			  AND status = 'paid'
			  AND description = $1
		)
	`, fmt.Sprintf("Refund VPS hypervisor orphan %s", instanceID)).Scan(&exists)
	return exists, err
}

// HasVPSInstanceRefund reports whether any VPS provision/orphan refund was already issued.
func (s *Store) HasVPSInstanceRefund(ctx context.Context, instanceID string) (bool, error) {
	provisionFail, err := s.HasVPSFailureRefund(ctx, instanceID)
	if err != nil || provisionFail {
		return provisionFail, err
	}
	return s.HasVPSOrphanRefund(ctx, instanceID)
}

func (s *Store) GetInstanceProvisionSSHKeys(ctx context.Context, instanceID string) ([]string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(provision_ssh_keys, '[]'::jsonb)
		FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			out = append(out, k)
		}
	}
	return out, nil
}
