package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

const dedicatedOpsAlertInterval = 15 * time.Minute

func syncDedicatedCatalog(ctx context.Context, st *store.Store, robot hetznerrobot.Client, cfg hetznerrobot.Config) error {
	cfg = cfg.WithFreshFX(ctx)
	if rate, at, src := hetznerrobot.CachedEurRub(); rate > 0 {
		log.Printf("dedicated fx: EUR/RUB=%.4f source=%s age=%s", rate, src, time.Since(at).Round(time.Second))
	}
	market, err := robot.ListMarketProducts(ctx)
	if err != nil {
		return fmt.Errorf("list market: %w", err)
	}
	n, err := st.UpsertDedicatedMarketPlans(ctx, cfg, market)
	if err != nil {
		return err
	}
	log.Printf("dedicated sync: %d market lots", n)

	products, err := robot.ListServerProducts(ctx)
	if err != nil {
		log.Printf("dedicated sync regular products: %v", err)
		return nil
	}
	m, err := st.UpsertDedicatedServerProducts(ctx, cfg, products)
	if err != nil {
		return err
	}
	log.Printf("dedicated sync: %d regular products", m)
	return nil
}

func handleDedicatedProvision(ctx context.Context, st *store.Store, robot hetznerrobot.Client, payload json.RawMessage) error {
	var data struct {
		InstanceID        string `json:"instance_id"`
		UserID            string `json:"user_id"`
		OSTemplateID      string `json:"os_template_id"`
		ExternalProductID string `json:"external_product_id"`
		RootPassword      string `json:"root_password"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		// Bad payload: drop outbox event (return nil → MarkOutboxPublished).
		log.Printf("dedicated provision: bad payload: %v", err)
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
		// Already finalized (running/error) — ack outbox, do not retry.
		return nil
	}

	prodID, _ := strconv.Atoi(strings.TrimSpace(data.ExternalProductID))
	if prodID <= 0 {
		_ = markDedicatedAwaitingManual(ctx, st, data.InstanceID, data.UserID, fmt.Errorf("missing external_product_id"))
		return nil
	}

	password := strings.TrimSpace(data.RootPassword)
	authKey := strings.TrimSpace(os.Getenv("HETZNER_ORDER_AUTHORIZED_KEY"))
	if password == "" && authKey == "" {
		_ = markDedicatedAwaitingManual(ctx, st, data.InstanceID, data.UserID,
			fmt.Errorf("missing root password and HETZNER_ORDER_AUTHORIZED_KEY (Robot requires password or authorized_key)"))
		return nil
	}

	addons := []string{"primary_ipv4"}
	tx, err := robot.OrderMarket(ctx, prodID, addons, password, authKey)
	if err != nil {
		// retry without addon if IPv4 not orderable
		tx, err = robot.OrderMarket(ctx, prodID, nil, password, authKey)
	}
	if err != nil {
		// Keep creating for manual provisioning — do not refund / mark error.
		_ = markDedicatedAwaitingManual(ctx, st, data.InstanceID, data.UserID, err)
		return nil
	}

	meta := map[string]any{
		"transaction_id":      tx.ID,
		"transaction_status":  tx.Status,
		"external_product_id": data.ExternalProductID,
		"os_template_id":      data.OSTemplateID,
	}
	if tx.ServerNumber != nil {
		meta["server_number"] = *tx.ServerNumber
	}
	if err := st.SetInstanceProviderMeta(ctx, data.InstanceID, meta); err != nil {
		return err
	}
	if tx.ServerNumber != nil && *tx.ServerNumber > 0 {
		ext := strconv.Itoa(*tx.ServerNumber)
		ip := ""
		if tx.ServerIP != nil {
			ip = *tx.ServerIP
		}
		_ = st.SetInstanceExternalID(ctx, data.InstanceID, ext)
		if ip != "" {
			_ = st.SetInstanceIP(ctx, data.InstanceID, ip)
		}
	}

	orderNo, _ := st.GetInstanceOrderNumber(ctx, data.InstanceID)
	orderLabel := data.InstanceID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, data.InstanceID)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nTransaction: %s\nStatus: %s\n\nАвтозаказ в Robot отправлен. Следите за готовностью / доведите вручную при необходимости.",
		orderLabel, data.UserID, tx.ID, tx.Status,
	)
	_ = sendDedicatedOpsAlert(ctx, st, data.InstanceID, meta, "🛒 Dedicated: заказ в Robot", body)
	return nil
}

func processDedicatedCreating(ctx context.Context, st *store.Store, robot hetznerrobot.Client) error {
	items, err := st.ListDedicatedCreating(ctx, 10)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Provider == "hostkey" {
			continue
		}
		remindDedicatedPending(ctx, st, item)
		if err := advanceDedicated(ctx, st, robot, item); err != nil {
			log.Printf("dedicated creating %s: %v", item.ID, err)
			// Soft timeout: leave creating for ops (no auto-refund). Alert only.
			if time.Since(item.UpdatedAt) > 45*time.Minute {
				_ = markDedicatedAwaitingManual(ctx, st, item.ID, item.UserID,
					fmt.Errorf("still creating after 45m: %w", err))
			}
		}
	}
	return nil
}

// processDedicatedExtraIPs refreshes all_ips for running dedicated servers after addon orders.
func processDedicatedExtraIPs(ctx context.Context, st *store.Store, robot hetznerrobot.Client) error {
	items, err := st.ListDedicatedExtraIPPending(ctx, 10)
	if err != nil {
		return err
	}
	for _, item := range items {
		var meta map[string]any
		_ = json.Unmarshal(item.ProviderMeta, &meta)
		if meta == nil {
			meta = map[string]any{}
		}
		serverNumber := item.ExternalID
		if serverNumber == "" {
			continue
		}
		qty := extraIPv4Qty(meta)
		pollDedicatedAddonTx(ctx, robot, meta)
		ipList := refreshDedicatedIPs(ctx, robot, serverNumber, "")
		if len(ipList) == 0 {
			continue
		}
		meta["all_ips"] = ipList
		haveExtra := len(ipList) - 1
		if haveExtra < 0 {
			haveExtra = 0
		}
		status, _ := meta["extra_ipv4_status"].(string)
		if qty > 0 && haveExtra >= qty {
			meta["extra_ipv4_status"] = "active"
			log.Printf("dedicated extra ipv4 ready %s: %v", item.ID, ipList)
		} else if status == "ordered" || status == "pending" {
			meta["extra_ipv4_status"] = "pending"
		}
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
		if ipList[0] != "" {
			_ = st.SetInstanceIP(ctx, item.ID, ipList[0])
		}
	}
	return nil
}

func markDedicatedAwaitingManual(ctx context.Context, st *store.Store, instanceID, userID string, cause error) error {
	log.Printf("dedicated awaiting manual %s: %v", instanceID, cause)
	var meta map[string]any
	// Merge flag into provider_meta without wiping existing keys.
	meta = map[string]any{
		"awaiting_manual":     true,
		"order_market_error":  cause.Error(),
		"awaiting_manual_at":  time.Now().UTC().Format(time.RFC3339),
	}
	_ = st.SetInstanceProviderMeta(ctx, instanceID, meta)

	orderNo, _ := st.GetInstanceOrderNumber(ctx, instanceID)
	orderLabel := instanceID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, instanceID)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nПричина автозаказа: %v\n\nСтатус у клиента: «Создаётся». Деньги НЕ возвращены.\nПоднимите сервер вручную в Hetzner Robot / админке.",
		orderLabel, userID, cause,
	)
	_ = sendDedicatedOpsAlert(ctx, st, instanceID, meta, "⏳ Dedicated: нужна ручная обработка", body)
	return nil
}

func remindDedicatedPending(ctx context.Context, st *store.Store, item store.DedicatedCreating) {
	var meta map[string]any
	_ = json.Unmarshal(item.ProviderMeta, &meta)
	if meta == nil {
		meta = map[string]any{}
	}

	orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
	orderLabel := item.ID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, item.ID)
	}
	txID, _ := meta["transaction_id"].(string)
	status := "ожидает ручной / авто-провижн"
	if txID != "" {
		status = fmt.Sprintf("transaction=%s", txID)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nHostname: %s\nСостояние: creating (%s)\n\nПока сервер не поднят — напоминание не чаще раза в 15 минут.",
		orderLabel, item.UserID, item.Hostname, status,
	)
	_ = sendDedicatedOpsAlert(ctx, st, item.ID, meta, "⏳ Dedicated: всё ещё creating", body)
}

// sendDedicatedOpsAlert sends at most once per dedicatedOpsAlertInterval per instance.
func sendDedicatedOpsAlert(ctx context.Context, st *store.Store, instanceID string, meta map[string]any, title, body string) error {
	if meta == nil {
		meta = map[string]any{}
	}
	if last, ok := meta["last_ops_alert_at"].(string); ok && last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil && time.Since(t) < dedicatedOpsAlertInterval {
			return nil
		}
	}
	if err := notify.OpsAlert(ctx, title, body); err != nil {
		log.Printf("dedicated ops alert %s: %v", instanceID, err)
		return err
	}
	meta["last_ops_alert_at"] = time.Now().UTC().Format(time.RFC3339)
	_ = st.SetInstanceProviderMeta(ctx, instanceID, meta)
	return nil
}

func advanceDedicated(ctx context.Context, st *store.Store, robot hetznerrobot.Client, item store.DedicatedCreating) error {
	var meta map[string]any
	_ = json.Unmarshal(item.ProviderMeta, &meta)
	if meta == nil {
		meta = map[string]any{}
	}

	txID, _ := meta["transaction_id"].(string)
	serverNumber := item.ExternalID
	if serverNumber == "" {
		if n, ok := meta["server_number"].(float64); ok && n > 0 {
			serverNumber = strconv.Itoa(int(n))
		}
	}

	if serverNumber == "" && txID != "" {
		tx, err := robot.GetTransaction(ctx, txID)
		if err != nil {
			return err
		}
		meta["transaction_status"] = tx.Status
		if tx.ServerNumber != nil && *tx.ServerNumber > 0 {
			serverNumber = strconv.Itoa(*tx.ServerNumber)
			meta["server_number"] = *tx.ServerNumber
			_ = st.SetInstanceExternalID(ctx, item.ID, serverNumber)
		}
		if tx.ServerIP != nil && *tx.ServerIP != "" {
			_ = st.SetInstanceIP(ctx, item.ID, *tx.ServerIP)
		}
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
		if serverNumber == "" {
			status := strings.ToLower(tx.Status)
			if strings.Contains(status, "cancel") || strings.Contains(status, "fail") {
				return failDedicated(ctx, st, item.ID, item.UserID, fmt.Errorf("transaction %s", tx.Status))
			}
			return nil
		}
	}
	if serverNumber == "" {
		return nil
	}

	srv, err := robot.GetServer(ctx, serverNumber)
	if err != nil {
		return err
	}
	ip := srv.IP
	if ip == "" {
		return nil
	}

	osDist := item.OSTemplateID
	if osDist == "" {
		if s, ok := meta["os_template_id"].(string); ok {
			osDist = s
		}
	}
	// Checkout password is sent to OrderMarket; after ActivateLinux/Rescue Hetzner
	// returns the install password that SSH actually uses — prefer that.
	password := strings.TrimSpace(item.RootPassword)
	if _, done := meta["os_activated"].(bool); !done && osDist != "" && !strings.EqualFold(osDist, "rescue") {
		dists, _ := robot.ListLinuxDist(ctx, serverNumber)
		dist := matchDist(osDist, dists)
		if dist == "" {
			dist = osDist
		}
		pass, actErr := robot.ActivateLinux(ctx, serverNumber, dist, "en")
		if actErr != nil {
			log.Printf("dedicated linux activate %s: %v (continuing with rescue)", item.ID, actErr)
			pass, actErr = robot.EnableRescue(ctx, serverNumber, "linux")
			if actErr != nil {
				return actErr
			}
			meta["rescue"] = true
		}
		if pass != "" {
			password = pass
		}
		_ = robot.Reset(ctx, serverNumber, "hw")
		meta["os_activated"] = true
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
	}

	if err := orderExtraIPv4s(ctx, st, robot, item, meta, serverNumber); err != nil {
		log.Printf("dedicated extra ipv4 %s: %v", item.ID, err)
	}

	// Refresh IPs after addon orders (may take a short while on Robot side).
	ipList := refreshDedicatedIPs(ctx, robot, serverNumber, ip)
	if len(ipList) > 0 {
		ip = ipList[0]
		meta["all_ips"] = ipList
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
	}
	wantExtra := extraIPv4Qty(meta)
	if wantExtra > 0 {
		haveExtra := len(ipList) - 1
		if haveExtra < 0 {
			haveExtra = 0
		}
		status, _ := meta["extra_ipv4_status"].(string)
		if status == "ordered" && haveExtra < wantExtra {
			// Brief wait for Robot to attach addons before emailing the client.
			for attempt := 0; attempt < 6 && haveExtra < wantExtra; attempt++ {
				time.Sleep(5 * time.Second)
				pollDedicatedAddonTx(ctx, robot, meta)
				ipList = refreshDedicatedIPs(ctx, robot, serverNumber, ip)
				if len(ipList) > 0 {
					ip = ipList[0]
					meta["all_ips"] = ipList
					haveExtra = len(ipList) - 1
					if haveExtra < 0 {
						haveExtra = 0
					}
					_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
				}
			}
			if haveExtra < wantExtra && status != "pending_manual" && status != "failed" {
				meta["extra_ipv4_status"] = "pending"
				_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
			}
		}
	}

	if err := st.CompleteDedicatedProvisioning(ctx, item.ID, serverNumber, ip, password); err != nil {
		return err
	}
	orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
	host := item.Hostname
	if host == "" {
		host = item.ID
	}
	ipForMail := ip
	if raw, ok := meta["all_ips"].([]string); ok && len(raw) > 0 {
		ipForMail = strings.Join(raw, ", ")
	} else if raw, ok := meta["all_ips"].([]any); ok && len(raw) > 0 {
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
			log.Printf("dedicated notify %s: %v", item.ID, err)
		}
	}
	return nil
}

func extraIPv4Qty(meta map[string]any) int {
	switch v := meta["extra_ipv4_qty"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func refreshDedicatedIPs(ctx context.Context, robot hetznerrobot.Client, serverNumber, fallbackIP string) []string {
	srv, err := robot.GetServer(ctx, serverNumber)
	if err != nil || srv == nil {
		if fallbackIP != "" {
			return []string{fallbackIP}
		}
		return nil
	}
	out := append([]string{}, srv.IPs...)
	if srv.IP != "" {
		found := false
		for _, ip := range out {
			if ip == srv.IP {
				found = true
				break
			}
		}
		if !found {
			out = append([]string{srv.IP}, out...)
		}
	}
	if len(out) == 0 && fallbackIP != "" {
		out = []string{fallbackIP}
	}
	return out
}

func pollDedicatedAddonTx(ctx context.Context, robot hetznerrobot.Client, meta map[string]any) {
	raw, ok := meta["extra_ipv4_tx_ids"].([]any)
	if !ok {
		if ids, ok := meta["extra_ipv4_tx_ids"].([]string); ok {
			for _, id := range ids {
				if id == "" {
					continue
				}
				if _, err := robot.GetAddonTransaction(ctx, id); err != nil {
					log.Printf("dedicated addon tx %s: %v", id, err)
				}
			}
		}
		return
	}
	for _, v := range raw {
		id, _ := v.(string)
		if id == "" {
			continue
		}
		if _, err := robot.GetAddonTransaction(ctx, id); err != nil {
			log.Printf("dedicated addon tx %s: %v", id, err)
		}
	}
}

func orderExtraIPv4s(ctx context.Context, st *store.Store, robot hetznerrobot.Client, item store.DedicatedCreating, meta map[string]any, serverNumber string) error {
	qty := extraIPv4Qty(meta)
	status, _ := meta["extra_ipv4_status"].(string)
	if qty <= 0 || status == "ordered" || status == "none" || status == "active" || status == "pending_manual" {
		return nil
	}
	if status == "failed" {
		return nil
	}

	num, err := strconv.Atoi(serverNumber)
	if err != nil || num <= 0 {
		return fmt.Errorf("invalid server number %q", serverNumber)
	}

	var txIDs []string
	ordered := 0
	var lastErr error
	for i := 0; i < qty; i++ {
		tx, ordErr := robot.OrderServerAddon(ctx, num, "additional_ipv4", "Cloud-hustle dedicated customer")
		if ordErr != nil {
			lastErr = ordErr
			log.Printf("dedicated OrderServerAddon %s #%d: %v", item.ID, i+1, ordErr)
			break
		}
		ordered++
		if tx != nil && tx.ID != "" {
			txIDs = append(txIDs, tx.ID)
		}
	}

	meta["extra_ipv4_tx_ids"] = txIDs
	meta["extra_ipv4_ordered"] = ordered
	if ordered >= qty {
		meta["extra_ipv4_status"] = "ordered"
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
		return nil
	}

	// Partial or total failure: keep server delivery; flag manual IP fulfillment.
	meta["extra_ipv4_status"] = "pending_manual"
	_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)

	charged := 0.0
	switch v := meta["extra_ipv4_charged"].(type) {
	case float64:
		charged = v
	}
	// Do not auto-refund when API cannot order — ops fulfills manually (client already paid).
	orderNo, _ := st.GetInstanceOrderNumber(ctx, item.ID)
	body := fmt.Sprintf(
		"Instance: %s\nOrder: %d\nServer: %s\nЗаказано IP через API: %d/%d\nОшибка: %v\nСтатус: pending_manual\n\nНужна ручная дозаказка IP в Hetzner Robot и внесение адресов в provider_meta.all_ips.",
		item.ID, orderNo, serverNumber, ordered, qty, lastErr,
	)
	_ = charged
	return sendDedicatedOpsAlert(ctx, st, item.ID, meta,
		"Dedicated: доп. IPv4 требуют ручной выдачи",
		body,
	)
}

func failDedicated(ctx context.Context, st *store.Store, instanceID, userID string, cause error) error {
	log.Printf("dedicated fail %s: %v", instanceID, cause)

	ok, err := st.FailProvisioningIfCreating(ctx, instanceID, cause.Error())
	if err != nil {
		return err
	}
	if !ok {
		// Already error/running — no second refund, no spam alert.
		return cause
	}

	refunded := 0.0
	if userID != "" {
		already, _ := st.HasDedicatedFailureRefund(ctx, instanceID)
		if !already {
			if amount, err := st.GetInstanceChargeAmount(ctx, instanceID); err == nil && amount > 0 {
				_ = st.CreditBalance(ctx, userID, amount, fmt.Sprintf("Refund dedicated provision failure %s", instanceID))
				refunded = amount
			}
		}
	}

	orderNo, _ := st.GetInstanceOrderNumber(ctx, instanceID)
	orderLabel := instanceID
	if orderNo > 0 {
		orderLabel = fmt.Sprintf("№%d (%s)", orderNo, instanceID)
	}
	refundNote := "возврат не делали (сумма 0)."
	if refunded > 0 {
		refundNote = fmt.Sprintf("возвращено %.0f ₽ на баланс.", refunded)
	}
	body := fmt.Sprintf(
		"Заказ: %s\nUser: %s\nПричина: %v\n\nНужна ручная обработка в Hetzner Robot / админке.\nКлиенту %s",
		orderLabel,
		userID,
		cause,
		refundNote,
	)
	// Fail alert bypasses 15m throttle once (first transition to error).
	if err := notify.OpsAlert(ctx, "🔥 Dedicated: заказ упал", body); err != nil {
		log.Printf("dedicated fail ops alert %s: %v", instanceID, err)
	}
	meta := map[string]any{
		"last_ops_alert_at": time.Now().UTC().Format(time.RFC3339),
		"fail_reason":       cause.Error(),
	}
	_ = st.SetInstanceProviderMeta(ctx, instanceID, meta)
	return cause
}

func matchDist(want string, available []string) string {
	want = strings.TrimSpace(want)
	if want == "" {
		if len(available) > 0 {
			return available[0]
		}
		return ""
	}
	for _, d := range available {
		if strings.EqualFold(d, want) {
			return d
		}
	}
	lw := strings.ToLower(want)
	for _, d := range available {
		if strings.Contains(strings.ToLower(d), lw) || strings.Contains(lw, strings.ToLower(d)) {
			return d
		}
	}
	// Prefer Ubuntu if requested vaguely
	if strings.Contains(lw, "ubuntu") {
		for _, d := range available {
			if strings.Contains(strings.ToLower(d), "ubuntu") {
				return d
			}
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return want
}

func processDedicatedPriceNotices(ctx context.Context, st *store.Store) error {
	items, err := st.ListDedicatedPriceReview(ctx, 7, 50)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	cfg := hetznerrobot.LoadConfig().WithFreshFX(ctx)
	for _, item := range items {
		if err := reviewDedicatedPrice(ctx, st, cfg, item); err != nil {
			log.Printf("dedicated price review %s: %v", item.ID, err)
		}
	}
	return nil
}

func reviewDedicatedPrice(ctx context.Context, st *store.Store, cfg hetznerrobot.Config, item store.DedicatedPriceReview) error {
	var meta map[string]any
	_ = json.Unmarshal(item.ProviderMeta, &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	var planMeta map[string]any
	_ = json.Unmarshal(item.PlanMeta, &planMeta)

	billingKey := item.NextBillingAt.UTC().Format(time.RFC3339)
	if notifiedFor, _ := meta["price_increase_notified_for"].(string); notifiedFor == billingKey {
		return nil
	}

	priceEUR := floatFromMeta(meta, "price_eur")
	if priceEUR <= 0 {
		priceEUR = floatFromMeta(planMeta, "price_eur")
	}
	if priceEUR <= 0 {
		return nil
	}

	locked := floatFromMeta(meta, "billing_price_rub")
	if locked <= 0 {
		locked = item.PlanPrice
	}
	if locked <= 0 {
		return nil
	}

	// Persist EUR + locked price for older instances that lack them.
	meta["price_eur"] = priceEUR
	if floatFromMeta(meta, "billing_price_rub") <= 0 {
		meta["billing_price_rub"] = locked
	}

	newPrice := cfg.SellPriceRub(priceEUR)
	if newPrice <= locked {
		// Cheaper or same — keep customer price; still backfill meta once.
		_ = st.SetInstanceProviderMeta(ctx, item.ID, meta)
		return nil
	}

	meta["billing_price_rub"] = newPrice
	meta["price_increase_notified_for"] = billingKey
	meta["price_previous_rub"] = locked
	if err := st.SetInstanceProviderMeta(ctx, item.ID, meta); err != nil {
		return err
	}

	daysLeft := int(time.Until(item.NextBillingAt).Hours()/24) + 1
	if daysLeft < 0 {
		daysLeft = 0
	}
	host := item.Hostname
	if host == "" {
		host = item.PlanName
	}
	orderSuffix := ""
	if item.OrderNumber > 0 {
		orderSuffix = fmt.Sprintf(" №%d", item.OrderNumber)
	}
	title := "Изменение цены dedicated" + orderSuffix
	body := fmt.Sprintf(
		"Сервер %s: через ~%d дн. срок аренды заканчивается.\n\nСтоимость продления выросла: было %.0f ₽/мес, станет %.0f ₽/мес (пересчёт по курсу EUR).\n\nЕсли стало бы дешевле — мы оставили бы прежнюю цену. Пополните баланс заранее.",
		host, daysLeft, locked, newPrice,
	)
	if err := notify.User(ctx, item.UserID, title, body, "billing", true); err != nil {
		log.Printf("dedicated price notify %s: %v", item.ID, err)
	}
	log.Printf("dedicated price up %s: %.0f -> %.0f rub (eur=%.2f)", item.ID, locked, newPrice, priceEUR)
	return nil
}

func floatFromMeta(meta map[string]any, key string) float64 {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}
