package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var allowedRegionTiers = map[string]struct{}{
	"prosto": {}, "midrange": {}, "hustle": {}, "custom": {},
}

type RegionTierRow struct {
	Region  string
	Tier    string
	Enabled bool
}

func normalizeRegionTier(region, tier string) (string, string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	tier = strings.ToLower(strings.TrimSpace(tier))
	if region == "" || tier == "" {
		return "", "", fmt.Errorf("region and tier required")
	}
	if _, ok := allowedRegionTiers[tier]; !ok {
		return "", "", fmt.Errorf("unknown tier: %s", tier)
	}
	return region, tier, nil
}

func (s *Store) ListRegionTiers(ctx context.Context) ([]RegionTierRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT rt.region, rt.tier, rt.enabled
		FROM vps.region_tiers rt
		JOIN vps.regions r ON r.code = rt.region
		ORDER BY r.sort_order ASC, r.code ASC, rt.tier ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RegionTierRow, 0)
	for rows.Next() {
		var row RegionTierRow
		if err := rows.Scan(&row.Region, &row.Tier, &row.Enabled); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListEnabledRegionTiers(ctx context.Context) (map[string]map[string]struct{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT region, tier
		FROM vps.region_tiers
		WHERE enabled = true
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

func (s *Store) RegionTierEnabled(ctx context.Context, region, tier string) (bool, error) {
	region, tier, err := normalizeRegionTier(region, tier)
	if err != nil {
		return false, err
	}
	var ok bool
	err = s.pool.QueryRow(ctx, `
		SELECT enabled
		FROM vps.region_tiers
		WHERE region = $1 AND tier = $2
	`, region, tier).Scan(&ok)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return ok, err
}

func (s *Store) SetRegionTierEnabled(ctx context.Context, region, tier string, enabled bool) error {
	region, tier, err := normalizeRegionTier(region, tier)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.region_tiers
		SET enabled = $3, updated_at = now()
		WHERE region = $1 AND tier = $2
	`, region, tier, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
