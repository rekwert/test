-- Platform integrity: outbox claims, billing constraints, FK hygiene, IP invariants.

CREATE SCHEMA IF NOT EXISTS platform;

ALTER TABLE vps.outbox
    ADD COLUMN IF NOT EXISTS claimed_at timestamptz,
    ADD COLUMN IF NOT EXISTS claimed_by text;

ALTER TABLE billing.invoices
    DROP CONSTRAINT IF EXISTS invoices_invoice_type_check;
ALTER TABLE billing.invoices
    ADD CONSTRAINT invoices_invoice_type_check
    CHECK (invoice_type IN ('topup', 'charge', 'refund'));

DROP TABLE IF EXISTS billing.outbox;

-- Recycled public IPs should not keep stale host keys forever.
CREATE OR REPLACE FUNCTION vps.cleanup_ip_ssh_host_keys() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.ip_address IS NOT NULL
           AND (NEW.ip_address IS NULL OR host(NEW.ip_address) IS DISTINCT FROM host(OLD.ip_address)) THEN
            DELETE FROM vps.ip_ssh_host_keys WHERE ip = OLD.ip_address;
        END IF;
    ELSIF TG_OP = 'DELETE' AND OLD.ip_address IS NOT NULL THEN
        DELETE FROM vps.ip_ssh_host_keys WHERE ip = OLD.ip_address;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_instances_ip_ssh_host_keys ON vps.instances;
CREATE TRIGGER trg_instances_ip_ssh_host_keys
    AFTER UPDATE OF ip_address OR DELETE ON vps.instances
    FOR EACH ROW EXECUTE FUNCTION vps.cleanup_ip_ssh_host_keys();

-- running without IP breaks panel/SSH; heal obvious stuck rows first.
UPDATE vps.instances
SET state = 'creating', updated_at = now()
WHERE state = 'running'
  AND ip_address IS NULL
  AND external_id IS NOT NULL
  AND TRIM(external_id) <> '';

UPDATE vps.instances
SET state = 'error',
    provider_meta = COALESCE(provider_meta, '{}'::jsonb)
        || jsonb_build_object('provision_error', 'running without ip (healed)', 'provision_failed_at', to_jsonb(now())),
    updated_at = now()
WHERE state = 'running' AND ip_address IS NULL;

ALTER TABLE vps.instances
    DROP CONSTRAINT IF EXISTS instances_running_requires_ip;
ALTER TABLE vps.instances
    ADD CONSTRAINT instances_running_requires_ip
    CHECK (state <> 'running' OR ip_address IS NOT NULL);

-- Cross-schema FKs (NOT VALID first; validate when clean).
ALTER TABLE vps.instances
    DROP CONSTRAINT IF EXISTS instances_user_id_fkey;
ALTER TABLE vps.instances
    ADD CONSTRAINT instances_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE vps.orders
    DROP CONSTRAINT IF EXISTS orders_user_id_fkey;
ALTER TABLE vps.orders
    ADD CONSTRAINT orders_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE billing.accounts
    DROP CONSTRAINT IF EXISTS accounts_user_id_fkey;
ALTER TABLE billing.accounts
    ADD CONSTRAINT accounts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE RESTRICT NOT VALID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM vps.instances i
        LEFT JOIN auth.users u ON u.id = i.user_id
        WHERE u.id IS NULL
    ) THEN
        ALTER TABLE vps.instances VALIDATE CONSTRAINT instances_user_id_fkey;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM vps.orders o
        LEFT JOIN auth.users u ON u.id = o.user_id
        WHERE u.id IS NULL
    ) THEN
        ALTER TABLE vps.orders VALIDATE CONSTRAINT orders_user_id_fkey;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM billing.accounts a
        LEFT JOIN auth.users u ON u.id = a.user_id
        WHERE u.id IS NULL
    ) THEN
        ALTER TABLE billing.accounts VALIDATE CONSTRAINT accounts_user_id_fkey;
    END IF;
END $$;

-- Idempotent IP-change refunds: collapse historical duplicates before unique index.
DELETE FROM billing.adjustments a
USING (
    SELECT user_id, reason, MIN(created_at) AS keep_at
    FROM billing.adjustments
    WHERE reason LIKE 'VPS IP change refund%'
    GROUP BY user_id, reason
    HAVING COUNT(*) > 1
) d
WHERE a.user_id = d.user_id
  AND a.reason = d.reason
  AND a.reason LIKE 'VPS IP change refund%'
  AND a.created_at > d.keep_at;

CREATE UNIQUE INDEX IF NOT EXISTS idx_adjustments_ip_change_refund_once
    ON billing.adjustments (user_id, reason)
    WHERE reason LIKE 'VPS IP change refund%';
