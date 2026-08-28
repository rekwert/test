package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/platformmigrate"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/fieldcrypto"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Instance struct {
	ID                string
	Hostname          *string
	State             string
	IPAddress         *string
	Region            string
	PlanID            string
	PlanName          string
	OSName              string
	SoftwareProfileID   string
	PriceMonthly        float64
	CPU               int
	RAMMb             int
	DiskGB            int
	BillingStatus     string
	NodeID            *string
	NodeName          *string
	Metrics           json.RawMessage
	BillingPeriodDays int
	NextBillingAt     *time.Time
	CreatedAt         *time.Time
	OrderNumber       *int64
	AutoRenew         bool
	ProductType       string
	Provider          string
	ProviderMeta      json.RawMessage
	AdminBlock        bool
	SmtpOutboundOpen  bool
}

type Store struct {
	pool    *pgxpool.Pool
	secrets *fieldcrypto.Cipher
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := dbpool.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	secrets, err := fieldcrypto.NewFromEnv("VPS_FIELD_ENCRYPTION_KEY")
	if err != nil {
		pool.Close()
		return nil, err
	}
	prodenv.RequireFieldEncryptionKey("VPS_FIELD_ENCRYPTION_KEY")
	if prodenv.IsProduction() && !secrets.Enabled() {
		pool.Close()
		log.Fatal("VPS_FIELD_ENCRYPTION_KEY must be configured in production")
	}
	s := &Store{pool: pool, secrets: secrets}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	return platformmigrate.Apply(ctx, s.pool, "vps", migrations.FS)
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) ListInstances(ctx context.Context, userID string) ([]Instance, error) {
	items, _, err := s.ListInstancesPage(ctx, userID, 200, 0)
	return items, err
}

func (s *Store) ListInstancesPage(ctx context.Context, userID string, limit, offset int) ([]Instance, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM vps.instances
		WHERE user_id = $1 AND state <> 'deleted'
	`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.hostname, i.state, host(i.ip_address)::text, i.region, i.plan_id::text,
			i.billing_status, n.id::text, n.name, i.metrics,
			COALESCE(p.name, ''), COALESCE(p.cpu, 0), COALESCE(p.ram_mb, 0), COALESCE(p.disk_gb, 0),
			o.order_number, COALESCE(i.auto_renew, true),
			COALESCE(i.billing_period_days, 30), i.next_billing_at, i.created_at,
			CASE
				WHEN COALESCE(i.provider, 'openstack') IN ('hetzner_robot', 'hostkey') THEN COALESCE(o.os_template_id, '')
				ELSE TRIM(CONCAT(COALESCE(ot.name, ''), ' ', COALESCE(ot.version, '')))
			END,
			COALESCE(NULLIF(o.software_profile_id, ''), 'clean'),
			COALESCE(p.price_monthly, 0)::float8,
			COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack'),
			COALESCE(i.provider_meta, '{}'::jsonb),
			COALESCE(i.admin_block, false),
			COALESCE(i.smtp_outbound_open, false)
		FROM vps.instances i
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		LEFT JOIN vps.os_templates ot ON ot.id = o.os_template_id
		WHERE i.user_id = $1 AND i.state <> 'deleted'
		ORDER BY i.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Instance
	for rows.Next() {
		var inst Instance
		var nextBilling, createdAt *time.Time
		if err := rows.Scan(
			&inst.ID, &inst.Hostname, &inst.State, &inst.IPAddress, &inst.Region,
			&inst.PlanID, &inst.BillingStatus, &inst.NodeID, &inst.NodeName, &inst.Metrics,
			&inst.PlanName, &inst.CPU, &inst.RAMMb, &inst.DiskGB,
			&inst.OrderNumber, &inst.AutoRenew,
			&inst.BillingPeriodDays, &nextBilling, &createdAt,
			&inst.OSName, &inst.SoftwareProfileID, &inst.PriceMonthly,
			&inst.ProductType, &inst.Provider, &inst.ProviderMeta, &inst.AdminBlock,
			&inst.SmtpOutboundOpen,
		); err != nil {
			return nil, 0, err
		}
		inst.NextBillingAt = nextBilling
		inst.CreatedAt = createdAt
		if inst.ProductType == "dedicated" {
			inst.PriceMonthly = dedicatedBillingPrice(inst.ProviderMeta, inst.PriceMonthly)
		}
		items = append(items, inst)
	}
	return items, total, rows.Err()
}

func (s *Store) ListInstancesAdmin(ctx context.Context, userID string) ([]Instance, error) {
	// Same enrichment as client list — ticket sidebar needs plan / node / specs.
	return s.ListInstances(ctx, userID)
}

func (s *Store) FindInstanceByIP(ctx context.Context, ip string) (*Instance, error) {
	var inst Instance
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, hostname, state, host(ip_address)::text, region, plan_id::text, billing_status,
			COALESCE(admin_block, false)
		FROM vps.instances
		WHERE host(ip_address) = $1
		  AND state NOT IN ('deleted', 'error')
		ORDER BY created_at DESC
		LIMIT 1
	`, ip).Scan(&inst.ID, &inst.Hostname, &inst.State, &inst.IPAddress, &inst.Region, &inst.PlanID, &inst.BillingStatus, &inst.AdminBlock)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Store) GetInstanceByID(ctx context.Context, instanceID string) (*Instance, error) {
	var inst Instance
	var createdAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, hostname, state, host(ip_address)::text, region, plan_id::text, billing_status, created_at,
			COALESCE(admin_block, false)
		FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&inst.ID, &inst.Hostname, &inst.State, &inst.IPAddress, &inst.Region, &inst.PlanID, &inst.BillingStatus, &createdAt, &inst.AdminBlock)
	if err != nil {
		return nil, err
	}
	inst.CreatedAt = createdAt
	return &inst, nil
}

