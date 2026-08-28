ALTER TABLE support.tickets ADD COLUMN IF NOT EXISTS instance_id UUID;
ALTER TABLE support.tickets ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';
