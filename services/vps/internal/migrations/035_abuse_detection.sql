-- Automated abuse detection: signals, cases, instance hold flag.

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS abuse_hold BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS abuse_state JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS vps.abuse_signals (
    id          BIGSERIAL PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES vps.instances(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    signal_type TEXT NOT NULL,
    weight      INT NOT NULL CHECK (weight > 0 AND weight <= 200),
    evidence    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_abuse_signals_instance_created
    ON vps.abuse_signals (instance_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_abuse_signals_type_created
    ON vps.abuse_signals (signal_type, created_at DESC);

CREATE TABLE IF NOT EXISTS vps.abuse_cases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id     UUID NOT NULL REFERENCES vps.instances(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    status          TEXT NOT NULL DEFAULT 'auto_stopped'
        CHECK (status IN ('open', 'auto_stopped', 'confirmed', 'false_positive', 'resolved')),
    total_score     INT NOT NULL DEFAULT 0,
    trigger_reason  TEXT NOT NULL DEFAULT '',
    trigger_signals JSONB NOT NULL DEFAULT '[]'::jsonb,
    auto_stopped_at TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_abuse_cases_instance_open
    ON vps.abuse_cases (instance_id, status)
    WHERE status IN ('open', 'auto_stopped', 'confirmed');