func (s *Store) GetInstanceForUser(ctx context.Context, userID, instanceID string) (*Instance, error) {
	var inst Instance
	var nextBilling, createdAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT i.id::text, i.hostname, i.state, host(i.ip_address)::text, i.region, i.plan_id::text,
			i.billing_status, n.id::text, i.metrics,
			COALESCE(p.name, ''), COALESCE(p.cpu, 0), COALESCE(p.ram_mb, 0), COALESCE(p.disk_gb, 0),
			COALESCE(i.billing_period_days, 30), i.next_billing_at, i.created_at,
			o.order_number, COALESCE(i.auto_renew, true),
			CASE
				WHEN COALESCE(i.provider, 'openstack') IN ('hetzner_robot', 'hostkey') THEN COALESCE(o.os_template_id, '')
				ELSE TRIM(CONCAT(COALESCE(ot.name, ''), ' ', COALESCE(ot.version, '')))
			END,
			COALESCE(NULLIF(o.software_profile_id, ''), 'clean'),
			COALESCE(p.price_monthly, 0)::float8,
			COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack'),
			COALESCE(i.provider_meta, '{}'::jsonb),
			COALESCE(i.admin_block, false),
			COALESCE(i.smtp_outbound_open, false)
		FROM vps.instances i
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		LEFT JOIN vps.os_templates ot ON ot.id = o.os_template_id
		WHERE i.id = $1::uuid AND i.user_id = $2::uuid
	`, instanceID, userID).Scan(
		&inst.ID, &inst.Hostname, &inst.State, &inst.IPAddress, &inst.Region,
		&inst.PlanID, &inst.BillingStatus, &inst.NodeID, &inst.Metrics,
		&inst.PlanName, &inst.CPU, &inst.RAMMb, &inst.DiskGB,
		&inst.BillingPeriodDays, &nextBilling, &createdAt,
		&inst.OrderNumber, &inst.AutoRenew, &inst.OSName, &inst.SoftwareProfileID, &inst.PriceMonthly,
		&inst.ProductType, &inst.Provider, &inst.ProviderMeta, &inst.AdminBlock,
		&inst.SmtpOutboundOpen,
	)
	if err != nil {
		return nil, err
	}
	inst.NextBillingAt = nextBilling
	inst.CreatedAt = createdAt
	if inst.ProductType == "dedicated" {
		inst.PriceMonthly = dedicatedBillingPrice(inst.ProviderMeta, inst.PriceMonthly)
	}
	return &inst, nil
}

var instanceUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func isInstanceUUIDRef(ref string) bool {
	return instanceUUIDRe.MatchString(strings.TrimSpace(ref))
}

// ResolveInstanceForUser loads an instance by UUID or hostname slug (lowercase hostname).
func (s *Store) ResolveInstanceForUser(ctx context.Context, userID, ref string) (*Instance, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, pgx.ErrNoRows
	}
	if isInstanceUUIDRef(ref) {
		return s.GetInstanceForUser(ctx, userID, ref)
	}
	slug := strings.ToLower(ref)
	var inst Instance
	var nextBilling, createdAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT i.id::text, i.hostname, i.state, host(i.ip_address)::text, i.region, i.plan_id::text,
			i.billing_status, n.id::text, i.metrics,
			COALESCE(p.name, ''), COALESCE(p.cpu, 0), COALESCE(p.ram_mb, 0), COALESCE(p.disk_gb, 0),
			COALESCE(i.billing_period_days, 30), i.next_billing_at, i.created_at,
			o.order_number, COALESCE(i.auto_renew, true),
			CASE
				WHEN COALESCE(i.provider, 'openstack') IN ('hetzner_robot', 'hostkey') THEN COALESCE(o.os_template_id, '')
				ELSE TRIM(CONCAT(COALESCE(ot.name, ''), ' ', COALESCE(ot.version, '')))
			END,
			COALESCE(NULLIF(o.software_profile_id, ''), 'clean'),
			COALESCE(p.price_monthly, 0)::float8,
			COALESCE(i.product_type, 'vps'), COALESCE(i.provider, 'openstack'),
			COALESCE(i.provider_meta, '{}'::jsonb),
			COALESCE(i.admin_block, false),
			COALESCE(i.smtp_outbound_open, false)
		FROM vps.instances i
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		LEFT JOIN vps.os_templates ot ON ot.id = o.os_template_id
		WHERE i.user_id = $1::uuid
		  AND i.state <> 'deleted'
		  AND lower(COALESCE(i.hostname, '')) = $2
		LIMIT 1
	`, userID, slug).Scan(
		&inst.ID, &inst.Hostname, &inst.State, &inst.IPAddress, &inst.Region,
		&inst.PlanID, &inst.BillingStatus, &inst.NodeID, &inst.Metrics,
		&inst.PlanName, &inst.CPU, &inst.RAMMb, &inst.DiskGB,
		&inst.BillingPeriodDays, &nextBilling, &createdAt,
		&inst.OrderNumber, &inst.AutoRenew, &inst.OSName, &inst.SoftwareProfileID, &inst.PriceMonthly,
		&inst.ProductType, &inst.Provider, &inst.ProviderMeta, &inst.AdminBlock,
		&inst.SmtpOutboundOpen,
	)
	if err != nil {
		return nil, err
	}
	inst.NextBillingAt = nextBilling
	inst.CreatedAt = createdAt
	if inst.ProductType == "dedicated" {
		inst.PriceMonthly = dedicatedBillingPrice(inst.ProviderMeta, inst.PriceMonthly)
	}
	return &inst, nil
}

