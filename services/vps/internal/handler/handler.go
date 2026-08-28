package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hvinit"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/password"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

var hostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Handler struct {
	jwtSecret  string
	store      *store.Store
	hypervisor hypervisor.Adapter
	robot      hetznerrobot.Client
	hostkey    hostkey.Client
}

func New(jwtSecret string, st *store.Store) *Handler {
	return &Handler{
		jwtSecret:  jwtSecret,
		store:      st,
		hypervisor: hvinit.NewAdapter(),
		robot:      hetznerrobot.NewFromEnv(),
		hostkey:    hostkey.NewFromEnv(),
	}
}

func (h *Handler) SetHypervisor(hv hypervisor.Adapter) {
	h.hypervisor = hv
}

func (h *Handler) SetHostkey(c hostkey.Client) {
	h.hostkey = c
}

func (h *Handler) SetRobot(c hetznerrobot.Client) {
	h.robot = c
}

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	productType := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("product_type")))
	if productType == "dedicated" {
		h.listDedicatedPlansJSON(w, r)
		return
	}

	// Public catalog: all active plans (landing shows the full offer).
	// available=false when admin disabled the region+tier line (LK OOS).
	regionTiers := map[string]map[string]struct{}{}
	if h.store != nil && !hypervisor.MockEnabled() {
		rt, err := h.store.ListEnabledRegionTiers(r.Context())
		if err != nil {
			log.Printf("ListPlans region tiers: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to list plans")
			return
		}
		regionTiers = rt
	}

	type planDTO struct {
		catalog.Plan
		Available bool `json:"available"`
	}
	out := make([]planDTO, 0, len(catalog.Plans()))
	for _, p := range catalog.Plans() {
		if !p.Active {
			continue
		}
		region := strings.ToLower(strings.TrimSpace(p.Region))
		tier := strings.ToLower(strings.TrimSpace(p.Tier))
		available := true
		if len(regionTiers) > 0 {
			// Admin region×line toggles (region_tiers) are the source of truth.
			_, available = regionTiers[region][tier]
		}
		out = append(out, planDTO{Plan: p, Available: available})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": out})
}

