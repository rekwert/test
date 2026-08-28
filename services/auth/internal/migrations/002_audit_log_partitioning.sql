-- Monthly range partitioning for auth.audit_log (retention + index size at scale).
-- PK includes created_at (required for RANGE partitioning).

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'auth' AND c.relname = 'audit_log_legacy'
    ) THEN
        RETURN;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS auth.audit_log_partitioned (
    id BIGSERIAL,
    actor_id UUID,
    action TEXT NOT NULL,
    entity TEXT NOT NULL,
    entity_id TEXT,
    metadata JSONB,
    ip INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS auth.audit_log_default
    PARTITION OF auth.audit_log_partitioned DEFAULT;

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
        part_name := 'audit_log_' || to_char(part_start, 'YYYY_MM');
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'auth' AND c.relname = part_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE auth.%I PARTITION OF auth.audit_log_partitioned FOR VALUES FROM (%L) TO (%L)',
                part_name, part_start, part_end
            );
        END IF;
        part_start := part_end;
    END LOOP;
END $$;

INSERT INTO auth.audit_log_partitioned (id, actor_id, action, entity, entity_id, metadata, ip, created_at)
SELECT id, actor_id, action, entity, entity_id, metadata, ip, created_at
FROM auth.audit_log;

ALTER TABLE auth.audit_log RENAME TO audit_log_legacy;
ALTER TABLE auth.audit_log_partitioned RENAME TO audit_log;

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON auth.audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON auth.audit_log (actor_id, created_at DESC);
