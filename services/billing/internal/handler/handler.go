package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/billing/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/config"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/heleket"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/robokassa"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/tbank"
)

type Handler struct {
	cfg     config.Config
	store   *store.Store
	tbank     *tbank.Client
	heleket   *heleket.Client
	robokassa *robokassa.Client
	secret    string
}

func New(cfg config.Config, st *store.Store, secret string) *Handler {
	var tbankClient *tbank.Client
	if cfg.TBankReady() {
		tbankClient = tbank.NewClient(cfg.TBankTerminalKey, cfg.TBankPassword, cfg.TBankAPIURL)
	}
	var heleketClient *heleket.Client
	if cfg.HeleketReady() {
		heleketClient = heleket.NewClient(cfg.HeleketMerchantID, cfg.HeleketAPIKey, cfg.HeleketAPIURL)
	}
	var robokassaClient *robokassa.Client
	if cfg.RobokassaReady() {
		robokassaClient = robokassa.NewClient(
			cfg.RobokassaMerchantLogin,
			cfg.RobokassaPassword1,
			cfg.RobokassaPassword2,
			cfg.RobokassaTestPassword1,
			cfg.RobokassaTestPassword2,
			cfg.RobokassaTestMode,
		)
	}
	return &Handler{cfg: cfg, store: st, tbank: tbankClient, heleket: heleketClient, robokassa: robokassaClient, secret: secret}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"service":       "billing",
		"mock":          h.cfg.Mock,
		"tbank_ready":     h.cfg.TBankReady(),
		"heleket_ready":   h.cfg.HeleketReady(),
		"robokassa_ready": h.cfg.RobokassaReady(),
	})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) requireCustomer(w http.ResponseWriter, r *http.Request, scopes ...string) (*authn.Identity, bool) {
	id, err := authn.Authenticate(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Api-Key"), h.secret, h.store, scopes...)
	if err != nil {
		if strings.Contains(err.Error(), "missing scope") {
			writeError(w, http.StatusForbidden, "insufficient scope")
			return nil, false
		}
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	return id, true
}

func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireCustomer(w, r, "billing")
	if !ok {
		return
	}
	acc, err := h.store.GetBalance(r.Context(), id.UserID)
	if err != nil {
		log.Printf("balance: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load balance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  acc.UserID,
		"balance":  acc.Balance,
		"currency": acc.Currency,
	})
}

func (h *Handler) PaymentMethods(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireCustomer(w, r, "billing"); !ok {
		return
	}
	methods := []map[string]any{
		{"id": "sbp", "provider": "tbank", "enabled": h.cfg.TBankReady()},
		{"id": "tpay", "provider": "tbank", "enabled": h.cfg.TBankReady()},
		{"id": "sberpay", "provider": "tbank", "enabled": h.cfg.TBankReady()},
		{"id": "card", "provider": "tbank", "enabled": h.cfg.TBankReady()},
		{"id": "card_foreign", "provider": "robokassa", "enabled": h.cfg.RobokassaReady()},
		{"id": "heleket", "provider": "heleket", "enabled": h.cfg.HeleketReady()},
	}
	writeJSON(w, http.StatusOK, map[string]any{"methods": methods})
}

func (h *Handler) Topup(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireVerifiedCustomer(w, r, "billing")
	if !ok {
		return
	}

	var req struct {
		Amount    float64 `json:"amount"`
		Method    string  `json:"method"`
		PromoCode string  `json:"promo_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid amount")
		return
	}
	if req.Amount < 10 {
		writeError(w, http.StatusBadRequest, "minimum topup is 10 RUB")
		return
	}

	method := req.Method
	if method == "" {
		method = tbank.MethodCard
	}

	if method == "heleket" {
		h.topupHeleket(w, r, id.UserID, req.Amount, req.PromoCode)
		return
	}
	if method == "card_foreign" {
		h.topupRobokassa(w, r, id.UserID, req.Amount, req.PromoCode)
		return
	}

	tbankMethod, ok := tbank.NormalizeMethod(method)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported payment method")
		return
	}
	h.topupTBank(w, r, id.UserID, id.Email, req.Amount, tbankMethod, req.PromoCode)
}

func (h *Handler) topupTBank(w http.ResponseWriter, r *http.Request, userID, customerEmail string, amount float64, method, promoCode string) {
	if !h.cfg.TBankReady() || h.tbank == nil {
		writeError(w, http.StatusServiceUnavailable, "payment provider not configured")
		return
	}

	kopecks := int64(math.Round(amount * 100))
	desc := fmt.Sprintf("Cloud-hustle balance top-up %.2f RUB", amount)

	preview, ok := h.resolveTopupPromo(w, r, userID, promoCode, amount)
	if !ok {
		return
	}

	inv, err := h.store.CreateTopupInvoice(r.Context(), userID, amount, desc, "tbank", promoIDPtr(preview), promoBonus(preview))
	if err != nil {
		log.Printf("topup create invoice: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}

	receipt := tbank.NewTopupReceipt(
		customerEmail,
		h.cfg.TBankTaxation,
		h.cfg.TBankReceiptTax,
		h.cfg.TBankReceiptItemName,
		kopecks,
	)
	if receipt == nil || receipt.Email == "" {
		writeError(w, http.StatusBadRequest, "email required for payment receipt")
		return
	}

	initResp, err := h.tbank.Init(r.Context(), tbank.InitRequest{
		Amount:          kopecks,
		OrderID:         inv.ID,
		Description:     desc,
		SuccessURL:      h.cfg.TBankSuccessURL,
		FailURL:         h.cfg.TBankFailURL,
		NotificationURL: h.cfg.TBankNotificationURL,
		Receipt:         receipt,
	})
	if err != nil {
		log.Printf("tbank init: %v", err)
		_ = h.store.MarkInvoiceFailedByID(r.Context(), inv.ID)
		writeError(w, http.StatusBadGateway, "payment initialization failed")
		return
	}

	paymentID := initResp.PaymentID.String()
	if paymentID == "" {
		writeError(w, http.StatusBadGateway, "payment initialization failed")
		return
	}

	paymentURL := tbank.PaymentURLForMethod(initResp.PaymentURL, method)
	qrPayload := ""

	if method == tbank.MethodSBP {
		qrResp, err := h.tbank.GetQr(r.Context(), tbank.GetQrRequest{
			PaymentID: paymentID,
			DataType:  "PAYLOAD",
		})
		if err != nil {
			log.Printf("tbank getqr: %v", err)
		} else if qrResp.Data != "" {
			qrPayload = qrResp.Data
			paymentURL = qrResp.Data
		}
	}

	if err := h.store.AttachPayment(r.Context(), inv.ID, paymentID, paymentURL); err != nil {
		log.Printf("topup attach payment: %v", err)
	}

	out := map[string]any{
		"status":         "pending",
		"invoice_id":     inv.ID,
		"payment_url":    paymentURL,
		"payment_method": method,
		"provider":       "tbank",
		"amount":         amount,
		"currency":       "RUB",
	}
	if qrPayload != "" {
		out["qr_payload"] = qrPayload
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (h *Handler) topupHeleket(w http.ResponseWriter, r *http.Request, userID string, amount float64, promoCode string) {
	if !h.cfg.HeleketReady() || h.heleket == nil {
		writeError(w, http.StatusServiceUnavailable, "crypto payments not configured")
		return
	}

	desc := fmt.Sprintf("Cloud-hustle balance top-up %.2f RUB", amount)

	preview, ok := h.resolveTopupPromo(w, r, userID, promoCode, amount)
	if !ok {
		return
	}

	inv, err := h.store.CreateTopupInvoice(r.Context(), userID, amount, desc, "heleket", promoIDPtr(preview), promoBonus(preview))
	if err != nil {
		log.Printf("heleket create invoice: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}

	currency := h.cfg.HeleketCurrency
	if currency == "" {
		currency = "RUB"
	}

	resp, err := h.heleket.CreateInvoice(r.Context(), heleket.CreateInvoiceRequest{
		Amount:      fmt.Sprintf("%.2f", amount),
		Currency:    currency,
		OrderID:     heleket.OrderIDForInvoice(inv.ID),
		URLCallback: h.cfg.HeleketCallbackURL,
		URLSuccess:  h.cfg.HeleketSuccessURL,
		URLReturn:   h.cfg.HeleketReturnURL,
	})
	if err != nil {
		log.Printf("heleket create: %v", err)
		_ = h.store.MarkInvoiceFailedByID(r.Context(), inv.ID)
		writeError(w, http.StatusBadGateway, "payment initialization failed")
		return
	}

	if err := h.store.AttachPayment(r.Context(), inv.ID, resp.UUID, resp.URL); err != nil {
		log.Printf("heleket attach payment: %v", err)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":         "pending",
		"invoice_id":     inv.ID,
		"payment_url":    resp.URL,
		"payment_method": "heleket",
		"provider":       "heleket",
		"amount":         amount,
		"currency":       currency,
	})
}

func (h *Handler) topupRobokassa(w http.ResponseWriter, r *http.Request, userID string, amount float64, promoCode string) {
	if !h.cfg.RobokassaReady() || h.robokassa == nil {
		writeError(w, http.StatusServiceUnavailable, "international card payments not configured")
		return
	}

	desc := fmt.Sprintf("Cloud-hustle balance top-up %.2f RUB", amount)

	preview, ok := h.resolveTopupPromo(w, r, userID, promoCode, amount)
	if !ok {
		return
	}

	inv, robokassaInvID, err := h.store.CreateRobokassaInvoice(r.Context(), userID, amount, desc, promoIDPtr(preview), promoBonus(preview))
	if err != nil {
		log.Printf("robokassa create invoice: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}

	paymentURL := h.robokassa.PaymentURL(robokassa.PaymentRequest{
		InvID:       robokassaInvID,
		Amount:      amount,
		Description: desc,
		SuccessURL:  h.cfg.RobokassaSuccessURL,
		FailURL:     h.cfg.RobokassaFailURL,
	})

	if err := h.store.AttachPayment(r.Context(), inv.ID, strconv.FormatInt(robokassaInvID, 10), paymentURL); err != nil {
		log.Printf("robokassa attach payment: %v", err)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":         "pending",
		"invoice_id":     inv.ID,
		"payment_url":    paymentURL,
		"payment_method": "card_foreign",
		"provider":       "robokassa",
		"amount":         amount,
		"currency":       "RUB",
	})
}

func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requireCustomer(w, r, "billing")
	if !ok {
		return
	}
	limit, offset := invoiceListParams(r)
	items, total, err := h.store.ListInvoicesPage(r.Context(), id.UserID, limit, offset)
	if err != nil {
		log.Printf("invoices: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load invoices")
		return
	}

	out := make([]map[string]any, 0, len(items))
	for _, inv := range items {
		out = append(out, invoiceToJSON(inv))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoices": out,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func invoiceToJSON(inv store.Invoice) map[string]any {
	row := map[string]any{
		"id":         inv.ID,
		"amount":     inv.Amount,
		"currency":   "RUB",
		"status":     inv.Status,
		"type":       inv.InvoiceType,
		"created_at": inv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if inv.BonusAmount > 0 {
		row["bonus_amount"] = inv.BonusAmount
	}
	if inv.InstanceID != nil && *inv.InstanceID != "" {
		row["instance_id"] = *inv.InstanceID
	}
	if inv.Description != nil {
		row["description"] = *inv.Description
	}
	if inv.Provider != nil {
		row["provider"] = *inv.Provider
	}
	if inv.BalanceAfter != nil {
		row["balance_after"] = *inv.BalanceAfter
	}
	if inv.Region != "" {
		row["region"] = inv.Region
	}
	if inv.PlanTier != "" {
		row["plan_tier"] = inv.PlanTier
	}
	if inv.PlanName != "" {
		row["plan_name"] = inv.PlanName
	}
	if inv.ProductType != "" {
		row["product_type"] = inv.ProductType
	}
	return row
}

func (h *Handler) PaymentWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	body, err := tbank.DecodeWebhook(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if !h.cfg.TBankReady() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	ok, err := tbank.VerifyToken(body, h.cfg.TBankPassword)
	if err != nil || !ok {
		log.Printf("tbank webhook: invalid token payment=%s status=%s keys=%s",
			tbank.PaymentIDString(body["PaymentId"]),
			tbank.StatusString(body),
			tbank.TokenFieldKeys(body),
		)
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	paymentID := tbank.PaymentIDString(body["PaymentId"])
	status := tbank.StatusString(body)
	success := tbank.SuccessBool(body)

	switch {
	case success && status == "CONFIRMED":
		// Credit only after capture (CONFIRMED). AUTHORIZED is two-stage hold — not yet paid.
		credited, err := h.store.MarkInvoicePaid(r.Context(), paymentID, 0)
		if err != nil {
			log.Printf("tbank webhook credit: %v", err)
			writeError(w, http.StatusInternalServerError, "processing failed")
			return
		}
		if credited {
			log.Printf("tbank webhook: credited payment %s", paymentID)
		}
	case status == "REJECTED" || status == "CANCELED" || status == "DEADLINE_EXPIRED":
		_ = h.store.MarkInvoiceFailed(r.Context(), paymentID)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handler) HeleketWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if !h.cfg.HeleketReady() {
		log.Printf("heleket webhook: ignored (HELEKET_ENABLED=false or missing credentials)")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	body, ok := heleket.VerifyWebhook(raw, h.cfg.HeleketAPIKey)
	if !ok {
		preview, _, _ := heleket.DecodeWebhook(raw)
		orderID := ""
		uuid := ""
		if preview != nil {
			orderID = heleket.OrderIDString(preview)
			uuid = heleket.UUIDString(preview)
		}
		log.Printf("heleket webhook: invalid sign order=%s uuid=%s", orderID, uuid)
		writeError(w, http.StatusUnauthorized, "invalid sign")
		return
	}

	orderID := heleket.OrderIDString(body)
	heleketUUID := heleket.UUIDString(body)
	status := heleket.StatusString(body)
	log.Printf("heleket webhook: order=%s uuid=%s status=%s", orderID, heleketUUID, status)

	switch {
	case heleket.IsPaidStatus(status):
		// Heleket payment_amount is in payer/crypto currency, not invoice RUB.
		// Credit the invoice amount we created (finalizeTopupTx uses DB amount when webhookAmount <= 0).
		invoiceAmount := heleket.InvoiceAmount(body)
		credited, err := h.store.MarkInvoicePaidByHeleketRef(r.Context(), orderID, heleketUUID, 0)
		if err != nil {
			log.Printf("heleket webhook credit: %v", err)
			writeError(w, http.StatusInternalServerError, "processing failed")
			return
		}
		if credited {
			log.Printf("heleket webhook: credited order=%s uuid=%s invoice_amount=%.2f webhook_payment_amount=%.2f",
				orderID, heleketUUID, invoiceAmount, heleket.PaymentAmount(body))
		} else {
			log.Printf("heleket webhook: paid status but invoice not found order=%s uuid=%s", orderID, heleketUUID)
		}
	case status == "cancel", status == "fail", status == "system_fail", status == "wrong_amount":
		if orderID != "" {
			_ = h.store.MarkInvoiceFailedByID(r.Context(), orderID)
		}
		if heleketUUID != "" {
			_ = h.store.MarkInvoiceFailed(r.Context(), heleketUUID)
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handler) RobokassaWebhook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form")
		return
	}

	outSum := firstForm(r, "OutSum")
	invID := firstForm(r, "InvId")
	signature := firstForm(r, "SignatureValue")

	if !h.cfg.RobokassaReady() {
		log.Printf("robokassa webhook: ignored (ROBOKASSA_ENABLED=false or missing credentials)")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	isTest := firstForm(r, "IsTest") == "1"
	password2 := h.cfg.RobokassaPassword2
	if isTest {
		if h.cfg.RobokassaTestPassword2 != "" {
			password2 = h.cfg.RobokassaTestPassword2
		}
	} else if h.robokassa != nil {
		password2 = h.robokassa.Password2ForResult(false)
	}

	if !robokassa.VerifyResult(outSum, invID, signature, password2) {
		log.Printf("robokassa webhook: invalid signature inv=%s outsum=%s", invID, outSum)
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	robokassaInvID, err := robokassa.ParseInvID(invID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid inv id")
		return
	}
	amount, _ := robokassa.ParseAmount(outSum)

	credited, err := h.store.MarkInvoicePaidByRobokassaInvID(r.Context(), robokassaInvID, amount)
	if err != nil {
		log.Printf("robokassa webhook credit: %v", err)
		writeError(w, http.StatusInternalServerError, "processing failed")
		return
	}
	if credited {
		log.Printf("robokassa webhook: credited inv=%d amount=%.2f", robokassaInvID, amount)
	} else {
		log.Printf("robokassa webhook: paid but invoice not found inv=%d", robokassaInvID)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK" + invID))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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

func firstForm(r *http.Request, key string) string {
	if v := r.FormValue(key); v != "" {
		return v
	}
	return r.URL.Query().Get(key)
}

type Mock struct {
	secret string
}

func NewMock(secret string) *Mock {
	return &Mock{secret: secret}
}

func (m *Mock) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "billing"})
}

func (m *Mock) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (m *Mock) Balance(w http.ResponseWriter, r *http.Request) {
	userID, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), m.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  userID,
		"balance":  0,
		"currency": "RUB",
		"mock":     true,
	})
}

func (m *Mock) PaymentMethods(w http.ResponseWriter, r *http.Request) {
	if _, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), m.secret); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"methods": []map[string]any{
			{"id": "sbp", "provider": "tbank", "enabled": false},
			{"id": "tpay", "provider": "tbank", "enabled": false},
			{"id": "sberpay", "provider": "tbank", "enabled": false},
			{"id": "card", "provider": "tbank", "enabled": false},
			{"id": "card_foreign", "provider": "robokassa", "enabled": false},
			{"id": "heleket", "provider": "heleket", "enabled": false},
		},
		"mock": true,
	})
}

func (m *Mock) Topup(w http.ResponseWriter, r *http.Request) {
	if _, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), m.secret); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":  "pending",
		"message": "topup mock — payment provider not connected",
		"mock":    true,
	})
}

func (m *Mock) Invoices(w http.ResponseWriter, r *http.Request) {
	if _, err := authn.UserIDFromRequest(r.Header.Get("Authorization"), m.secret); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": []any{}, "mock": true})
}

func (m *Mock) PaymentWebhook(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "mock": "true"})
}

func (m *Mock) HeleketWebhook(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "mock": "true"})
}

func (m *Mock) RobokassaWebhook(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "mock": "true"})
}

func (m *Mock) ValidatePromo(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "promo unavailable in mock mode")
}

func (m *Mock) ApplyPromo(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "promo unavailable in mock mode")
}
