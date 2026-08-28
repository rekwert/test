package store

import (
	"context"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
)

type OSTemplateRow struct {
	ID                string
	Name              string
	Version           string
	Family            string
	Active            bool
	ExternalVersionID *int
}

func (s *Store) ListActiveOSTemplates(ctx context.Context) ([]OSTemplateRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, version, family, active, external_version_id
		FROM vps.os_templates
		WHERE active = true
		ORDER BY name ASC, version ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OSTemplateRow
	for rows.Next() {
		var row OSTemplateRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Version, &row.Family, &row.Active, &row.ExternalVersionID); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) IsOSTemplateActive(ctx context.Context, id string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `
		SELECT active FROM vps.os_templates WHERE id = $1
	`, id).Scan(&active)
	return active, err
}

func (s *Store) SyncOSTemplates(ctx context.Context, matched map[string]int, syncedAt time.Time) error {
	entries := make([]catalog.OSCatalogEntry, 0, len(matched))
	order := 0
	for id, versionID := range matched {
		if versionID <= 0 {
			continue
		}
		order++
		name, version, family := id, "", "debian"
		if t, ok := catalog.CatalogTemplateByID(id); ok {
			name, version, family = t.Name, t.Version, t.Family
		}
		entries = append(entries, catalog.OSCatalogEntry{
			ID: id, Name: name, Version: version, Family: family,
			VersionID: versionID, SortOrder: order,
		})
	}
	return s.SyncOSTemplatesFull(ctx, entries, syncedAt)
}

func (s *Store) SyncOSTemplatesFull(ctx context.Context, entries []catalog.OSCatalogEntry, syncedAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vps.os_templates
		SET active = false, external_version_id = NULL, synced_at = $1
	`, syncedAt); err != nil {
		return err
	}

	for _, e := range entries {
		if e.VersionID <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO vps.os_templates (id, name, version, family, active, sort_order, external_version_id, synced_at)
			VALUES ($1, $2, $3, $4, true, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				version = EXCLUDED.version,
				family = EXCLUDED.family,
				active = true,
				sort_order = EXCLUDED.sort_order,
				external_version_id = EXCLUDED.external_version_id,
				synced_at = EXCLUDED.synced_at
		`, e.ID, e.Name, e.Version, e.Family, e.SortOrder, e.VersionID, syncedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO vps.os_software (os_id, software_id)
			VALUES ($1, 'clean')
			ON CONFLICT DO NOTHING
		`, e.ID); err != nil {
			return err
		}
		// Linux families get optional preinstall profiles in the catalog.
		if e.Family != "windows" && e.Family != "none" {
			for _, sw := range []string{"3x-ui", "marzban", "python3"} {
				if e.Family == "freebsd" && (sw == "3x-ui" || sw == "marzban") {
					continue
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO vps.os_software (os_id, software_id)
					VALUES ($1, $2)
					ON CONFLICT DO NOTHING
				`, e.ID, sw); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) ListOSExternalMap(ctx context.Context) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, external_version_id
		FROM vps.os_templates
		WHERE active = true AND external_version_id IS NOT NULL AND external_version_id > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var versionID int
		if err := rows.Scan(&id, &versionID); err != nil {
			return nil, err
		}
		out[id] = versionID
	}
	return out, rows.Err()
}

func (s *Store) UpdateInstanceRootPassword(ctx context.Context, instanceID, password string) error {
	stored, err := s.sealSecret(password)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE vps.instances SET root_password = $2, updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, stored)
	return err
}
