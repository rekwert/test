package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func (h *Handler) listDedicatedPlansJSON(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"plans": []any{}})
		return
	}
	items, err := h.store.ListDedicatedPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		row := map[string]any{
			"id":            p.ID,
			"name":          p.Name,
			"tier":          "dedicated",
			"cpu":           p.CPU,
			"ram_mb":        p.RAMMb,
			"disk_gb":       p.DiskGB,
			"price_monthly": p.PriceMonthly,
			"region":        p.Region,
			"active":        p.Active && p.Available,
			"product_type":  "dedicated",
			"provider":      p.Provider,
			"external_product_id": p.ExternalProductID,
		}
		if len(p.ProviderMeta) > 2 {
			row["provider_meta"] = json.RawMessage(p.ProviderMeta)
		}
		out = append(out, row)
	}
	cfg := hetznerrobot.LoadConfig().WithFreshFX(r.Context())
	hkCfg := hostkey.LoadConfig().WithFreshFX(r.Context())
	monthly, setup := cfg.ExtraIPv4UnitRub()
	writeJSON(w, http.StatusOK, map[string]any{
		"plans": out,
		"extra_ipv4": map[string]any{
			"monthly_rub": monthly,
			"setup_rub":   setup,
			"max_qty":     cfg.ExtraIPv4Max,
			"eur_monthly": cfg.ExtraIPv4EUR,
			"eur_setup":   cfg.ExtraIPv4SetupEUR,
		},
		"providers": map[string]any{
			"hetzner_robot": map[string]any{"enabled": cfg.Enabled},
			"hostkey": map[string]any{
				"enabled":       hkCfg.Enabled,
				"markup_percent": hkCfg.MarkupPercent,
				"extra_ipv4_max": hkCfg.ExtraIPv4Max,
			},
		},
	})
}

