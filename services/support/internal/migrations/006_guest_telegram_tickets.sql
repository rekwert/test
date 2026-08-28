-- Guest (pre-registration) tickets from Telegram bot: no user_id yet, replies go to telegram_chat_id.
ALTER TABLE support.tickets
  ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE support.tickets
  ADD COLUMN IF NOT EXISTS telegram_chat_id BIGINT;

ALTER TABLE support.ticket_messages
  ALTER COLUMN author_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_telegram_chat
  ON support.tickets(telegram_chat_id)
  WHERE telegram_chat_id IS NOT NULL AND status <> 'closed';
