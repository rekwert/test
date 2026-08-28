package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func syncHostkeyCatalog(ctx context.Context, st *store.Store, hk hostkey.Client, cfg hostkey.Config) error {
	cfg = cfg.WithFreshFX(ctx)
	presets, err := hk.ListPresets(ctx)
	if err != nil {
		return fmt.Errorf("hostkey list presets: %w", err)
	}
	osNames := map[int][]string{}
	for _, p := range presets {
		if p.ID <= 0 {
			continue
		}
		images, err := hk.ListOS(ctx, p.ID)
		if err != nil {
			log.Printf("hostkey os list preset %d: %v", p.ID, err)
			continue
		}
		names := make([]string, 0, len(images))
		for _, img := range images {
			names = append(names, img.Name)
		}
		osNames[p.ID] = names
	}
	n, err := st.UpsertHostkeyPresetPlans(ctx, cfg, presets, osNames)
	if err != nil {
		return err
	}
	log.Printf("hostkey sync: %d preset offers", n)

	stocks, err := hk.ListStocks(ctx, "")
	if err != nil {
		log.Printf("hostkey sync stocks: %v", err)
		return nil
	}
	m, err := st.UpsertHostkeyStockPlans(ctx, cfg, stocks)
	if err != nil {
		return err
	}
	log.Printf("hostkey sync: %d stock lots", m)
	return nil
}