func (h *Handler) createDedicatedOrder(
	w http.ResponseWriter,
	r *http.Request,
	userID, planID, region, hostname, rootPassword, osTemplateID string,
	periodMonths int,
	sshKeyIDs []string,
	extraIPv4Qty int,
	plan *store.DedicatedPlan,
) {
	if plan.Provider == "hostkey" {
		h.createHostkeyDedicatedOrder(w, r, userID, planID, region, hostname, rootPassword, osTemplateID, periodMonths, extraIPv4Qty, plan)
		return
	}
	cfg := hetznerrobot.LoadConfig()
	robot := h.robot
	if robot == nil {
		robot = hetznerrobot.NewFromEnv()
	}

	extID := plan.ExternalProductID
	prodID, _ := strconv.Atoi(extID)
	source := "market"
	var meta map[string]any
	_ = json.Unmarshal(plan.ProviderMeta, &meta)
	if meta != nil {
		if s, ok := meta["source"].(string); ok && s != "" {
			source = s
		}
	}
	if source == "market" && prodID > 0 {
		live, err := robot.GetMarketProduct(r.Context(), prodID)
		if err != nil {
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
			return
		}
		if err := store.ValidateMarketPrice(plan.PriceMonthly, live.PriceEUR, cfg); err != nil {
			if errors.Is(err, store.ErrLotPriceChanged) {
				writeError(w, http.StatusConflict, "dedicated lot price changed")
				return
			}
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
			return
		}
	}

	osID := strings.TrimSpace(osTemplateID)
	if osID == "" {
		if meta != nil {
			if dists, ok := meta["dist"].([]any); ok && len(dists) > 0 {
				if s, ok := dists[0].(string); ok {
					osID = s
				}
			}
		}
	}
	if osID == "" {
		osID = "Ubuntu 24.04.1 LTS base"
	}

	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname != "" && !hostnameRe.MatchString(hostname) {
		writeError(w, http.StatusBadRequest, "invalid hostname")
		return
	}
	rootPassword = strings.TrimSpace(rootPassword)
	if len(rootPassword) < 8 {
		writeError(w, http.StatusBadRequest, "root_password must be at least 8 characters")
		return
	}
	// Dedicated: only monthly billing (Hetzner has no multi-month prepaid).
	periodMonths = 1
	region = strings.TrimSpace(strings.ToLower(region))
	if region == "" {
		region = plan.Region
	}
	if region == "" {
		region = "de"
	}

	result, err := h.store.CreateOrderWithBilling(r.Context(), store.CreateOrderInput{
		UserID:            userID,
		PlanID:            planID,
		Region:            region,
		Hostname:          hostname,
		RootPassword:      rootPassword,
		OSTemplateID:      osID,
		SoftwareProfileID: "clean",
		PeriodMonths:      periodMonths,
		SSHKeyIDs:         sshKeyIDs,
		ProductType:       "dedicated",
		Provider:          "hetzner_robot",
		ExternalProductID: extID,
		ExtraIPv4Qty:      extraIPv4Qty,
	})
	if err != nil {
		log.Printf("CreateDedicatedOrder failed user=%s plan=%s: %v", userID, plan.ID, err)
		switch {
		case errors.Is(err, store.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
		case errors.Is(err, store.ErrLotUnavailable):
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
		case errors.Is(err, store.ErrLotPriceChanged):
			writeError(w, http.StatusConflict, "dedicated lot price changed")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing suspended")
		case errors.Is(err, store.ErrInvalidPeriod):
			writeError(w, http.StatusBadRequest, "invalid period_months")
		case errors.Is(err, store.ErrInvalidExtraIPv4):
			writeError(w, http.StatusBadRequest, "invalid extra_ipv4_qty")
		default:
			writeError(w, http.StatusInternalServerError, "order failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":            result.OrderID,
		"order_number":        result.OrderNumber,
		"instance_id":         result.InstanceID,
		"plan_id":             plan.ID,
		"region":              region,
		"hostname":            hostname,
		"os_template_id":      osID,
		"software_profile_id": "clean",
		"period_months":       periodMonths,
		"extra_ipv4_qty":      extraIPv4Qty,
		"amount_charged":      result.Amount,
		"status":              result.Status,
		"queued":              false,
		"product_type":        "dedicated",
		"message":             "Dedicated server order paid; provisioning via Hetzner",
	})

	orderLabel := result.InstanceID
	if result.OrderNumber > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", result.OrderNumber, result.InstanceID)
	}
	alertBody := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nPlan: %s\nHostname: %s\nОС: %s\nСумма: %.0f ₽\n\nКлиент оформил dedicated. Пока сервер creating — напоминания раз в 15 минут.",
		orderLabel, userID, plan.Name, hostname, osID, result.Amount,
	)
	if err := notify.OpsAlert(r.Context(), "🛒 Dedicated: новый заказ", alertBody); err != nil {
		log.Printf("dedicated new-order ops alert: %v", err)
	}
	_ = h.store.SetInstanceProviderMeta(r.Context(), result.InstanceID, map[string]any{
		"last_ops_alert_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) createHostkeyDedicatedOrder(
	w http.ResponseWriter,
	r *http.Request,
	userID, planID, region, hostname, rootPassword, osTemplateID string,
	periodMonths int,
	extraIPv4Qty int,
	plan *store.DedicatedPlan,
) {
	cfg := hostkey.LoadConfig()
	hk := h.hostkey
	if hk == nil {
		hk = hostkey.NewFromEnv()
	}

	extID := plan.ExternalProductID
	var meta map[string]any
	_ = json.Unmarshal(plan.ProviderMeta, &meta)
	source := hostkeySourceFromHandlerMeta(meta)

	presetID, location, stockID, _ := hostkey.ParseExternalProductID(extID)
	if source == "stock" && stockID > 0 {
		// stock price validated from plan row
	} else if presetID > 0 {
		live, err := hk.GetPreset(r.Context(), presetID, location)
		if err != nil {
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
			return
		}
		if err := store.ValidateHostkeyPrice(plan.PriceMonthly, live.PriceEUR, live.PriceRUB, cfg); err != nil {
			if errors.Is(err, store.ErrLotPriceChanged) {
				writeError(w, http.StatusConflict, "dedicated lot price changed")
				return
			}
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
			return
		}
	}

	osID := strings.TrimSpace(osTemplateID)
	if osID == "" && meta != nil {
		if dists, ok := meta["dist"].([]any); ok && len(dists) > 0 {
			if s, ok := dists[0].(string); ok {
				osID = s
			}
		}
	}
	if osID == "" {
		osID = "219"
	}

	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname != "" && !hostnameRe.MatchString(hostname) {
		writeError(w, http.StatusBadRequest, "invalid hostname")
		return
	}
	rootPassword = strings.TrimSpace(rootPassword)
	if len(rootPassword) < 8 {
		writeError(w, http.StatusBadRequest, "root_password must be at least 8 characters")
		return
	}
	if strings.ContainsAny(rootPassword, "@#") {
		writeError(w, http.StatusBadRequest, "root_password must not contain @ or # (Hostkey restriction)")
		return
	}
	periodMonths = 1
	region = strings.TrimSpace(strings.ToLower(region))
	if region == "" {
		region = plan.Region
	}
	if region == "" {
		region = hostkey.RegionFromLocation(location)
	}
	if extraIPv4Qty > cfg.ExtraIPv4Max {
		writeError(w, http.StatusBadRequest, "invalid extra_ipv4_qty")
		return
	}

	result, err := h.store.CreateOrderWithBilling(r.Context(), store.CreateOrderInput{
		UserID:            userID,
		PlanID:            planID,
		Region:            region,
		Hostname:          hostname,
		RootPassword:      rootPassword,
		OSTemplateID:      osID,
		SoftwareProfileID: "clean",
		PeriodMonths:      periodMonths,
		ProductType:       "dedicated",
		Provider:          "hostkey",
		ExternalProductID: extID,
		ExtraIPv4Qty:      extraIPv4Qty,
	})
	if err != nil {
		log.Printf("CreateHostkeyDedicatedOrder failed user=%s plan=%s: %v", userID, plan.ID, err)
		switch {
		case errors.Is(err, store.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
		case errors.Is(err, store.ErrLotUnavailable):
			writeError(w, http.StatusConflict, "dedicated lot unavailable")
		case errors.Is(err, store.ErrLotPriceChanged):
			writeError(w, http.StatusConflict, "dedicated lot price changed")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing suspended")
		case errors.Is(err, store.ErrInvalidExtraIPv4):
			writeError(w, http.StatusBadRequest, "invalid extra_ipv4_qty")
		default:
			writeError(w, http.StatusInternalServerError, "order failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":            result.OrderID,
		"order_number":        result.OrderNumber,
		"instance_id":         result.InstanceID,
		"plan_id":             plan.ID,
		"region":              region,
		"hostname":            hostname,
		"os_template_id":      osID,
		"software_profile_id": "clean",
		"period_months":       periodMonths,
		"extra_ipv4_qty":      extraIPv4Qty,
		"amount_charged":      result.Amount,
		"status":              result.Status,
		"queued":              false,
		"product_type":        "dedicated",
		"provider":            "hostkey",
		"message":             "Dedicated server order paid; provisioning via Hostkey",
	})

	orderLabel := result.InstanceID
	if result.OrderNumber > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", result.OrderNumber, result.InstanceID)
	}
	alertBody := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nPlan: %s\nHostname: %s\nОС: %s\nProvider: Hostkey\nСумма: %.0f ₽",
		orderLabel, userID, plan.Name, hostname, osID, result.Amount,
	)
	_ = notify.OpsAlert(r.Context(), "🛒 Hostkey Dedicated: новый заказ", alertBody)
	_ = h.store.SetInstanceProviderMeta(r.Context(), result.InstanceID, map[string]any{
		"last_ops_alert_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func hostkeySourceFromHandlerMeta(meta map[string]any) string {
	if meta == nil {
		return "preset"
	}
	if s, ok := meta["source"].(string); ok && s != "" {
		return s
	}
	return "preset"
}

func (h *Handler) InstanceRescue(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	_ = userID
	provider, _, externalID, err := h.store.GetInstanceProvider(r.Context(), instanceID)
	if err != nil || externalID == "" {
		writeError(w, http.StatusConflict, "server not ready")
		return
	}
	if !store.IsDedicatedProvider(provider) {
		writeError(w, http.StatusBadRequest, "rescue only for dedicated")
		return
	}
	if provider == "hostkey" {
		writeError(w, http.StatusBadRequest, "rescue not supported for hostkey dedicated")
		return
	}
	robot := h.robot
	if robot == nil {
		robot = hetznerrobot.NewFromEnv()
	}
	pass, err := robot.EnableRescue(r.Context(), externalID, "linux")
	if err != nil {
		writeError(w, http.StatusBadGateway, "rescue failed")
		return
	}
	_ = robot.Reset(r.Context(), externalID, "hw")
	if pass != "" {
		_ = h.store.UpdateInstanceRootPassword(r.Context(), instanceID, pass)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "password": pass, "mode": "rescue"})
}
