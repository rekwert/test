CREATE SCHEMA IF NOT EXISTS vps;

CREATE TABLE IF NOT EXISTS vps.plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    cpu INT NOT NULL,
    ram_mb INT NOT NULL,
    disk_gb INT NOT NULL,
    price_monthly NUMERIC(12, 2) NOT NULL,
    region TEXT NOT NULL DEFAULT 'moscow',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vps.orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    plan_id UUID NOT NULL REFERENCES vps.plans(id),
    region TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vps.instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    order_id UUID REFERENCES vps.orders(id),
    plan_id UUID NOT NULL REFERENCES vps.plans(id),
    external_id TEXT,
    region TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'creating',
    billing_status TEXT NOT NULL DEFAULT 'active',
    ip_address INET,
    hostname TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vps.instance_addons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES vps.instances(id),
    addon_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vps.outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    published BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO vps.plans (name, cpu, ram_mb, disk_gb, price_monthly, region) VALUES
    ('First', 1, 1024, 20, 149.00, 'moscow'),
    ('Pro', 4, 4096, 60, 499.00, 'moscow')
ON CONFLICT DO NOTHING;
