-- Optional: backfill email history for users already anonymized (deleted_*@removed.local).
-- Uses support.tickets.client_email where available. Run once after migration 014.
INSERT INTO auth.user_email_history (user_id, email, reason, actor_id)
SELECT DISTINCT ON (u.id) u.id, t.client_email, 'backfill_deleted', NULL
FROM auth.users u
JOIN support.tickets t ON t.user_id = u.id
WHERE u.deleted_at IS NOT NULL
  AND u.email LIKE 'deleted_%@removed.local'
  AND t.client_email NOT LIKE 'deleted_%'
  AND NOT EXISTS (
    SELECT 1 FROM auth.user_email_history h WHERE h.user_id = u.id
  )
ORDER BY u.id, t.created_at DESC;
