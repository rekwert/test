-- CRM workflow: answered status, slots, shifts, response tracking

ALTER TABLE support.tickets DROP CONSTRAINT IF EXISTS tickets_status_check;
ALTER TABLE support.tickets ADD CONSTRAINT tickets_status_check
  CHECK (status IN ('open', 'in_progress', 'waiting_client', 'answered', 'return_pending', 'resolved', 'closed'));

ALTER TABLE support.tickets
  ADD COLUMN IF NOT EXISTS last_message_by_staff BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS answered_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS auto_close_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS is_return BOOLEAN NOT NULL DEFAULT false;

UPDATE support.tickets
SET status = 'answered', last_message_by_staff = true
WHERE status = 'waiting_client'
  AND EXISTS (
    SELECT 1 FROM support.ticket_messages m
    WHERE m.ticket_id = support.tickets.id AND m.is_staff = true
  );

UPDATE support.tickets
SET status = 'open', last_message_by_staff = false
WHERE status = 'waiting_client';

UPDATE support.tickets t
SET last_message_by_staff = sub.is_staff
FROM (
  SELECT DISTINCT ON (ticket_id) ticket_id, is_staff
  FROM support.ticket_messages
  ORDER BY ticket_id, created_at DESC
) sub
WHERE t.id = sub.ticket_id;

CREATE TABLE IF NOT EXISTS support.staff_shifts (
  staff_id UUID PRIMARY KEY,
  on_duty BOOLEAN NOT NULL DEFAULT false,
  started_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS support.slot_config (
  id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  max_new_slots INT NOT NULL DEFAULT 2,
  max_return_slots INT NOT NULL DEFAULT 2,
  rebind_hours INT NOT NULL DEFAULT 2,
  auto_close_hours INT NOT NULL DEFAULT 12
);

INSERT INTO support.slot_config (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_tickets_answered ON support.tickets(auto_close_at)
  WHERE status = 'answered';
CREATE INDEX IF NOT EXISTS idx_tickets_return_pending ON support.tickets(assignee_id, last_message_at)
  WHERE status = 'return_pending';
CREATE INDEX IF NOT EXISTS idx_tickets_response ON support.tickets(last_message_by_staff, status);
