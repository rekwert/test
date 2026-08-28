package store

import "context"

func (s *Store) UpdateNotificationPrefs(ctx context.Context, userID string, notifyEmail, notifyTelegram *bool) error {
	if notifyEmail == nil && notifyTelegram == nil {
		return nil
	}
	if notifyEmail != nil && notifyTelegram != nil {
		_, err := s.pool.Exec(ctx, `
			UPDATE auth.users
			SET notify_email = $2, notify_telegram = $3, updated_at = now()
			WHERE id = $1
		`, userID, *notifyEmail, *notifyTelegram)
		return err
	}
	if notifyEmail != nil {
		_, err := s.pool.Exec(ctx, `
			UPDATE auth.users SET notify_email = $2, updated_at = now() WHERE id = $1
		`, userID, *notifyEmail)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE auth.users SET notify_telegram = $2, updated_at = now() WHERE id = $1
	`, userID, *notifyTelegram)
	return err
}
