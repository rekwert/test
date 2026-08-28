package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ReferrerPercent  = 10
	ReferralCurrency = "RUB"
)

type ReferralEvent struct {
	ID           string
	MaskedEmail  string
	Status       string
	Earned       float64
	CreatedAt    time.Time
}

type ReferralStats struct {
	TotalEarned      float64
	PendingEarned    float64
	EarnedThisMonth  float64
	ActiveReferrals  int
	PendingReferrals int
	LinkClicks       int
}

type ReferralDashboard struct {
	Code   string
	Link   string
	Stats  ReferralStats
	Events []ReferralEvent
}

type AdminReferralRow struct {
	ID           string
	ReferredID   string
	Email        string
	Status       string
	TotalEarned  float64
	CreatedAt    time.Time
}

type ReferrerSummary struct {
	UserID string
	Email  string
}

var (
	ErrReferralAlreadyAssigned = errors.New("referral already assigned")
	ErrReferralSelfAssign      = errors.New("cannot refer yourself")
	ErrReferralUserNotFound    = errors.New("referred user not found")
)

func generateReferralCode(userID string, attempt int) string {
	clean := strings.ReplaceAll(userID, "-", "")
	if len(clean) < 6 {
		return "CH" + strings.ToUpper(clean)
	}
	start := attempt * 2
	if start+6 > len(clean) {
		start = 0
	}
	return "CH" + strings.ToUpper(clean[start:start+6])
}

func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	return parts[0][:1] + "***@" + parts[1]
}

func (s *Store) EnsureReferralCode(ctx context.Context, userID string) (string, error) {
	var code string
	err := s.pool.QueryRow(ctx, `SELECT code FROM referral.codes WHERE user_id = $1`, userID).Scan(&code)
	if err == nil {
		return code, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	for attempt := 0; attempt < 8; attempt++ {
		code = generateReferralCode(userID, attempt)
		tag, err := s.pool.Exec(ctx, `
			INSERT INTO referral.codes (user_id, code)
			VALUES ($1, $2)
			ON CONFLICT (code) DO NOTHING
		`, userID, code)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() > 0 {
			return code, nil
		}
		err = s.pool.QueryRow(ctx, `SELECT code FROM referral.codes WHERE user_id = $1`, userID).Scan(&code)
		if err == nil {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate referral code")
}

func (s *Store) GetUserIDByReferralCode(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(strings.ToUpper(code))
	var userID string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id::text FROM referral.codes WHERE UPPER(code) = $1
	`, code).Scan(&userID)
	if err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) CreateReferralRegistration(ctx context.Context, referrerUserID, referredUserID string) error {
	if referrerUserID == referredUserID {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO referral.registrations (referrer_user_id, referred_user_id)
		VALUES ($1, $2)
		ON CONFLICT (referred_user_id) DO NOTHING
	`, referrerUserID, referredUserID)
	return err
}

func (s *Store) RecordReferralClick(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral.codes WHERE UPPER(code) = UPPER($1))`, code).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO referral.link_clicks (code) VALUES ($1)`, strings.ToUpper(code))
	return err
}

func (s *Store) GetReferralDashboard(ctx context.Context, userID, baseURL string) (*ReferralDashboard, error) {
	code, err := s.EnsureReferralCode(ctx, userID)
	if err != nil {
		return nil, err
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://cloud-hustle.com"
	}

	var stats ReferralStats
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT SUM(amount)::float8 FROM referral.earnings
			WHERE referrer_user_id = $1 AND status = 'credited'
		), 0),
		COALESCE((
			SELECT SUM(amount)::float8 FROM referral.earnings
			WHERE referrer_user_id = $1 AND status = 'pending'
		), 0),
		COALESCE((
			SELECT SUM(amount)::float8 FROM referral.earnings
			WHERE referrer_user_id = $1
			  AND status = 'credited'
			  AND COALESCE(credited_at, created_at) >= date_trunc('month', now())
		), 0),
		COALESCE((
			SELECT COUNT(*)::int FROM referral.registrations
			WHERE referrer_user_id = $1 AND status IN ('paid', 'earning')
		), 0),
		COALESCE((
			SELECT COUNT(*)::int FROM referral.registrations
			WHERE referrer_user_id = $1 AND status = 'registered'
		), 0),
		COALESCE((
			SELECT COUNT(*)::int FROM referral.link_clicks WHERE UPPER(code) = UPPER($2)
		), 0)
	`, userID, code).Scan(
		&stats.TotalEarned,
		&stats.PendingEarned,
		&stats.EarnedThisMonth,
		&stats.ActiveReferrals,
		&stats.PendingReferrals,
		&stats.LinkClicks,
	)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, u.email, r.status, r.total_earned::float8, r.created_at
		FROM referral.registrations r
		JOIN auth.users u ON u.id = r.referred_user_id
		WHERE r.referrer_user_id = $1
		ORDER BY r.created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]ReferralEvent, 0)
	for rows.Next() {
		var ev ReferralEvent
		var email string
		if err := rows.Scan(&ev.ID, &email, &ev.Status, &ev.Earned, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.MaskedEmail = MaskEmail(email)
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ReferralDashboard{
		Code:   code,
		Link:   fmt.Sprintf("%s/register?ref=%s", baseURL, code),
		Stats:  stats,
		Events: events,
	}, nil
}

func (s *Store) GetReferrerForUser(ctx context.Context, referredUserID string) (*ReferrerSummary, error) {
	var ref ReferrerSummary
	err := s.pool.QueryRow(ctx, `
		SELECT r.referrer_user_id::text, u.email
		FROM referral.registrations r
		JOIN auth.users u ON u.id = r.referrer_user_id
		WHERE r.referred_user_id = $1::uuid
	`, referredUserID).Scan(&ref.UserID, &ref.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &ref, nil
}

func (s *Store) ListReferralsByReferrer(ctx context.Context, referrerUserID string) ([]AdminReferralRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.referred_user_id::text, u.email, r.status,
			r.total_earned::float8, r.created_at
		FROM referral.registrations r
		JOIN auth.users u ON u.id = r.referred_user_id
		WHERE r.referrer_user_id = $1::uuid
		ORDER BY r.created_at DESC
		LIMIT 200
	`, referrerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AdminReferralRow, 0)
	for rows.Next() {
		var row AdminReferralRow
		if err := rows.Scan(&row.ID, &row.ReferredID, &row.Email, &row.Status, &row.TotalEarned, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) AdminAssignReferralByEmail(ctx context.Context, referrerUserID, referredEmail string) (*AdminReferralRow, error) {
	referredEmail = strings.TrimSpace(strings.ToLower(referredEmail))
	if referredEmail == "" {
		return nil, fmt.Errorf("email required")
	}
	referred, err := s.GetUserByEmail(ctx, referredEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReferralUserNotFound
		}
		return nil, err
	}
	if referrerUserID == referred.ID {
		return nil, ErrReferralSelfAssign
	}

	var exists bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM referral.registrations WHERE referred_user_id = $1::uuid)
	`, referred.ID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrReferralAlreadyAssigned
	}

	if _, err := s.EnsureReferralCode(ctx, referrerUserID); err != nil {
		return nil, err
	}
	if err := s.CreateReferralRegistration(ctx, referrerUserID, referred.ID); err != nil {
		return nil, err
	}

	rows, err := s.ListReferralsByReferrer(ctx, referrerUserID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ReferredID == referred.ID {
			return &row, nil
		}
	}
	return nil, fmt.Errorf("referral created but not found")
}
