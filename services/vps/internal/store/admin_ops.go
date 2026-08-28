package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AdminInstanceRow struct {
	ID            string
	UserID        string
	UserEmail     string
	Hostname      *string
	State         string
	IPAddress     *string
	Region        string
	PlanID        string
	PlanName      string
	CPU           int
	RAMMb         int
	DiskGB        int
	BillingStatus string
	ExternalID    *string
	NodeID        *string
	NodeName      *string
	NodeVFName    *string
	OrderID       *string
	OrderNumber   *int64
	CreatedAt     time.Time
	NextBillingAt *time.Time
	ProductType   string
}

type NodeRow struct {
	ID                string
	Name              string
	Region            string
	Status            string
	CapacityInstances int
	ActiveInstances   int
	PendingInstances  int
	SupportedTiers    []string
	ExternalID        *string
	VFName            *string
	VFIP              *string
	VFEnabled         *bool
	MaintenanceMode   bool
	MaxCPUCores       *int
	CPUAllocated      *int
	CPUUsedPercent    *float64
	MaxMemoryMB       *int
	MemoryAllocatedMB *int
	MemoryUsedPercent *float64
	MaxDiskGB         *int
	DiskAllocatedGB   *int
	DiskUsedPercent   *float64
	VFServerCount     *int
	LastSyncedAt      *time.Time
}

type DashboardStats struct {
	UsersCount            int
	InstancesActive       int
	InstancesRunning      int
	InstancesGrace        int
	InstancesCreating     int
	InstancesError        int
	InstancesNotRunning   int
	TicketsOpen           int
	TicketsStale24h       int
	RegistrationsPeriod   int
	RevenueTotal          float64
	NodesOnline           int
	NodesOffline          int
	NodesTotal            int
	ClientsOnFreePlan     int
	PeriodDays            int
	PeriodFrom            string
	PeriodTo              string
}

type AdminAction struct {
	ID         int64
	StaffID    *string
	UserID     *string
	InstanceID *string
	Action     string
	Details    json.RawMessage
	CreatedAt  time.Time
}

type StatsPeriod struct {
	From time.Time
	To   time.Time
	Days int
}

func NewStatsPeriod(days int, fromRaw, toRaw string) StatsPeriod {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	parseDay := func(raw string) (time.Time, bool) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return time.Time{}, false
		}
		t, err := time.ParseInLocation("2006-01-02", raw, time.UTC)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}

	from, okFrom := parseDay(fromRaw)
	to, okTo := parseDay(toRaw)
	if okFrom && okTo {
		if to.Before(from) {
			from, to = to, from
		}
		if to.Sub(from) > 366*24*time.Hour {
			from = to.AddDate(0, 0, -365)
		}
		d := int(to.Sub(from).Hours()/24) + 1
		return StatsPeriod{From: from, To: to, Days: d}
	}

	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	from = today.AddDate(0, 0, -(days - 1))
	return StatsPeriod{From: from, To: today, Days: days}
}

