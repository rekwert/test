-- Monthly partitioning for vps.outbox (worker poll + retention at scale).

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'vps' AND c.relname = 'outbox_legacy'
    ) THEN
        RETURN;
    END IF;
END $$;

-- Ensure source table has worker poll columns (041 may be marked applied on older DBs).
ALTER TABLE vps.outbox
    ADD COLUMN IF NOT EXISTS worker_poll_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS worker_poll_claimed_by TEXT;

-- Clean up partial run from a failed attempt.
DROP TABLE IF EXISTS vps.outbox_partitioned CASCADE;

CREATE TABLE vps.outbox_partitioned (
    id BIGSERIAL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    published BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    worker_poll_claimed_at TIMESTAMPTZ,
    worker_poll_claimed_by TEXT,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS vps.outbox_default
    PARTITION OF vps.outbox_partitioned DEFAULT;

DO $$
DECLARE
    start_month date := date_trunc('month', now() - interval '1 month')::date;
    end_month date := (date_trunc('month', now()) + interval '3 months')::date;
    part_name text;
    part_start date;
    part_end date;
BEGIN
    part_start := start_month;
    WHILE part_start < end_month LOOP
        part_end := (part_start + interval '1 month')::date;
        part_name := 'outbox_' || to_char(part_start, 'YYYY_MM');
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'vps' AND c.relname = part_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE vps.%I PARTITION OF vps.outbox_partitioned FOR VALUES FROM (%L) TO (%L)',
                part_name, part_start, part_end
            );
        END IF;
        part_start := part_end;
    END LOOP;
END $$;

INSERT INTO vps.outbox_partitioned (id, event_type, payload, published, created_at, worker_poll_claimed_at, worker_poll_claimed_by)
SELECT id, event_type, payload, published, created_at,
       worker_poll_claimed_at, worker_poll_claimed_by
FROM vps.outbox;

ALTER TABLE vps.outbox RENAME TO outbox_legacy;
ALTER TABLE vps.outbox_partitioned RENAME TO outbox;

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON vps.outbox (id ASC) WHERE published = false;
