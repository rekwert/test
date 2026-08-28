package store

import (
	"context"
)

type ConnectionMeta struct {
	IP         string
	ClientPort *int
	ServerPort *int
}

type LoginAttempt struct {
	Email         string
	UserID        string
	Success       bool
	FailureReason string
	IP            string
	ClientPort    *int
	ServerPort    *int
	UserAgent     string
}

func (s *Store) InsertLoginAttempt(ctx context.Context, a LoginAttempt) error {
	var userIDArg any
	if a.UserID != "" {
		userIDArg = a.UserID
	}
	var ipArg any
	if a.IP != "" {
		ipArg = a.IP
	}
	var reasonArg any
	if a.FailureReason != "" {
		reasonArg = a.FailureReason
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth.login_attempts (
			email, user_id, success, failure_reason, ip, client_port, server_port, user_agent
		) VALUES ($1, $2::uuid, $3, $4, $5::inet, $6, $7, NULLIF($8, ''))
	`, a.Email, userIDArg, a.Success, reasonArg, ipArg, a.ClientPort, a.ServerPort, a.UserAgent)
	return err
}

func (s *Store) InsertUserEmailHistory(ctx context.Context, userID, email, reason, actorID string) error {
	var actorArg any
	if actorID != "" {
		actorArg = actorID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth.user_email_history (user_id, email, reason, actor_id)
		VALUES ($1::uuid, $2, $3, $4::uuid)
	`, userID, email, reason, actorArg)
	return err
}
