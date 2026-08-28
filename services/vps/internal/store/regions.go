package store

import (
	"context"
)

type RegionRow struct {
	Code      string
	NameEN    string
	NameRU    string
	CityEN    string
	CityRU    string
	Enabled   bool
	Available bool
	SortOrder int
	ProbeHost string
}

// ListRegions returns catalog regions. Available is true when the region is
// enabled and has at least one online provisionable node (orders may waitlist
// when capacity is full).
func (s *Store) ListRegions(ctx context.Context) ([]RegionRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			r.code,
			r.name_en,
			r.name_ru,
			r.city_en,
			r.city_ru,
			r.enabled,
			r.sort_order,
			COALESCE((
				SELECT NULLIF(TRIM(n.vf_ip), '')
				FROM vps.nodes n
				WHERE n.region = r.code
				  AND n.status = 'online'
				  AND COALESCE(n.vf_enabled, true) = true
				  AND COALESCE(n.maintenance_mode, false) = false
				ORDER BY n.name ASC
				LIMIT 1
			), '') AS probe_host,
			EXISTS (
				SELECT 1
				FROM vps.nodes n
				WHERE n.region = r.code
				  AND n.status = 'online'
				  AND n.external_id IS NOT NULL AND n.external_id <> ''
				  AND COALESCE(n.vf_enabled, true) = true
				  AND COALESCE(n.maintenance_mode, false) = false
			) AS has_node
		FROM vps.regions r
		ORDER BY r.sort_order ASC, r.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RegionRow, 0)
	for rows.Next() {
		var r RegionRow
		var hasNode bool
		if err := rows.Scan(
			&r.Code, &r.NameEN, &r.NameRU, &r.CityEN, &r.CityRU,
			&r.Enabled, &r.SortOrder, &r.ProbeHost, &hasNode,
		); err != nil {
			return nil, err
		}
		r.Available = r.Enabled && hasNode
		out = append(out, r)
	}
	return out, rows.Err()
}
