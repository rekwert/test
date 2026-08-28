CREATE TABLE IF NOT EXISTS support.message_attachments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id UUID NOT NULL REFERENCES support.tickets(id) ON DELETE CASCADE,
  message_id UUID REFERENCES support.ticket_messages(id) ON DELETE CASCADE,
  uploader_id UUID NOT NULL,
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  storage_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_message_attachments_ticket ON support.message_attachments(ticket_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_attachments_message ON support.message_attachments(message_id);
