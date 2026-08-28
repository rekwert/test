package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// activeInstancesOnNode counts instances that consume node capacity.
// queued waitlist rows do not occupy a slot until promoted to creating.
const activeInstancesOnNode = `
	SELECT COUNT(*)::int
	FROM vps.instances i
	WHERE i.node_id = n.id
	  AND i.state IN ('creating', 'running', 'reinstalling', 'stopped', 'suspended')
`

func (s *Store) PickNodeForRegion(ctx context.Context, region, tier string) (string, error) {
	tier = strings.ToLower(strings.TrimSpace(tier))
	var nodeID string
	err := s.pool.QueryRow(ctx, `
		SELECT n.id::text
		FROM vps.nodes n
		WHERE n.region = $1
		  AND n.status = 'online'
		  AND n.external_id IS NOT NULL AND n.external_id <> ''
		  AND COALESCE(n.vf_enabled, true) = true
		  AND COALESCE(n.maintenance_mode, false) = false
		  AND ($2 = '' OR $2 = ANY(n.supported_tiers))
		  AND (`+activeInstancesOnNode+`) < n.capacity_instances
		ORDER BY (`+activeInstancesOnNode+`) ASC, n.name ASC
		LIMIT 1
	`, region, tier).Scan(&nodeID)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("%w: %s", ErrNoNodeForRegion, region)
	}
	return nodeID, err
}

// RegionHasOnlineNode reports whether the region sells the product tier on any online
// node (admin supported_tiers). Capacity and dedicated hypervisor hardware are separate:
// midrange/hustle orders queue when the tier is enabled; PromoteWaitlisted picks a
// midrange-only / hustle-only node when one exists.
func (s *Store) RegionHasOnlineNode(ctx context.Context, region, tier string) (bool, error) {
	tier = strings.ToLower(strings.TrimSpace(tier))
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM vps.nodes n
			WHERE n.region = $1
			  AND n.status = 'online'
			  AND n.external_id IS NOT NULL AND n.external_id <> ''
			  AND COALESCE(n.vf_enabled, true) = true
			  AND COALESCE(n.maintenance_mode, false) = false
			  AND ($2 = '' OR $2 = ANY(n.supported_tiers))
		)
	`, region, tier).Scan(&ok)
	return ok, err
}

// ListOnlineRegionTiers returns region → set of sellable product tiers from online nodes.
func (s *Store) ListOnlineRegionTiers(ctx context.Context) (map[string]map[string]struct{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.region, lower(t.tier)
		FROM vps.nodes n
		CROSS JOIN LATERAL unnest(n.supported_tiers) AS t(tier)
		WHERE n.status = 'online'
		  AND n.external_id IS NOT NULL AND n.external_id <> ''
		  AND COALESCE(n.vf_enabled, true) = true
		  AND COALESCE(n.maintenance_mode, false) = false
		  AND trim(t.tier) <> ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]map[string]struct{}{}
	for rows.Next() {
		var region, tier string
		if err := rows.Scan(&region, &tier); err != nil {
			return nil, err
		}
		region = strings.ToLower(strings.TrimSpace(region))
		tier = strings.ToLower(strings.TrimSpace(tier))
		if region == "" || tier == "" {
			continue
		}
		if out[region] == nil {
			out[region] = map[string]struct{}{}
		}
		out[region][tier] = struct{}{}
	}
	return out, rows.Err()
}

var allowedNodeTiers = map[string]struct{}{
	"trial": {}, "prosto": {}, "midrange": {}, "hustle": {}, "custom": {},
}

// UpdateNodeSupportedTiers sets which product lines a node may host.
func (s *Store) UpdateNodeSupportedTiers(ctx context.Context, nodeID string, tiers []string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("node id required")
	}
	clean := make([]string, 0, len(tiers))
	seen := map[string]struct{}{}
	unknown := 0
	for _, t := range tiers {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := allowedNodeTiers[t]; !ok {
			unknown++
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		clean = append(clean, t)
	}
	// Avoid wiping sellable lines when the UI sends an unknown tier (e.g. trial before allowlist).
	if len(clean) == 0 && (len(tiers) > 0 || unknown > 0) {
		return fmt.Errorf("no valid supported tiers in request")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.nodes SET supported_tiers = $2::text[]
		WHERE id = $1::uuid
	`, nodeID, clean)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ResolvePlanTier returns the catalog tier for a plan id (from DB, with name fallback).
func (s *Store) ResolvePlanTier(ctx context.Context, planID string) (string, error) {
	var tier string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(
			NULLIF(lower(trim(tier)), ''),
			CASE
				WHEN lower(name) LIKE 'trial%' THEN 'trial'
				WHEN lower(name) LIKE 'prosto%' THEN 'prosto'
				WHEN lower(name) LIKE 'midrange%' THEN 'midrange'
				WHEN lower(name) LIKE 'hustle%' THEN 'hustle'
				WHEN lower(name) LIKE 'custom%' THEN 'custom'
				WHEN lower(name) LIKE 'storage%' THEN 'custom'
				ELSE ''
			END
		)
		FROM vps.plans
		WHERE id = $1::uuid
	`, planID).Scan(&tier)
	if err == pgx.ErrNoRows {
		return "", ErrPlanNotFound
	}
	return tier, err
}

func (s *Store) GetNodeExternalID(ctx context.Context, nodeID string) (string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return "", nil
	}
	var externalID string
	err := s.pool.QueryRow(ctx, `
		SELECT external_id FROM vps.nodes WHERE id = $1::uuid
	`, nodeID).Scan(&externalID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(externalID), nil
}

func (s *Store) GetNodeComputeResourceID(ctx context.Context, nodeID string) (int, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0, nil
	}
	var externalID string
	err := s.pool.QueryRow(ctx, `
		SELECT external_id FROM vps.nodes WHERE id = $1::uuid
	`, nodeID).Scan(&externalID)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(externalID)
	if err != nil || id <= 0 {
		return 0, nil
	}
	return id, nil
}

func (s *Store) ListSSHPublicKeys(ctx context.Context, userID string, keyIDs []string) ([]string, error) {
	if len(keyIDs) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT public_key FROM auth.ssh_keys
		WHERE user_id = $1::uuid AND id = ANY($2::uuid[])
	`, userID, keyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ListAllSSHPublicKeys returns every public key registered for the user.
// Used when retrying a build after the outbox payload is gone.
func (s *Store) ListAllSSHPublicKeys(ctx context.Context, userID string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT public_key FROM auth.ssh_keys
		WHERE user_id = $1::uuid
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
