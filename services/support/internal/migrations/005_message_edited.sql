ALTER TABLE support.ticket_messages
  ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;
