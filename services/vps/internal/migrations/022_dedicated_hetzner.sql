-- Dedicated / Hetzner Robot catalog + instance provider fields

ALTER TABLE vps.plans
    ADD COLUMN IF NOT EXISTS product_type TEXT NOT NULL DEFAULT 'vps',
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'virtfusion',
    ADD COLUMN IF NOT EXISTS external_product_id TEXT,
    ADD COLUMN IF NOT EXISTS provider_meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS available BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS plans_provider_external_uidx
    ON vps.plans (provider, external_product_id)
    WHERE external_product_id IS NOT NULL AND TRIM(external_product_id) <> '';

CREATE INDEX IF NOT EXISTS plans_product_type_available_idx
    ON vps.plans (product_type, available, active)
    WHERE product_type = 'dedicated';

ALTER TABLE vps.instances
    ADD COLUMN IF NOT EXISTS product_type TEXT NOT NULL DEFAULT 'vps',
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'virtfusion',
    ADD COLUMN IF NOT EXISTS provider_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS instances_provider_creating_idx
    ON vps.instances (provider, state)
    WHERE state IN ('creating', 'reinstalling');