func handleHostkeyDedicatedProvision(ctx context.Context, st *store.Store, hk hostkey.Client, payload json.RawMessage) error {
	var data struct {
		InstanceID        string `json:"instance_id"`
		UserID            string `json:"user_id"`
		OSTemplateID      string `json:"os_template_id"`
		ExternalProductID string `json:"external_product_id"`
		RootPassword      string `json:"root_password"`
		Hostname          string `json:"hostname"`
		ExtraIPv4Qty      int    `json:"extra_ipv4_qty"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Printf("hostkey dedicated provision: bad payload: %v", err)
		return nil
	}
	if strings.TrimSpace(data.RootPassword) == "" {
		pwd, pwdErr := st.GetInstanceRootPassword(ctx, data.InstanceID)
		if pwdErr != nil {
			return pwdErr
		}
		data.RootPassword = pwd
	}

	state, err := st.GetInstanceState(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if state != "creating" {
		return nil
	}

	password := strings.TrimSpace(data.RootPassword)
	if password == "" {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("missing root password"))
		return nil
	}
	if strings.ContainsAny(password, "@#") {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("hostkey password must not contain @ or #"))
		return nil
	}

	presetID, location, stockID, source := hostkey.ParseExternalProductID(data.ExternalProductID)
	if source == "stock" && stockID <= 0 {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("missing stock id"))
		return nil
	}
	if source != "stock" && presetID <= 0 {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("missing preset id"))
		return nil
	}

	images, _ := hk.ListOS(ctx, presetID)
	osID := hostkey.ResolveOSID(data.OSTemplateID, images)
	if osID <= 0 {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("no compatible os_id for %q", data.OSTemplateID))
		return nil
	}

	req := hostkey.OrderRequest{
		PresetID:     presetID,
		StockID:      stockID,
		Location:     location,
		OSID:         osID,
		RootPassword: password,
		Hostname:     data.Hostname,
		DeployPeriod: "monthly",
		ExtraIPv4:    data.ExtraIPv4Qty,
	}
	result, err := hk.OrderInstance(ctx, req)
	if err != nil {
		_ = markHostkeyAwaitingManual(ctx, st, data.InstanceID, data.UserID, err)
		return nil
	}

	meta := map[string]any{
		"external_product_id": data.ExternalProductID,
		"os_template_id":      data.OSTemplateID,
		"hostkey_os_id":       osID,
		"location_code":       location,
		"deploy_status":       result.DeployStatus,
		"order_callback":      result.Callback,
	}
	if result.ServerID > 0 {
		meta["server_number"] = result.ServerID
		ext := strconv.Itoa(result.ServerID)
		_ = st.SetInstanceExternalID(ctx, data.InstanceID, ext)
	}
	_ = st.SetInstanceProviderMeta(ctx, data.InstanceID, meta)

	orderNo, _ := st.GetInstanceOrderNumber(ctx, data.InstanceID)
	orderLabel := data.InstanceID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, data.InstanceID)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nHostkey order sent\nServer ID: %d\nStatus: %s\n\nСледите за деплоем в Invapi.",
		orderLabel, data.UserID, result.ServerID, result.DeployStatus,
	)
	_ = sendHostkeyOpsAlert(ctx, st, data.InstanceID, meta, "🛒 Hostkey Dedicated: заказ отправлен", body)
	return nil
}

func processHostkeyDedicatedCreating(ctx context.Context, st *store.Store, hk hostkey.Client) error {
	items, err := st.ListDedicatedCreating(ctx, 10)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Provider != "hostkey" {
			continue
		}
		if err := advanceHostkeyDedicated(ctx, st, hk, item); err != nil {
			log.Printf("hostkey dedicated creating %s: %v", item.ID, err)
			if time.Since(item.UpdatedAt) > 45*time.Minute {
				_ = markHostkeyAwaitingManual(ctx, st, item.ID, item.UserID,
					fmt.Errorf("still creating after 45m: %w", err))
			}
		}
	}
	return nil
}

func advanceHostkeyDedicated(ctx context.Context, st *store.Store, hk hostkey.Client, item store.DedicatedCreating) error {
	var meta map[string]any
	_ = json.Unmarshal(item.ProviderMeta, &meta)
	if meta == nil {
		meta = map[string]any{}
	}

	serverNumber := item.ExternalID
	if serverNumber == "" {
		if n, ok := meta["server_number"].(float64); ok && n > 0 {
			serverNumber = strconv.Itoa(int(n))
		}
	}
	if serverNumber == "" {
		return nil
	}

	srv, err := hk.GetServer(ctx, serverNumber)
	if err != nil {
		return err
	}
	ip := srv.IP
	if ip == "" && len(srv.IPs) > 0 {
		ip = srv.IPs[0]
	}
	if ip == "" {
		if !hostkey.IsReadyStatus(srv.Status) {
			return nil
		}
		return nil
	}

	if len(srv.IPs) > 0 {
		meta["all_ips"] = srv.IPs
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
	}

	password := strings.TrimSpace(item.RootPassword)
	if err := st.CompleteDedicatedProvisioning(ctx, item.ID, serverNumber, ip, password); err != nil {
		return err
	}

	orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
	host := item.Hostname
	if host == "" {
		host = item.ID
	}
	ipForMail := ip
	if raw, ok := meta["all_ips"].([]any); ok && len(raw) > 0 {
		parts := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			ipForMail = strings.Join(parts, ", ")
		}
	}
	if item.UserID != "" {
		includeSSH := !catalog.IsWindowsOS(item.OSTemplateID)
		if err := notify.InstanceReadyEmail(ctx, item.UserID, host, ipForMail, password, orderNo, notify.ReadyDedicated, includeSSH); err != nil {
			log.Printf("hostkey dedicated notify %s: %v", item.ID, err)
		}
	}
	return nil
}

func markHostkeyAwaitingManual(ctx context.Context, st *store.Store, instanceID, userID string, cause error) error {
	log.Printf("hostkey dedicated awaiting manual %s: %v", instanceID, cause)
	meta := map[string]any{
		"awaiting_manual":    true,
		"order_error":        cause.Error(),
		"awaiting_manual_at": time.Now().UTC().Format(time.RFC3339),
	}
	_ = st.SetInstanceProviderMeta(ctx, instanceID, meta)

	orderNo, _ := st.GetInstanceOrderNumber(ctx, instanceID)
	orderLabel := instanceID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, instanceID)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nПричина: %v\n\nHostkey dedicated — нужна ручная обработка.",
		orderLabel, userID, cause,
	)
	_ = sendHostkeyOpsAlert(ctx, st, instanceID, meta, "⏳ Hostkey Dedicated: ручная обработка", body)
	return nil
}

func sendHostkeyOpsAlert(ctx context.Context, st *store.Store, instanceID string, meta map[string]any, title, body string) error {
	if meta == nil {
		meta = map[string]any{}
	}
	if last, ok := meta["last_ops_alert_at"].(string); ok && last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil && time.Since(t) < dedicatedOpsAlertInterval {
			return nil
		}
	}
	if err := notify.OpsAlert(ctx, title, body); err != nil {
		return err
	}
	meta["last_ops_alert_at"] = time.Now().UTC().Format(time.RFC3339)
	_ = st.SetInstanceProviderMeta(ctx, instanceID, meta)
	return nil
}

func routeDedicatedProvision(ctx context.Context, st *store.Store, robot hetznerrobot.Client, hk hostkey.Client, payload json.RawMessage) error {
	var data struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return handleDedicatedProvision(ctx, st, robot, payload)
	}
	provider, _, _, err := st.GetInstanceProvider(ctx, data.InstanceID)
	if err != nil {
		return err
	}
	if provider == "hostkey" {
		return handleHostkeyDedicatedProvision(ctx, st, hk, payload)
	}
	return handleDedicatedProvision(ctx, st, robot, payload)
}
