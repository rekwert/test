package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrUserDeleted = errors.New("user already deleted")
var ErrCannotDeleteStaff = errors.New("cannot delete staff user")

func (s *Store) SoftDeleteUser(ctx context.Context, userID, actorID string) (deletedEmail string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var email string
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT email, deleted_at FROM auth.users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&email, &deletedAt); err != nil {
		if err == pgx.ErrNoRows {
			return "", pgx.ErrNoRows
		}
		return "", err
	}
	if deletedAt != nil {
		return "", ErrUserDeleted
	}

	roles, err := s.getUserRolesTx(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	for _, role := range roles {
		switch role {
		case "owner", "admin", "support":
			return "", ErrCannotDeleteStaff
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO auth.user_email_history (user_id, email, reason, actor_id)
		VALUES ($1, $2, 'account_deleted', NULLIF($3, '')::uuid)
	`, userID, email, actorID); err != nil {
		return "", err
	}

	anonEmail := fmt.Sprintf("deleted_%s@removed.local", strings.ReplaceAll(userID, "-", ""))
	if _, err := tx.Exec(ctx, `
		UPDATE auth.users
		SET deleted_at = now(),
		    email = $2,
		    updated_at = now()
		WHERE id = $1
	`, userID, anonEmail); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID); err != nil {
		return "", err
	}

	// Suspend billing and active services for deleted account.
	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts
		SET billing_status = 'suspended', updated_at = now()
		WHERE user_id = $1
	`, userID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET auto_renew = false,
		    billing_status = 'suspended',
		    updated_at = now()
		WHERE user_id = $1
		  AND state <> 'deleted'
	`, userID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		SELECT 'instance.stop_requested',
		       jsonb_build_object(
		           'instance_id', i.id::text,
		           'external_id', COALESCE(i.external_id, ''),
		           'user_id', i.user_id::text,
		           'reason', 'user_deleted'
		       )
		FROM vps.instances i
		WHERE i.user_id = $1
		  AND i.state IN ('running', 'stopped')
		  AND COALESCE(i.provider, 'openstack') = 'openstack'
		  AND COALESCE(i.external_id, '') <> ''
	`, userID); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return email, nil
}

func (s *Store) getUserRolesTx(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.name
		FROM auth.user_roles ur
		JOIN auth.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}
