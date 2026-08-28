CREATE SCHEMA IF NOT EXISTS support;

CREATE TABLE IF NOT EXISTS support.tickets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL,
  client_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  priority TEXT NOT NULL DEFAULT 'normal',
  assignee_id UUID,
  sla_due_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_message_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT tickets_status_check CHECK (status IN ('open', 'in_progress', 'waiting_client', 'resolved', 'closed')),
  CONSTRAINT tickets_priority_check CHECK (priority IN ('low', 'normal', 'high', 'urgent'))
);

CREATE TABLE IF NOT EXISTS support.ticket_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id UUID NOT NULL REFERENCES support.tickets(id) ON DELETE CASCADE,
  author_id UUID NOT NULL,
  author_email TEXT NOT NULL,
  is_staff BOOLEAN NOT NULL DEFAULT false,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tickets_user ON support.tickets(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_queue ON support.tickets(priority, sla_due_at, created_at)
  WHERE status = 'open' AND assignee_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_tickets_assignee ON support.tickets(assignee_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket ON support.ticket_messages(ticket_id, created_at ASC);
