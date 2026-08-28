package authuser

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type emailVerifier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EmailVerified reads auth.users.email_verified for a live account.
func EmailVerified(ctx context.Context, q emailVerifier, userID string) (bool, error) {
	var verified bool
	err := q.QueryRow(ctx, `
		SELECT email_verified FROM auth.users
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return verified, err
}
