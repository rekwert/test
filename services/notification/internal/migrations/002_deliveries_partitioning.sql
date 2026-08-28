-- Monthly partitioning for notification.deliveries (email queue at scale).

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'notification' AND c.relname = 'deliveries_legacy'
    ) THEN
        RETURN;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS notification.deliveries_partitioned (
    LIKE notification.deliveries INCLUDING DEFAULTS
) PARTITION BY RANGE (created_at);

ALTER TABLE notification.deliveries_partitioned
    DROP CONSTRAINT IF EXISTS deliveries_pkey;

ALTER TABLE notification.deliveries_partitioned
    ADD PRIMARY KEY (id, created_at);

CREATE TABLE IF NOT EXISTS notification.deliveries_default
    PARTITION OF notification.deliveries_partitioned DEFAULT;

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
        part_name := 'deliveries_' || to_char(part_start, 'YYYY_MM');
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'notification' AND c.relname = part_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE notification.%I PARTITION OF notification.deliveries_partitioned FOR VALUES FROM (%L) TO (%L)',
                part_name, part_start, part_end
            );
        END IF;
        part_start := part_end;
    END LOOP;
END $$;

INSERT INTO notification.deliveries_partitioned
SELECT * FROM notification.deliveries;

ALTER TABLE notification.deliveries RENAME TO deliveries_legacy;
ALTER TABLE notification.deliveries_partitioned RENAME TO deliveries;

CREATE INDEX IF NOT EXISTS deliveries_pending_email_idx
    ON notification.deliveries (created_at)
    WHERE channel = 'email' AND status = 'pending';