func (s *Store) SetInstanceAutoRenew(ctx context.Context, userID, instanceID string, autoRenew bool) (*Instance, error) {
	if autoRenew {
		var blocked bool
		err := s.pool.QueryRow(ctx, `
			SELECT COALESCE((provider_meta->>'admin_issued')::boolean, false)
			    OR COALESCE((provider_meta->>'no_renew')::boolean, false)
			FROM vps.instances
			WHERE id = $1::uuid AND user_id = $2::uuid AND state <> 'deleted'
		`, instanceID, userID).Scan(&blocked)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("instance not found")
		}
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, fmt.Errorf("renewal not allowed for this instance")
		}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET auto_renew = $3,
		    billing_period_days = CASE
		      WHEN $3::boolean
		        AND (
		          COALESCE((provider_meta->>'free_week')::boolean, false)
		          OR COALESCE((provider_meta->>'trial')::boolean, false)
		        )
		        AND NOT COALESCE((provider_meta->>'converted_to_paid')::boolean, false)
		      THEN 30
		      ELSE billing_period_days
		    END,
		    provider_meta = CASE
		      WHEN $3::boolean AND COALESCE((provider_meta->>'free_week')::boolean, false) THEN
		        COALESCE(provider_meta, '{}'::jsonb)
		            || '{"converted_to_paid": true, "initial_prepaid_days": 30}'::jsonb
		      ELSE provider_meta
		    END,
		    updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND state <> 'deleted'
	`, instanceID, userID, autoRenew)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("instance not found")
	}
	return s.GetInstanceForUser(ctx, userID, instanceID)
}

func (s *Store) SetInstanceHostname(ctx context.Context, userID, instanceID, hostname string) (*Instance, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET hostname = $3, updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND state <> 'deleted'
	`, instanceID, userID, hostname)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("instance not found")
	}
	return s.GetInstanceForUser(ctx, userID, instanceID)
}

func (s *Store) MarkInstanceDeletePending(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) || '{"delete_pending":true}'::jsonb,
		    updated_at = now()
		WHERE id = $1::uuid AND state <> 'deleted'
	`, instanceID)
	return err
}

func (s *Store) SetInstanceState(ctx context.Context, instanceID, state string) error {
	if state == "deleted" {
		s.logInstanceIPReleaseIfAny(ctx, instanceID, IPSourceInstanceDeleted, "")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = $2,
		    ip_address = CASE WHEN $2 = 'deleted' THEN NULL ELSE ip_address END,
		    external_id = CASE WHEN $2 = 'deleted' THEN NULL ELSE external_id END,
		    next_billing_at = CASE
		      WHEN $2 = 'running' AND next_billing_at IS NULL THEN
		        now() + make_interval(days => COALESCE(
		          NULLIF((provider_meta->>'initial_prepaid_days')::int, 0),
		          NULLIF(billing_period_days, 0),
		          30
		        ))
		      ELSE next_billing_at
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, state)
	return err
}

func dedicatedBillingPrice(metaJSON json.RawMessage, fallback float64) float64 {
	if len(metaJSON) < 3 {
		return fallback
	}
	var meta map[string]any
	if err := json.Unmarshal(metaJSON, &meta); err != nil || meta == nil {
		return fallback
	}
	switch v := meta["billing_price_rub"].(type) {
	case float64:
		if v > 0 {
			return v
		}
	}
	return fallback
}
