package platformmigrate

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Apply runs embedded SQL migrations once per service, tracked in platform.schema_migrations.
func Apply(ctx context.Context, pool *pgxpool.Pool, service string, migrations fs.FS) error {
	if pool == nil || migrations == nil {
		return nil
	}
	service = strings.TrimSpace(service)
	if service == "" {
		return fmt.Errorf("platformmigrate: service name required")
	}
	if _, err := pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS platform;
		CREATE TABLE IF NOT EXISTS platform.schema_migrations (
			service text NOT NULL,
			filename text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (service, filename)
		);
	`); err != nil {
		return fmt.Errorf("platformmigrate init: %w", err)
	}

	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var applied bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM platform.schema_migrations
				WHERE service = $1 AND filename = $2
			)
		`, service, e.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		b, err := fs.ReadFile(migrations, e.Name())
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("migration %s/%s: %w", service, e.Name(), err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO platform.schema_migrations (service, filename)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, service, e.Name()); err != nil {
			return fmt.Errorf("migration record %s/%s: %w", service, e.Name(), err)
		}
	}
	return nil
}
