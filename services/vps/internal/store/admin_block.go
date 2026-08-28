package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) InstanceAdminBlock(ctx context.Context, instanceID string) (bool, error) {
	var blocked bool
	err := s.pool.QueryRow(ctx, `
		SELECT admin_block FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&blocked)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return blocked, err
}

func (s *Store) ApplyAdminBlock(ctx context.Context, instanceID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET admin_block = true,
		    state = 'stopped',
		    updated_at = now()
		WHERE id = $1::uuid
		  AND state <> 'deleted'
	`, instanceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ClearAdminBlock(ctx context.Context, instanceID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET admin_block = false,
		    state = CASE
		        WHEN billing_status = 'active'
		         AND COALESCE(abuse_hold, false) = false
		         AND state = 'stopped' THEN 'starting'
		        ELSE state
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND state <> 'deleted'
	`, instanceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