func (s *Store) DashboardStats(ctx context.Context, period StatsPeriod) (*DashboardStats, error) {
	var stats DashboardStats
	stats.PeriodDays = period.Days
	stats.PeriodFrom = period.From.Format("2006-01-02")
	stats.PeriodTo = period.To.Format("2006-01-02")
	toExclusive := period.To.AddDate(0, 0, 1)

	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(DISTINCT u.id)::int
			 FROM auth.users u
			 JOIN auth.user_roles ur ON ur.user_id = u.id
			 JOIN auth.roles r ON r.id = ur.role_id
			 WHERE r.name = 'client' AND u.deleted_at IS NULL),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE billing_status = 'active' AND state <> 'deleted'),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE state = 'running' AND billing_status IN ('active', 'grace_period')),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE billing_status IN ('grace_period', 'past_due') AND state <> 'deleted'),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE state IN ('creating', 'queued')),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE state = 'error'),
			(SELECT COUNT(*)::int FROM vps.instances
			 WHERE state <> 'deleted'
			   AND billing_status IN ('active', 'grace_period')
			   AND state IN ('stopped', 'suspended')),
			(SELECT COUNT(*)::int FROM support.tickets
			 WHERE status NOT IN ('closed', 'resolved', 'answered')),
			(SELECT COUNT(*)::int FROM support.tickets
			 WHERE status IN ('open', 'in_progress', 'return_pending', 'waiting_client')
			   AND COALESCE(last_message_at, created_at) < now() - interval '24 hours'),
			(SELECT COUNT(*)::int FROM auth.users
			 WHERE deleted_at IS NULL
			   AND created_at >= $1
			   AND created_at < $2),
			COALESCE((SELECT SUM(amount)::float8 FROM billing.invoices WHERE status = 'paid' AND invoice_type = 'topup'), 0),
			(SELECT COUNT(*)::int FROM vps.nodes
			 WHERE status = 'online'
			   AND external_id IS NOT NULL AND external_id <> ''
			   AND COALESCE(vf_enabled, true) = true
			   AND COALESCE(maintenance_mode, false) = false),
			(SELECT COUNT(*)::int FROM vps.nodes
			 WHERE external_id IS NOT NULL AND external_id <> ''
			   AND (
			     status <> 'online'
			     OR COALESCE(vf_enabled, true) = false
			     OR COALESCE(maintenance_mode, false) = true
			   )),
			(SELECT COUNT(*)::int FROM vps.nodes
			 WHERE external_id IS NOT NULL AND external_id <> ''),
			(SELECT COUNT(DISTINCT i.user_id)::int
			 FROM vps.instances i
			 JOIN vps.plans p ON p.id = i.plan_id
			 WHERE i.billing_status IN ('active', 'grace_period')
			   AND i.state NOT IN ('deleted', 'creating', 'queued')
			   AND (
			     COALESCE((i.provider_meta->>'free_week')::boolean, false)
			     OR COALESCE((i.provider_meta->>'trial')::boolean, false)
			     OR LOWER(COALESCE(p.tier, '')) = 'trial'
			   ))
	`, period.From, toExclusive).Scan(
		&stats.UsersCount,
		&stats.InstancesActive,
		&stats.InstancesRunning,
		&stats.InstancesGrace,
		&stats.InstancesCreating,
		&stats.InstancesError,
		&stats.InstancesNotRunning,
		&stats.TicketsOpen,
		&stats.TicketsStale24h,
		&stats.RegistrationsPeriod,
		&stats.RevenueTotal,
		&stats.NodesOnline,
		&stats.NodesOffline,
		&stats.NodesTotal,
		&stats.ClientsOnFreePlan,
	)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (s *Store) ListAllInstances(ctx context.Context, query, ipQuery, userID string, limit int) ([]AdminInstanceRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := strings.TrimSpace(query)
	if q != "" {
		q = escapeLikePattern(q)
	}
	ip := strings.TrimSpace(ipQuery)
	if ip != "" {
		ip = escapeLikePattern(ip)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, COALESCE(u.email, ''), i.hostname, i.state,
			host(i.ip_address)::text, i.region, i.plan_id::text, COALESCE(p.name, ''),
			COALESCE(p.cpu, 0), COALESCE(p.ram_mb, 0), COALESCE(p.disk_gb, 0),
			i.billing_status, i.external_id, n.id::text, n.name, n.vf_name, i.order_id::text, o.order_number,
			i.created_at, i.next_billing_at, COALESCE(NULLIF(i.product_type, ''), 'vps')
		FROM vps.instances i
		LEFT JOIN auth.users u ON u.id = i.user_id
		LEFT JOIN vps.plans p ON p.id = i.plan_id
		LEFT JOIN vps.nodes n ON n.id = i.node_id
		LEFT JOIN vps.orders o ON o.id = i.order_id
		WHERE ($1 = '' OR u.email ILIKE '%' || $1 || '%' ESCAPE '\'
			OR i.hostname ILIKE '%' || $1 || '%' ESCAPE '\'
			OR host(i.ip_address) ILIKE '%' || $1 || '%' ESCAPE '\'
			OR i.id::text ILIKE '%' || $1 || '%' ESCAPE '\'
			OR i.order_id::text ILIKE '%' || $1 || '%' ESCAPE '\'
			OR CAST(o.order_number AS TEXT) ILIKE '%' || $1 || '%' ESCAPE '\')
		AND ($4 = '' OR host(i.ip_address) ILIKE '%' || $4 || '%' ESCAPE '\')
		AND ($2 = '' OR i.user_id = $2::uuid)
		AND i.state <> 'deleted'
		ORDER BY i.created_at DESC
		LIMIT $3
	`, q, userID, limit, ip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AdminInstanceRow
	for rows.Next() {
		var row AdminInstanceRow
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.UserEmail, &row.Hostname, &row.State,
			&row.IPAddress, &row.Region, &row.PlanID, &row.PlanName,
			&row.CPU, &row.RAMMb, &row.DiskGB,
			&row.BillingStatus, &row.ExternalID, &row.NodeID, &row.NodeName, &row.NodeVFName, &row.OrderID, &row.OrderNumber,
			&row.CreatedAt, &row.NextBillingAt, &row.ProductType,
		); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) ListNodes(ctx context.Context) ([]NodeRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.id::text, n.name, n.region, n.status, n.capacity_instances,
			(SELECT COUNT(*)::int FROM vps.instances i
			 WHERE i.node_id = n.id
			   AND i.state IN ('running', 'stopped', 'reinstalling', 'error')
			   AND COALESCE(i.billing_status, '') <> 'deleted'),
			(SELECT COUNT(*)::int FROM vps.instances i
			 WHERE i.node_id = n.id
			   AND i.state IN ('creating', 'queued')),
			COALESCE(n.supported_tiers, '{}'::text[]),
			n.external_id, n.vf_name, n.vf_ip, n.vf_enabled, n.maintenance_mode,
			n.max_cpu_cores, n.cpu_allocated, n.cpu_used_percent,
			n.max_memory_mb, n.memory_allocated_mb, n.memory_used_percent,
			n.max_disk_gb, n.disk_allocated_gb, n.disk_used_percent, n.vf_server_count, n.last_synced_at
		FROM vps.nodes n
		WHERE n.external_id IS NOT NULL AND n.external_id <> ''
		ORDER BY n.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NodeRow
	for rows.Next() {
		var row NodeRow
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Region, &row.Status, &row.CapacityInstances,
			&row.ActiveInstances, &row.PendingInstances,
			&row.SupportedTiers,
			&row.ExternalID, &row.VFName, &row.VFIP, &row.VFEnabled, &row.MaintenanceMode,
			&row.MaxCPUCores, &row.CPUAllocated, &row.CPUUsedPercent,
			&row.MaxMemoryMB, &row.MemoryAllocatedMB, &row.MemoryUsedPercent,
			&row.MaxDiskGB, &row.DiskAllocatedGB, &row.DiskUsedPercent, &row.VFServerCount, &row.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) SuspendClient(ctx context.Context, userID, staffID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id, billing_status)
		VALUES ($1, 'suspended')
		ON CONFLICT (user_id) DO UPDATE SET billing_status = 'suspended', updated_at = now()
	`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances SET billing_status = 'suspended', updated_at = now()
		WHERE user_id = $1::uuid AND billing_status = 'active'
	`, userID); err != nil {
		return err
	}
	if err := insertAdminAction(ctx, tx, staffID, userID, "", "client_suspend", nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UnsuspendClient(ctx context.Context, userID, staffID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts SET billing_status = 'active', updated_at = now()
		WHERE user_id = $1::uuid
	`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances SET billing_status = 'active', updated_at = now()
		WHERE user_id = $1::uuid AND billing_status = 'suspended'
	`, userID); err != nil {
		return err
	}
	if err := insertAdminAction(ctx, tx, staffID, userID, "", "client_unsuspend", nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpgradeInstancePlan(ctx context.Context, instanceID, newPlanID, staffID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var oldPlanID, userID string
	err = tx.QueryRow(ctx, `
		SELECT plan_id::text, user_id::text FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&oldPlanID, &userID)
	if err != nil {
		return err
	}

	var planName string
	err = tx.QueryRow(ctx, `SELECT name FROM vps.plans WHERE id = $1::uuid AND active = true`, newPlanID).Scan(&planName)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances SET plan_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, newPlanID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.instance_addons (instance_id, addon_type, status)
		VALUES ($1::uuid, 'plan_upgrade', 'applied')
	`, instanceID); err != nil {
		return err
	}

	details, _ := json.Marshal(map[string]string{
		"from_plan_id": oldPlanID,
		"to_plan_id":   newPlanID,
		"plan_name":    planName,
	})
	if err := insertAdminAction(ctx, tx, staffID, userID, instanceID, "instance_upgrade", details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListAdminActions(ctx context.Context, userID, instanceID string, limit int) ([]AdminAction, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, staff_id::text, user_id::text, instance_id::text, action, details, created_at
		FROM vps.admin_actions
		WHERE ($1 = '' OR user_id = $1::uuid)
		AND ($2 = '' OR instance_id = $2::uuid)
		ORDER BY created_at DESC
		LIMIT $3
	`, userID, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AdminAction
	for rows.Next() {
		var row AdminAction
		if err := rows.Scan(&row.ID, &row.StaffID, &row.UserID, &row.InstanceID, &row.Action, &row.Details, &row.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) LogAdminAction(ctx context.Context, staffID, userID, instanceID, action string, details json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vps.admin_actions (staff_id, user_id, instance_id, action, details)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, $4, $5)
	`, staffID, userID, instanceID, action, normalizeDetails(details))
	return err
}

func insertAdminAction(ctx context.Context, tx pgx.Tx, staffID, userID, instanceID, action string, details json.RawMessage) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vps.admin_actions (staff_id, user_id, instance_id, action, details)
		VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, $4, $5)
	`, staffID, userID, instanceID, action, normalizeDetails(details))
	return err
}

func normalizeDetails(details json.RawMessage) json.RawMessage {
	if len(details) == 0 {
		return json.RawMessage(`{}`)
	}
	return details
}
