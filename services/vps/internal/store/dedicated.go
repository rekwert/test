package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrLotUnavailable   = errors.New("dedicated lot unavailable")
	ErrLotPriceChanged  = errors.New("dedicated lot price changed")
	ErrNotDedicated     = errors.New("not a dedicated plan")
	ErrInvalidExtraIPv4 = errors.New("invalid extra_ipv4_qty")
)

// Stable UUID namespace for Hetzner market/product plan IDs.
var hetznerPlanNS = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

func DedicatedPlanID(providerKey string) string {
	return uuid.NewSHA1(hetznerPlanNS, []byte(providerKey)).String()
}

type DedicatedPlan struct {
	ID                string
	Name              string
	CPU               int
	RAMMb             int
	DiskGB            int
	PriceMonthly      float64
	Region            string
	Active            bool
	Available         bool
	ProductType       string
	Provider          string
	ExternalProductID string
	ProviderMeta      json.RawMessage
	SyncedAt          *time.Time
}

func (s *Store) UpsertDedicatedMarketPlans(ctx context.Context, cfg hetznerrobot.Config, products []hetznerrobot.MarketProduct) (int, error) {
	seen := make([]string, 0, len(products))
	n := 0
	for _, p := range products {
		extID := strconv.Itoa(p.ID)
		planID := DedicatedPlanID("market:" + extID)
		seen = append(seen, planID)
		region := regionFromDC(p.Datacenter)
		meta, _ := json.Marshal(map[string]any{
			"source":         p.Source,
			"cpu_model":      p.CPU,
			"cpu_benchmark":  p.CPUBenchmark,
			"memory_gb":      p.MemoryGB,
			"disk_gb":        p.DiskGB,
			"disk_count":     p.DiskCount,
			"disk_text":      p.DiskText,
			"datacenter":     p.Datacenter,
			"network_speed":  p.NetworkSpeed,
			"traffic":        p.Traffic,
			"price_eur":      p.PriceEUR,
			"price_setup_eur": p.PriceSetupEUR,
			"fixed_price":    p.FixedPrice,
			"dist":           p.Dist,
			"addons":         p.Addons,
			"description":    p.Description,
			"product_name":   p.Name,
		})
		price := cfg.SellPriceRub(p.PriceEUR)
		ramMb := p.MemoryGB * 1024
		if ramMb <= 0 {
			ramMb = 1024
		}
		disk := p.DiskGB
		if disk <= 0 {
			disk = 100
		}
		cpu := 1
		if p.CPUBenchmark > 0 {
			cpu = max(1, p.CPUBenchmark/5000)
		}
		// Prefer CPU label — Hetzner market product name is often the useless "Server Auction".
		name := fmt.Sprintf("%s · #%s", displayDedicatedName(p.Name, p.CPU), extID)
		_, err := s.pool.Exec(ctx, `
			INSERT INTO vps.plans (
				id, name, cpu, ram_mb, disk_gb, price_monthly, region, active,
				product_type, provider, external_product_id, provider_meta, available, synced_at
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6, $7, true,
				'dedicated', 'hetzner_robot', $8, $9::jsonb, true, now()
			)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				cpu = EXCLUDED.cpu,
				ram_mb = EXCLUDED.ram_mb,
				disk_gb = EXCLUDED.disk_gb,
				price_monthly = EXCLUDED.price_monthly,
				region = EXCLUDED.region,
				active = true,
				product_type = 'dedicated',
				provider = 'hetzner_robot',
				external_product_id = EXCLUDED.external_product_id,
				provider_meta = EXCLUDED.provider_meta,
				available = true,
				synced_at = now()
		`, planID, name, cpu, ramMb, disk, price, region, extID, meta)
		if err != nil {
			return n, err
		}
		n++
	}
	// Deactivate market lots no longer listed (keep ordered plans for history).
	if _, err := s.pool.Exec(ctx, `
		UPDATE vps.plans
		SET available = false, active = false, synced_at = now()
		WHERE product_type = 'dedicated'
		  AND provider = 'hetzner_robot'
		  AND COALESCE(provider_meta->>'source', 'market') = 'market'
		  AND NOT (id = ANY($1::uuid[]))
	`, seen); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Store) UpsertDedicatedServerProducts(ctx context.Context, cfg hetznerrobot.Config, products []hetznerrobot.ServerProduct) (int, error) {
	n := 0
	for _, p := range products {
		if strings.TrimSpace(p.ID) == "" {
			continue
		}
		planID := DedicatedPlanID("product:" + p.ID)
		region := "de"
		if len(p.Location) > 0 {
			region = regionFromDC(p.Location[0])
		}
		meta, _ := json.Marshal(map[string]any{
			"source":          "product",
			"cpu_model":       p.CPU,
			"memory_gb":       p.MemoryGB,
			"disk_gb":         p.DiskGB,
			"datacenter":      p.Datacenter,
			"price_eur":       p.PriceEUR,
			"price_setup_eur": p.PriceSetupEUR,
			"dist":            p.Dist,
			"location":        p.Location,
			"description":     p.Description,
			"product_name":    p.Name,
		})
		price := cfg.SellPriceRub(p.PriceEUR)
		ramMb := p.MemoryGB * 1024
		if ramMb <= 0 {
			ramMb = 4096
		}
		disk := p.DiskGB
		if disk <= 0 {
			disk = 500
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = p.ID
		} else if !strings.Contains(name, p.ID) {
			name = fmt.Sprintf("%s · %s", name, p.ID)
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO vps.plans (
				id, name, cpu, ram_mb, disk_gb, price_monthly, region, active,
				product_type, provider, external_product_id, provider_meta, available, synced_at
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6, $7, true,
				'dedicated', 'hetzner_robot', $8, $9::jsonb, true, now()
			)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				cpu = EXCLUDED.cpu,
				ram_mb = EXCLUDED.ram_mb,
				disk_gb = EXCLUDED.disk_gb,
				price_monthly = EXCLUDED.price_monthly,
				region = EXCLUDED.region,
				active = true,
				provider_meta = EXCLUDED.provider_meta,
				available = true,
				synced_at = now()
		`, planID, name, 4, ramMb, disk, price, region, p.ID, meta)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Store) ListDedicatedPlans(ctx context.Context) ([]DedicatedPlan, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, cpu, ram_mb, disk_gb, price_monthly::float8, region, active, available,
			product_type, provider, COALESCE(external_product_id, ''), provider_meta, synced_at
		FROM vps.plans
		WHERE product_type = 'dedicated' AND active = true AND available = true
		  AND COALESCE(provider_meta->>'source', 'market') IN ('market', 'preset', 'stock')
		  AND COALESCE(name, '') NOT ILIKE '%MOCK%'
		ORDER BY price_monthly ASC
		LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DedicatedPlan
	for rows.Next() {
		var p DedicatedPlan
		if err := rows.Scan(
			&p.ID, &p.Name, &p.CPU, &p.RAMMb, &p.DiskGB, &p.PriceMonthly, &p.Region, &p.Active, &p.Available,
			&p.ProductType, &p.Provider, &p.ExternalProductID, &p.ProviderMeta, &p.SyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (s *Store) GetPlanRow(ctx context.Context, planID string) (*DedicatedPlan, error) {
	var p DedicatedPlan
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, name, cpu, ram_mb, disk_gb, price_monthly::float8, region, active, available,
			COALESCE(product_type, 'vps'), COALESCE(provider, 'openstack'),
			COALESCE(external_product_id, ''), COALESCE(provider_meta, '{}'::jsonb), synced_at
		FROM vps.plans WHERE id = $1::uuid
	`, planID).Scan(
		&p.ID, &p.Name, &p.CPU, &p.RAMMb, &p.DiskGB, &p.PriceMonthly, &p.Region, &p.Active, &p.Available,
		&p.ProductType, &p.Provider, &p.ExternalProductID, &p.ProviderMeta, &p.SyncedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

type DedicatedCreating struct {
	ID             string
	UserID         string
	Hostname       string
	RootPassword   string
	ExternalID     string
	Provider       string
	OSTemplateID   string
	PlanID         string
	ProviderMeta   json.RawMessage
	ExternalProdID string
	UpdatedAt      time.Time
}

func (s *Store) ListDedicatedCreating(ctx context.Context, limit int) ([]DedicatedCreating, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, COALESCE(i.hostname, ''), COALESCE(i.root_password, ''),
			COALESCE(i.external_id, ''), COALESCE(i.provider, ''), COALESCE(o.os_template_id, ''), i.plan_id::text,
			COALESCE(i.provider_meta, '{}'::jsonb), COALESCE(p.external_product_id, ''), i.updated_at
		FROM vps.instances i
		LEFT JOIN vps.orders o ON o.id = i.order_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		WHERE i.state = 'creating' AND i.provider IN ('hetzner_robot', 'hostkey')
		ORDER BY i.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DedicatedCreating
	for rows.Next() {
		var item DedicatedCreating
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Hostname, &item.RootPassword, &item.ExternalID, &item.Provider,
			&item.OSTemplateID, &item.PlanID, &item.ProviderMeta, &item.ExternalProdID, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.RootPassword, err = s.openSecret(item.RootPassword)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListDedicatedExtraIPPending returns running dedicated instances waiting for extra IPv4 attachment.
func (s *Store) ListDedicatedExtraIPPending(ctx context.Context, limit int) ([]DedicatedCreating, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, COALESCE(i.hostname, ''), COALESCE(i.root_password, ''),
			COALESCE(i.external_id, ''), COALESCE(i.provider, ''), COALESCE(o.os_template_id, ''), i.plan_id::text,
			COALESCE(i.provider_meta, '{}'::jsonb), COALESCE(p.external_product_id, ''), i.updated_at
		FROM vps.instances i
		LEFT JOIN vps.orders o ON o.id = i.order_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		WHERE i.provider IN ('hetzner_robot', 'hostkey')
		  AND COALESCE(i.product_type, '') = 'dedicated'
		  AND i.state = 'running'
		  AND COALESCE(i.external_id, '') <> ''
		  AND COALESCE(i.provider_meta->>'extra_ipv4_status', '') IN ('pending', 'ordered', 'pending_manual')
		ORDER BY i.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DedicatedCreating
	for rows.Next() {
		var item DedicatedCreating
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Hostname, &item.RootPassword, &item.ExternalID, &item.Provider,
			&item.OSTemplateID, &item.PlanID, &item.ProviderMeta, &item.ExternalProdID, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.RootPassword, err = s.openSecret(item.RootPassword)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// DedicatedPriceReview is a running dedicated instance approaching renewal.
type DedicatedPriceReview struct {
	ID             string
	UserID         string
	Hostname       string
	PlanName       string
	PlanPrice      float64
	PlanMeta       json.RawMessage
	ProviderMeta   json.RawMessage
	NextBillingAt  time.Time
	OrderNumber    int64
}

// ListDedicatedPriceReview returns dedicated instances with renewal in the next withinDays days.
func (s *Store) ListDedicatedPriceReview(ctx context.Context, withinDays int, limit int) ([]DedicatedPriceReview, error) {
	if withinDays <= 0 {
		withinDays = 7
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, COALESCE(i.hostname, ''), COALESCE(p.name, ''),
			COALESCE(p.price_monthly, 0)::float8,
			COALESCE(p.provider_meta, '{}'::jsonb),
			COALESCE(i.provider_meta, '{}'::jsonb),
			i.next_billing_at,
			COALESCE(o.order_number, 0)
		FROM vps.instances i
		JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		WHERE i.provider IN ('hetzner_robot', 'hostkey')
		  AND COALESCE(i.product_type, '') = 'dedicated'
		  AND i.state NOT IN ('deleted', 'creating', 'error')
		  AND i.billing_status IN ('active', 'grace_period')
		  AND i.next_billing_at IS NOT NULL
		  AND i.next_billing_at > now()
		  AND i.next_billing_at <= now() + make_interval(days => $1)
		ORDER BY i.next_billing_at ASC
		LIMIT $2
	`, withinDays, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DedicatedPriceReview
	for rows.Next() {
		var item DedicatedPriceReview
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Hostname, &item.PlanName, &item.PlanPrice,
			&item.PlanMeta, &item.ProviderMeta, &item.NextBillingAt, &item.OrderNumber,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SetInstanceProviderMeta(ctx context.Context, instanceID string, meta map[string]any) error {
	b, _ := json.Marshal(meta)
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) || $2::jsonb, updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, b)
	return err
}

func (s *Store) CompleteDedicatedProvisioning(ctx context.Context, instanceID, externalID, ip, rootPassword string) error {
	stored := rootPassword
	if strings.TrimSpace(rootPassword) != "" {
		var err error
		stored, err = s.sealSecret(rootPassword)
		if err != nil {
			return err
		}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET state = 'running',
		    external_id = $2,
		    ip_address = NULLIF($3, '')::inet,
		    root_password = CASE WHEN NULLIF($4, '') IS NULL THEN root_password ELSE $4 END,
		    next_billing_at = CASE
		      WHEN next_billing_at IS NULL THEN
		        now() + make_interval(days => COALESCE(
		          NULLIF((provider_meta->>'initial_prepaid_days')::int, 0),
		          NULLIF(billing_period_days, 0),
		          30
		        ))
		      ELSE next_billing_at
		    END,
		    updated_at = now()
		WHERE id = $1::uuid AND state = 'creating'
	`, instanceID, externalID, ip, stored)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 && strings.TrimSpace(ip) != "" {
		userID, ownerErr := s.GetInstanceOwner(ctx, instanceID)
		if ownerErr == nil {
			_ = s.LogIPAssigned(ctx, userID, instanceID, ip, "", &IPAssignmentLogOpts{Source: IPSourceDedicatedProvision})
		}
	}
	return nil
}

func (s *Store) CreditBalance(ctx context.Context, userID string, amount float64, description string) error {
	if amount <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, userID); err != nil {
		return err
	}
	var balanceAfterKopecks int64
	creditKopecks := int64(math.Round(amount * 100))
	if err := tx.QueryRow(ctx, `
		UPDATE billing.accounts SET balance_kopecks = balance_kopecks + $2, updated_at = now()
		WHERE user_id = $1::uuid
		RETURNING balance_kopecks
	`, userID, creditKopecks).Scan(&balanceAfterKopecks); err != nil {
		return err
	}
	balanceAfter := float64(balanceAfterKopecks) / 100
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'refund', $4)
	`, userID, amount, description, balanceAfter); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetInstanceChargeAmount(ctx context.Context, instanceID string) (float64, error) {
	var amount float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(amount, 0)::float8
		FROM billing.invoices
		WHERE instance_id = $1::uuid AND invoice_type = 'charge' AND status = 'paid'
		ORDER BY created_at DESC
		LIMIT 1
	`, instanceID).Scan(&amount)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return amount, err
}

func (s *Store) GetInstanceProvider(ctx context.Context, instanceID string) (provider, productType, externalID string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(provider, 'openstack'), COALESCE(product_type, 'vps'), COALESCE(external_id, '')
		FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&provider, &productType, &externalID)
	return
}

func (s *Store) createDedicatedOrderWithBilling(ctx context.Context, in CreateOrderInput) (*CreateOrderResult, error) {
	// Dedicated: only 1-month prepaid (no multi-month discounts — Hetzner bills monthly).
	in.PeriodMonths = 1
	if in.SoftwareProfileID == "" {
		in.SoftwareProfileID = "clean"
	}

	sshKeys, err := s.ListSSHPublicKeys(ctx, in.UserID, in.SSHKeyIDs)
	if err != nil {
		return nil, err
	}
	sshKeysJSON, _ := json.Marshal(sshKeys)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var planName string
	var priceMonthly float64
	var available, active bool
	var productType, provider, extProd string
	var planMetaRaw []byte
	err = tx.QueryRow(ctx, `
		SELECT name, price_monthly::float8, available, active,
			COALESCE(product_type, 'vps'), COALESCE(provider, 'openstack'), COALESCE(external_product_id, ''),
			COALESCE(provider_meta, '{}'::jsonb)
		FROM vps.plans
		WHERE id = $1::uuid
		FOR UPDATE
	`, in.PlanID).Scan(&planName, &priceMonthly, &available, &active, &productType, &provider, &extProd, &planMetaRaw)
	if err == pgx.ErrNoRows {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	if productType != "dedicated" || !IsDedicatedProvider(provider) {
		return nil, ErrNotDedicated
	}
	if !active || !available {
		return nil, ErrLotUnavailable
	}
	if in.ExternalProductID == "" {
		in.ExternalProductID = extProd
	}

	cfg := hetznerrobot.LoadConfig().WithFreshFX(ctx)
	hkCfg := hostkey.LoadConfig().WithFreshFX(ctx)
	if in.ExtraIPv4Qty < 0 {
		in.ExtraIPv4Qty = 0
	}
	maxExtra := cfg.ExtraIPv4Max
	if provider == "hostkey" {
		maxExtra = hkCfg.ExtraIPv4Max
	}
	if in.ExtraIPv4Qty > maxExtra {
		return nil, ErrInvalidExtraIPv4
	}

	serverAmount := math.Round(priceMonthly*100) / 100
	ipAmount := cfg.ExtraIPv4OrderChargeRub(in.ExtraIPv4Qty, 1, 0)
	amount := math.Round((serverAmount+ipAmount)*100) / 100

	var promoDiscount float64
	if amount > 0 {
		promoDiscount, err = s.claimChargeDiscountTx(ctx, tx, in.UserID, "")
		if err != nil {
			return nil, err
		}
		if promoDiscount > 0 {
			amount = applyChargeDiscount(amount, promoDiscount)
		}
	}

	var planMeta map[string]any
	_ = json.Unmarshal(planMetaRaw, &planMeta)
	priceEUR := 0.0
	if planMeta != nil {
		switch v := planMeta["price_eur"].(type) {
		case float64:
			priceEUR = v
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, in.UserID); err != nil {
		return nil, err
	}

	var billingStatus string
	var balance float64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance, 0)::float8
		FROM billing.accounts a
		WHERE a.user_id = $1::uuid
		FOR UPDATE
	`, in.UserID).Scan(&billingStatus, &balance)
	if err != nil {
		return nil, err
	}
	if billingStatus == "suspended" {
		return nil, ErrBillingSuspended
	}
	if balance < amount {
		return nil, ErrInsufficientBalance
	}

	orderID := uuid.New().String()
	instanceID := uuid.New().String()
	prepaidDays := in.PeriodMonths * 30
	hostArg := strOrNil(in.Hostname)
	rootPassArg := any(nil)
	if in.RootPassword != "" {
		sealed, sealErr := s.sealSecret(in.RootPassword)
		if sealErr != nil {
			return nil, sealErr
		}
		rootPassArg = sealed
	}

	balanceAfter := balance
	if amount > 0 {
		if err := tx.QueryRow(ctx, `
			UPDATE billing.accounts
			SET balance = balance - $2, updated_at = now()
			WHERE user_id = $1::uuid
			RETURNING balance::float8
		`, in.UserID, amount).Scan(&balanceAfter); err != nil {
			return nil, err
		}
	}

	var orderNumber int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO vps.orders (id, user_id, plan_id, region, status, os_template_id, software_profile_id, hostname)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'paid', $5, $6, $7)
		RETURNING order_number
	`, orderID, in.UserID, in.PlanID, in.Region, in.OSTemplateID, in.SoftwareProfileID, hostArg).Scan(&orderNumber); err != nil {
		return nil, err
	}

	ipStatus := "none"
	if in.ExtraIPv4Qty > 0 {
		ipStatus = "pending"
	}
	metaObj := map[string]any{
		"external_product_id":   in.ExternalProductID,
		"extra_ipv4_qty":        in.ExtraIPv4Qty,
		"extra_ipv4_status":     ipStatus,
		"extra_ipv4_charged":    ipAmount,
		"server_charged":        serverAmount,
		"billing_price_rub":     priceMonthly,
		"price_eur":             priceEUR,
		"initial_prepaid_days":  prepaidDays,
	}
	metaJSON, _ := json.Marshal(metaObj)

	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.instances (
			id, user_id, order_id, plan_id, region, state, billing_status,
			hostname, root_password, billing_period_days, next_billing_at, provision_ssh_keys,
			product_type, provider, provider_meta
		)
		VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'creating', 'active',
			$6, $7, 30, NULL, $8::jsonb,
			'dedicated', $9, $10::jsonb
		)
	`, instanceID, in.UserID, orderID, in.PlanID, in.Region, hostArg, rootPassArg, sshKeysJSON, provider, metaJSON); err != nil {
		return nil, err
	}

	hostLabel := in.Hostname
	if hostLabel == "" {
		hostLabel = planName
	}
	desc := fmt.Sprintf("Dedicated initial charge — %s (1 mo.)", hostLabel)
	if in.ExtraIPv4Qty > 0 {
		desc = fmt.Sprintf("%s + %d extra IPv4", desc, in.ExtraIPv4Qty)
	}
	if promoDiscount > 0 {
		desc = fmt.Sprintf("%s (−%.0f%% post-trial)", desc, promoDiscount)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, in.UserID, amount, desc, instanceID, balanceAfter); err != nil {
		return nil, err
	}

	if amount > 0 {
		if err := s.processReferralPayment(ctx, tx, in.UserID, amount); err != nil {
			return nil, err
		}
	}

	outboxPayload, _ := json.Marshal(map[string]any{
		"instance_id":         instanceID,
		"order_id":            orderID,
		"user_id":             in.UserID,
		"plan_id":             in.PlanID,
		"region":              in.Region,
		"hostname":            in.Hostname,
		"os_template_id":      in.OSTemplateID,
		"software_profile_id": in.SoftwareProfileID,
		"external_product_id": in.ExternalProductID,
		"extra_ipv4_qty":      in.ExtraIPv4Qty,
		"ssh_keys":            sshKeys,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.outbox (event_type, payload)
		VALUES ('dedicated.provision_requested', $1::jsonb)
	`, outboxPayload); err != nil {
		return nil, err
	}

	// Mark lot unavailable so another customer cannot buy the same market id.
	if _, err := tx.Exec(ctx, `
		UPDATE vps.plans SET available = false, synced_at = now()
		WHERE id = $1::uuid AND product_type = 'dedicated'
	`, in.PlanID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CreateOrderResult{
		OrderID:     orderID,
		OrderNumber: orderNumber,
		InstanceID:  instanceID,
		Amount:      amount,
		Status:      "paid",
		Queued:      false,
	}, nil
}

func ValidateMarketPrice(catalogPriceRub, liveEUR float64, cfg hetznerrobot.Config) error {
	live := cfg.SellPriceRub(liveEUR)
	if live <= 0 {
		return ErrLotUnavailable
	}
	limit := catalogPriceRub * (1 + cfg.PriceSlackPct/100)
	if live > limit+0.01 {
		return fmt.Errorf("%w: catalog=%.2f live=%.2f", ErrLotPriceChanged, catalogPriceRub, live)
	}
	return nil
}

func displayDedicatedName(code, cpu string) string {
	code = strings.TrimSpace(code)
	cpu = strings.TrimSpace(cpu)
	if isGenericDedicatedProductName(code) {
		code = ""
	}
	if cpu != "" {
		return cpu
	}
	if code != "" {
		return code
	}
	return "Dedicated"
}

func isGenericDedicatedProductName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "server auction", "serverauction", "auction", "dedicated", "dedicated server":
		return true
	}
	return strings.Contains(n, "server auction")
}

func regionFromDC(dc string) string {
	dc = strings.ToUpper(strings.TrimSpace(dc))
	switch {
	case strings.HasPrefix(dc, "FSN"), strings.HasPrefix(dc, "NBG"), strings.HasPrefix(dc, "FKB"):
		return "de"
	case strings.HasPrefix(dc, "HEL"):
		return "fi"
	case strings.HasPrefix(dc, "ASH"):
		return "us"
	case strings.HasPrefix(dc, "SIN"):
		return "sg"
	default:
		if dc == "" {
			return "de"
		}
		return strings.ToLower(dc[:min(2, len(dc))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
