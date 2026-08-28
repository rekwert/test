package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidStaffRole          = errors.New("invalid staff role")
	ErrOnlyOwnerCanManageRoles   = errors.New("only owner can manage staff roles")
	ErrCannotAssignOwner         = errors.New("only owner can assign owner role")
	ErrCannotAssignAdmin         = errors.New("only owner/admin can assign admin role")
	ErrCannotModifyOwner         = errors.New("cannot modify owner roles")
)

var staffRoleNames = map[string]bool{
	"support": true,
	"admin":   true,
	"owner":   true,
}

func (s *Store) SetUserStaffRoles(ctx context.Context, actorRoles []string, userID string, roles []string) ([]string, error) {
	if !hasRole(actorRoles, "owner") {
		return nil, ErrOnlyOwnerCanManageRoles
	}

	targetRoles, err := s.getUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, r := range targetRoles {
		if r == "owner" {
			return nil, ErrCannotModifyOwner
		}
	}

	isOwner := hasRole(actorRoles, "owner")

	staffSet := map[string]bool{}
	for _, r := range roles {
		r = normalizeRole(r)
		if r == "client" || r == "" {
			continue
		}
		if !staffRoleNames[r] {
			return nil, ErrInvalidStaffRole
		}
		if r == "owner" && !isOwner {
			return nil, ErrCannotAssignOwner
		}
		staffSet[r] = true
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM auth.user_roles ur
		USING auth.roles r
		WHERE ur.role_id = r.id
		  AND ur.user_id = $1::uuid
		  AND r.name IN ('owner', 'admin', 'support')
	`, userID); err != nil {
		return nil, err
	}

	for role := range staffSet {
		var roleID int
		if err := tx.QueryRow(ctx, `SELECT id FROM auth.roles WHERE name = $1`, role).Scan(&roleID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth.user_roles (user_id, role_id)
			VALUES ($1::uuid, $2)
			ON CONFLICT DO NOTHING
		`, userID, roleID); err != nil {
			return nil, err
		}
	}

	var clientRoleID int
	if err := tx.QueryRow(ctx, `SELECT id FROM auth.roles WHERE name = 'client'`).Scan(&clientRoleID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth.user_roles (user_id, role_id)
		VALUES ($1::uuid, $2)
		ON CONFLICT DO NOTHING
	`, userID, clientRoleID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.getUserRoles(ctx, userID)
}

func (s *Store) GetUserAuthProfile(ctx context.Context, userID string) (email string, roles []string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT email FROM auth.users WHERE id = $1::uuid`, userID).Scan(&email)
	if err == pgx.ErrNoRows {
		return "", nil, err
	}
	if err != nil {
		return "", nil, err
	}
	roles, err = s.getUserRoles(ctx, userID)
	return email, roles, err
}

func hasRole(roles []string, name string) bool {
	for _, r := range roles {
		if r == name {
			return true
		}
	}
	return false
}

func normalizeRole(role string) string {
	switch role {
	case "owner", "admin", "support", "client":
		return role
	default:
		return ""
	}
}
