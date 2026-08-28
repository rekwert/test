package store

import (
	"context"
	"strings"
	"time"
)

type UserListItem struct {
	ID            string
	Email         string
	DisplayName   string
	EmailVerified bool
	Roles         []string
	BillingStatus string
	CreatedAt     time.Time
	LastActiveAt  *time.Time
}

type UserListFilters struct {
	Query         string
	Role          string
	Status        string
	EmailVerified string // "", "true", "false"
	CreatedFrom   string
	CreatedTo     string
	Limit         int
	Offset        int
}

type UserActivityTimes struct {
	LastActiveAt *time.Time
	LastLoginAt  *time.Time
}

func MaxTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.After(*a) {
		return b
	}
	return a
}

func (t UserActivityTimes) PortalAt() *time.Time {
	return MaxTime(t.LastActiveAt, t.LastLoginAt)
}

func (s *Store) GetUserActivityTimes(ctx context.Context, userID string) (*UserActivityTimes, error) {
	var out UserActivityTimes
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT MAX(rt.last_active_at) FROM auth.refresh_tokens rt WHERE rt.user_id = $1::uuid),
			(SELECT MAX(al.created_at) FROM auth.audit_log al
			 WHERE al.actor_id = $1::uuid AND al.action = 'user.login')
	`, userID).Scan(&out.LastActiveAt, &out.LastLoginAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) GetUserBillingStatus(ctx context.Context, userID string) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(billing_status, 'active') FROM billing.accounts WHERE user_id = $1::uuid
	`, userID).Scan(&status)
	if err != nil {
		if IsNotFound(err) {
			return "active", nil
		}
		return "", err
	}
	return status, nil
}

func (s *Store) ListUsers(ctx context.Context, f UserListFilters) ([]UserListItem, int, error) {
	limit := f.Limit
	offset := f.Offset
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	q := strings.TrimSpace(f.Query)
	role := strings.TrimSpace(strings.ToLower(f.Role))
	status := strings.TrimSpace(strings.ToLower(f.Status))
	emailVerified := strings.TrimSpace(strings.ToLower(f.EmailVerified))
	if !validUserRoleFilter(role) {
		return nil, 0, ErrInvalidRoleFilter
	}
	if !validBillingStatusFilter(status) {
		return nil, 0, ErrInvalidStatusFilter
	}
	if !validEmailVerifiedFilter(emailVerified) {
		return nil, 0, ErrInvalidEmailVerifiedFilter
	}
	pattern := ""
	if q != "" {
		pattern = "%" + escapeLikePattern(q) + "%"
	}
	createdFrom := strings.TrimSpace(f.CreatedFrom)
	createdTo := strings.TrimSpace(f.CreatedTo)

	whereCount := `
		FROM auth.users u
		LEFT JOIN billing.accounts ba ON ba.user_id = u.id
		WHERE ($1 = '' OR u.email ILIKE $2 ESCAPE '\' OR u.display_name ILIKE $2 ESCAPE '\')
		AND ($3 = '' OR EXISTS (
			SELECT 1 FROM auth.user_roles ur
			JOIN auth.roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id AND r.name = $3
		))
		AND ($4 = '' OR COALESCE(ba.billing_status, 'active') = $4)
		AND ($5 = '' OR u.created_at >= $5::date)
		AND ($6 = '' OR u.created_at < ($6::date + interval '1 day'))
		AND ($7 = '' OR u.email_verified = ($7 = 'true'))
		AND u.deleted_at IS NULL
	`

	whereSelect := `
		FROM auth.users u
		LEFT JOIN billing.accounts ba ON ba.user_id = u.id
		WHERE ($1 = '' OR u.email ILIKE $2 ESCAPE '\' OR u.display_name ILIKE $2 ESCAPE '\')
		AND ($5 = '' OR EXISTS (
			SELECT 1 FROM auth.user_roles ur
			JOIN auth.roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id AND r.name = $5
		))
		AND ($6 = '' OR COALESCE(ba.billing_status, 'active') = $6)
		AND ($7 = '' OR u.created_at >= $7::date)
		AND ($8 = '' OR u.created_at < ($8::date + interval '1 day'))
		AND ($9 = '' OR u.email_verified = ($9 = 'true'))
		AND u.deleted_at IS NULL
	`

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int `+whereCount,
		q, pattern, role, status, createdFrom, createdTo, emailVerified,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.email, COALESCE(u.display_name, ''), u.email_verified, u.created_at,
			COALESCE(ba.billing_status, 'active'),
			(SELECT MAX(ts) FROM (
				SELECT MAX(rt.last_active_at) AS ts
				FROM auth.refresh_tokens rt
				WHERE rt.user_id = u.id
				UNION ALL
				SELECT MAX(al.created_at) AS ts
				FROM auth.audit_log al
				WHERE al.actor_id = u.id AND al.action = 'user.login'
			) portal WHERE ts IS NOT NULL)
		`+whereSelect+`
		ORDER BY u.created_at DESC
		LIMIT $3 OFFSET $4
	`, q, pattern, limit, offset, role, status, createdFrom, createdTo, emailVerified)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []UserListItem
	var userIDs []string
	for rows.Next() {
		var item UserListItem
		if err := rows.Scan(
			&item.ID, &item.Email, &item.DisplayName, &item.EmailVerified, &item.CreatedAt, &item.BillingStatus, &item.LastActiveAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
		userIDs = append(userIDs, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rolesByUser, err := s.getUserRolesBatch(ctx, userIDs)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i].Roles = rolesByUser[items[i].ID]
	}
	return items, total, nil
}