func (h *Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	resp := catalog.CatalogResponse()
	if h.store != nil {
		if templates, err := h.activeOSTemplates(r.Context()); err == nil && len(templates) > 0 {
			resp["os_templates"] = catalog.EnrichOSTemplatesWithSoftware(templates)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListRegions(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"regions": []any{}})
		return
	}
	items, err := h.store.ListRegions(r.Context())
	if err != nil {
		log.Printf("ListRegions: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list regions")
		return
	}
	backbone := backboneLatencyMatrix(items)
	out := make([]map[string]any, 0, len(items))
	for _, reg := range items {
		row := map[string]any{
			"id":        reg.Code,
			"code":      reg.Code,
			"name_en":   reg.NameEN,
			"name_ru":   reg.NameRU,
			"city_en":   reg.CityEN,
			"city_ru":   reg.CityRU,
			"enabled":   reg.Enabled,
			"available": reg.Available,
			"flag":      reg.Code,
		}
		if probeURL := regionProbeURL(reg.Code, reg.ProbeHost); probeURL != "" {
			row["probe_url"] = probeURL
		}
		if ms, ok := backboneMsForRegion(backbone, reg.Code); ok {
			row["backbone_ms"] = ms
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": out})
}

func (h *Handler) ListOSTemplates(w http.ResponseWriter, r *http.Request) {
	if h.store != nil {
		templates, err := h.activeOSTemplates(r.Context())
		if err != nil {
			log.Printf("ListOSTemplates: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to list os templates")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"os_templates": catalog.EnrichOSTemplatesWithSoftware(templates),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"os_templates": catalog.EnrichOSTemplatesWithSoftware(catalog.OSTemplates()),
	})
}

func (h *Handler) activeOSTemplates(ctx context.Context) ([]catalog.OSTemplate, error) {
	rows, err := h.store.ListActiveOSTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return catalog.FilterOSTemplatesByMap(hypervisor.ActiveOSTemplateMap()), nil
	}
	out := make([]catalog.OSTemplate, 0, len(rows))
	for _, row := range rows {
		out = append(out, catalog.OSTemplate{
			ID:      row.ID,
			Name:    row.Name,
			Version: row.Version,
			Family:  row.Family,
		})
	}
	return out, nil
}

func (h *Handler) osTemplateAvailable(ctx context.Context, osID string) bool {
	if h.store != nil {
		active, err := h.store.IsOSTemplateActive(ctx, osID)
		return err == nil && active
	}
	return hypervisor.OSTemplateConfigured(osID)
}

func (h *Handler) SoftwareForOS(w http.ResponseWriter, r *http.Request) {
	osID := r.PathValue("os_id")
	if osID == "" {
		writeError(w, http.StatusBadRequest, "os_id required")
		return
	}
	profiles, _ := catalog.SoftwareForOSOrClean(osID)
	writeJSON(w, http.StatusOK, map[string]any{
		"os_id":             osID,
		"software_profiles": profiles,
	})
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireVerifiedCustomer(w, r, "vps.write")
	if !ok {
		return
	}

	var req struct {
		PlanID            string   `json:"plan_id"`
		Region            string   `json:"region"`
		Hostname          string   `json:"hostname"`
		RootPassword      string   `json:"root_password"`
		OSTemplateID      string   `json:"os_template_id"`
		SoftwareProfileID string   `json:"software_profile_id"`
		PeriodMonths      int      `json:"period_months"`
		SSHKeyIDs         []string `json:"ssh_key_ids"`
		ExtraIPv4Qty      int      `json:"extra_ipv4_qty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Region = strings.TrimSpace(strings.ToLower(req.Region))
	if req.Region == "" {
		req.Region = "nl"
	}

	if h.store != nil {
		if planRow, err := h.store.GetPlanRow(r.Context(), req.PlanID); err == nil && planRow.ProductType == "dedicated" {
			req.RootPassword = strings.TrimSpace(req.RootPassword)
			if len(req.RootPassword) < 8 {
				writeError(w, http.StatusBadRequest, "root_password must be at least 8 characters")
				return
			}
			h.createDedicatedOrder(w, r, userID, planRow.ID, req.Region, req.Hostname, req.RootPassword, req.OSTemplateID, req.PeriodMonths, nil, req.ExtraIPv4Qty, planRow)
			return
		}
	}

	if !hypervisor.MockEnabled() && !hypervisor.RegionEnabled(req.Region) {
		writeError(w, http.StatusBadRequest, "region not available")
		return
	}
	plan, ok := catalog.PlanByID(req.PlanID)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown plan_id")
		return
	}
	if plan.Region != "" && plan.Region != req.Region {
		writeError(w, http.StatusBadRequest, "plan not available in region")
		return
	}
	if !hypervisor.MockEnabled() {
		waitlistTier := store.TierAcceptsCapacityWaitlist(plan.Tier)
		if !waitlistTier && hypervisor.ActivePlanMapLen() > 0 && !hypervisor.HasExplicitPlan(plan.ID) {
			writeError(w, http.StatusBadRequest, "plan not available")
			return
		}
		if h.store != nil {
			ok, err := h.store.RegionTierEnabled(r.Context(), req.Region, plan.Tier)
			if err != nil {
				log.Printf("CreateOrder tier check: %v", err)
				writeError(w, http.StatusInternalServerError, "order failed")
				return
			}
			if !ok {
				writeError(w, http.StatusBadRequest, "plan not available")
				return
			}
		}
	}
	if !hypervisor.MockEnabled() {
		regions, err := h.store.ListRegions(r.Context())
		if err == nil {
			regionOK := false
			for _, r := range regions {
				if r.Code == req.Region && r.Enabled && r.Available {
					regionOK = true
					break
				}
			}
			if !regionOK {
				writeError(w, http.StatusBadRequest, "region not available")
				return
			}
		}
	}
	if req.OSTemplateID == "" {
		writeError(w, http.StatusBadRequest, "os_template_id required")
		return
	}
	if !h.osTemplateAvailable(r.Context(), req.OSTemplateID) {
		writeError(w, http.StatusBadRequest, "os_template not available")
		return
	}
	if _, ok := catalog.SoftwareForOSOrClean(req.OSTemplateID); !ok {
		writeError(w, http.StatusBadRequest, "unknown os_template_id")
		return
	}
	softwareID := req.SoftwareProfileID
	if softwareID == "" {
		softwareID = "clean"
	}
	if !catalog.SoftwareAllowedForPlan(plan.Name, plan.Tier, req.OSTemplateID, softwareID) {
		writeError(w, http.StatusBadRequest, "software not compatible with os")
		return
	}
	req.Hostname = strings.TrimSpace(strings.ToLower(req.Hostname))
	if req.Hostname != "" && !hostnameRe.MatchString(req.Hostname) {
		writeError(w, http.StatusBadRequest, "invalid hostname")
		return
	}
	if req.PeriodMonths <= 0 {
		req.PeriodMonths = 1
	}
	req.RootPassword = strings.TrimSpace(req.RootPassword)
	if req.RootPassword != "" && len(req.RootPassword) < 8 {
		writeError(w, http.StatusBadRequest, "root_password must be at least 8 characters")
		return
	}
	if req.RootPassword == "" {
		generated, err := password.GenerateRoot()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "order failed")
			return
		}
		req.RootPassword = generated
	}

	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "orders unavailable")
		return
	}

	result, err := h.store.CreateOrderWithBilling(r.Context(), store.CreateOrderInput{
		UserID:            userID,
		PlanID:            plan.ID,
		Region:            req.Region,
		Hostname:          req.Hostname,
		RootPassword:      req.RootPassword,
		OSTemplateID:      req.OSTemplateID,
		SoftwareProfileID: softwareID,
		PeriodMonths:      req.PeriodMonths,
		SSHKeyIDs:         req.SSHKeyIDs,
	})
	if err != nil {
		log.Printf("CreateOrder failed user=%s plan=%s region=%s: %v", userID, plan.ID, req.Region, err)
		switch {
		case errors.Is(err, store.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing account suspended")
		case errors.Is(err, store.ErrPlanNotFound):
			writeError(w, http.StatusBadRequest, "plan not available")
		case errors.Is(err, store.ErrInvalidPeriod):
			writeError(w, http.StatusBadRequest, "invalid period_months")
		case errors.Is(err, store.ErrNoNodeForRegion):
			writeError(w, http.StatusBadRequest, "region not available")
		case errors.Is(err, store.ErrTrialAlreadyUsed):
			writeError(w, http.StatusConflict, "trial already used")
		default:
			writeError(w, http.StatusInternalServerError, "order failed")
		}
		return
	}

	msg := "order paid, instance provisioning started"
	if result.Queued {
		msg = "order paid, instance waiting for capacity"
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":                  result.OrderID,
		"order_number":        result.OrderNumber,
		"instance_id":         result.InstanceID,
		"user_id":             userID,
		"plan_id":             plan.ID,
		"region":              req.Region,
		"hostname":            req.Hostname,
		"os_template_id":      req.OSTemplateID,
		"software_profile_id": softwareID,
		"period_months":       req.PeriodMonths,
		"amount_charged":      result.Amount,
		"status":              result.Status,
		"queued":              result.Queued,
		"message":             msg,
		"created_at":          time.Now().UTC(),
	})
}

// FreeWeekStatus reports whether the user can still claim a one-time
// 7-day free PROSTO-1 on any location.
func (h *Handler) FreeWeekStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), h.jwtSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	available := true
	if h.store != nil {
		used, err := h.store.UserHasFreeWeek(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "status failed")
			return
		}
		available = !used
	}
	postTrialDiscount := 0.0
	if h.store != nil {
		if pct, err := h.store.UserPostTrialDiscountPercent(r.Context(), userID); err == nil {
			postTrialDiscount = pct
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available":                   available,
		"plan":                        "PROSTO-1",
		"days":                        7,
		"post_trial_discount_percent": postTrialDiscount,
	})
}

func (h *Handler) ListInstances(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireCustomer(w, r, "vps.read")
	if !ok {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []any{}})
		return
	}
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1_000_000)
	items, total, err := h.store.ListInstancesPage(r.Context(), userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, inst := range items {
		out = append(out, instanceToJSON(inst))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instances": out,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func (h *Handler) GetInstanceBySlug(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireCustomer(w, r, "vps.read")
	if !ok {
		return
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	inst, err := h.store.ResolveInstanceForUser(r.Context(), userID, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	writeJSON(w, http.StatusOK, instanceToJSON(*inst))
}

func (h *Handler) GetInstance(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireCustomer(w, r, "vps.read")
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	inst, err := h.store.ResolveInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	writeJSON(w, http.StatusOK, instanceToJSON(*inst))
}

func powerBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "423") ||
		strings.Contains(msg, "pending tasks") ||
		strings.Contains(msg, "already pending") ||
		strings.Contains(msg, "task in progress") ||
		strings.Contains(msg, "busy")
}

func runPowerWithRetry(ctx context.Context, fn func() error) error {
	var last error
	// ~15s total wait — covers VirtFusion "pending tasks" after package/reboot.
	for attempt := 1; attempt <= 5; attempt++ {
		last = fn()
		if last == nil {
			return nil
		}
		if !powerBusyError(last) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return last
}

func (h *Handler) InstanceAction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireCustomer(w, r, "vps.manage")
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	var action string
	var targetState string
	switch {
	case strings.HasSuffix(r.URL.Path, "/start"):
		action = "start"
		targetState = "running"
	case strings.HasSuffix(r.URL.Path, "/stop"):
		action = "stop"
		targetState = "stopped"
	case strings.HasSuffix(r.URL.Path, "/reboot"):
		action = "reboot"
		targetState = "running"
	case r.Method == http.MethodDelete:
		action = "delete"
		targetState = "deleted"
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}

	inst, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	// Provisioning / waitlist: no power actions until the VM exists.
	if action == "delete" {
		if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" {
			writeError(w, http.StatusConflict, "server is busy")
			return
		}
	}
	if action != "delete" {
		if inst.State == "creating" || inst.State == "queued" || inst.IPAddress == nil || *inst.IPAddress == "" {
			writeError(w, http.StatusConflict, "server is still provisioning")
			return
		}
	}

	if action == "start" || action == "reboot" {
		hold, holdErr := h.store.InstanceAbuseHold(r.Context(), instanceID)
		if holdErr == nil && hold {
			writeError(w, http.StatusForbidden, "server suspended pending abuse review — open a support ticket")
			return
		}
		adminBlock, blockErr := h.store.InstanceAdminBlock(r.Context(), instanceID)
		if blockErr == nil && adminBlock {
			writeError(w, http.StatusForbidden, "server blocked by administrator — contact support")
			return
		}
		if store.ClientPowerBlocked(inst.BillingStatus) {
			writeError(w, http.StatusForbidden, "server unavailable — top up balance to restore access")
			return
		}
	}

	externalID, err := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if err != nil || externalID == "" {
		if action != "delete" {
			writeError(w, http.StatusConflict, "server is still provisioning")
			return
		}
	}

	hv := h.hv()
	robot := h.robot
	if robot == nil {
		robot = hetznerrobot.NewFromEnv()
	}
	isDedicated := store.IsDedicatedProvider(inst.Provider) || inst.ProductType == "dedicated"
	switch action {
	case "start":
		var pErr error
		if isDedicated {
			if inst.Provider == "hostkey" {
				hk := h.hostkey
				if hk == nil {
					hk = hostkey.NewFromEnv()
				}
				pErr = hk.PowerOn(r.Context(), externalID)
			} else {
				pErr = robot.Reset(r.Context(), externalID, "power")
			}
		} else {
			pErr = runPowerWithRetry(r.Context(), func() error {
				return hv.StartServer(r.Context(), externalID)
			})
		}
		if pErr != nil {
			log.Printf("start %s: %v", instanceID, pErr)
			if powerBusyError(pErr) {
				writeError(w, http.StatusConflict, "server is busy, try again shortly")
				return
			}
			writeError(w, http.StatusBadGateway, "start failed")
			return
		}
	case "stop":
		var pErr error
		if isDedicated {
			if inst.Provider == "hostkey" {
				hk := h.hostkey
				if hk == nil {
					hk = hostkey.NewFromEnv()
				}
				pErr = hk.PowerOff(r.Context(), externalID)
			} else {
				pErr = robot.Reset(r.Context(), externalID, "power")
			}
		} else {
			pErr = runPowerWithRetry(r.Context(), func() error {
				return hv.StopServer(r.Context(), externalID)
			})
		}
		if pErr != nil {
			log.Printf("stop %s: %v", instanceID, pErr)
			if powerBusyError(pErr) {
				writeError(w, http.StatusConflict, "server is busy, try again shortly")
				return
			}
			writeError(w, http.StatusBadGateway, "stop failed")
			return
		}
	case "reboot":
		var pErr error
		if isDedicated {
			if inst.Provider == "hostkey" {
				hk := h.hostkey
				if hk == nil {
					hk = hostkey.NewFromEnv()
				}
				pErr = hk.Reboot(r.Context(), externalID)
			} else {
				pErr = robot.Reset(r.Context(), externalID, "hw")
			}
		} else {
			pErr = runPowerWithRetry(r.Context(), func() error {
				return hv.RebootServer(r.Context(), externalID)
			})
		}
		if pErr != nil {
			log.Printf("reboot %s: %v", instanceID, pErr)
			if powerBusyError(pErr) {
				writeError(w, http.StatusConflict, "server is busy, try again shortly")
				return
			}
			writeError(w, http.StatusBadGateway, "reboot failed")
			return
		}
	case "delete":
		if externalID != "" && !isDedicated {
			if err := h.store.EnqueueInstanceDestroy(r.Context(), instanceID, externalID, userID); err != nil {
				log.Printf("enqueue destroy %s: %v", instanceID, err)
				writeError(w, http.StatusInternalServerError, "delete failed")
				return
			}
		} else if externalID != "" {
			if isDedicated {
				log.Printf("dedicated delete local-only %s external=%s", instanceID, externalID)
			}
		}
	}

	deleteStatus := http.StatusOK
	if action == "delete" && !isDedicated && externalID != "" {
		deleteStatus = http.StatusAccepted
	}

	if err := h.store.SetInstanceState(r.Context(), instanceID, targetState); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	inst.State = targetState
	writeJSON(w, deleteStatus, map[string]any{
		"ok":       true,
		"instance": instanceToJSON(*inst),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) requireBillingAccess(w http.ResponseWriter, r *http.Request, instanceID string) bool {
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return false
	}
	blocked, err := h.store.ClientBillingAccessBlocked(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "billing check failed")
		return false
	}
	if blocked {
		writeError(w, http.StatusForbidden, "server unavailable — top up balance to restore access")
		return false
	}
	return true
}

func invoiceListParams(r *http.Request) (limit, offset int) {
	limit = queryInt(r, "limit", 50, 1, 100)
	offset = queryInt(r, "offset", 0, 0, 1_000_000)
	return limit, offset
}

func queryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
