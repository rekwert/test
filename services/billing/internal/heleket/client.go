package heleket

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type Client struct {
	merchantID string
	apiKey     string
	baseURL    string
	http       *http.Client
}

type CreateInvoiceRequest struct {
	Amount      string
	Currency    string
	OrderID     string
	URLCallback string
	URLSuccess  string
	URLReturn   string
}

type CreateInvoiceResponse struct {
	UUID          string `json:"uuid"`
	OrderID       string `json:"order_id"`
	Amount        string `json:"amount"`
	PaymentStatus string `json:"payment_status"`
	URL           string `json:"url"`
}

var (
	signFieldTrailing = regexp.MustCompile(`(?is),\s*"sign"\s*:\s*"[^"]*"\s*\}`)
	signFieldLeading  = regexp.MustCompile(`(?is)\{\s*"sign"\s*:\s*"[^"]*"\s*,\s*`)
)

func NewClient(merchantID, apiKey, baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		merchantID: merchantID,
		apiKey:     apiKey,
		baseURL:    baseURL,
		http:       &http.Client{},
	}
}

// OrderIDForInvoice returns a Heleket-compatible order id (max 32 alpha_dash chars).
func OrderIDForInvoice(invoiceID string) string {
	compact := strings.ReplaceAll(invoiceID, "-", "")
	if len(compact) <= 32 {
		return compact
	}
	return compact[:32]
}

func (c *Client) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*CreateInvoiceResponse, error) {
	payload := map[string]string{
		"amount":   req.Amount,
		"currency": req.Currency,
		"order_id": req.OrderID,
	}
	if req.URLCallback != "" {
		payload["url_callback"] = req.URLCallback
	}
	if req.URLSuccess != "" {
		payload["url_success"] = req.URLSuccess
	}
	if req.URLReturn != "" {
		payload["url_return"] = req.URLReturn
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sign := SignBody(body, c.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payment", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("merchant", c.merchantID)
	httpReq.Header.Set("sign", sign)

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("heleket http %d: %s", res.StatusCode, string(raw))
	}

	var envelope struct {
		State  int                   `json:"state"`
		Result CreateInvoiceResponse `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Result.URL != "" {
		return &envelope.Result, nil
	}

	var direct CreateInvoiceResponse
	if err := json.Unmarshal(raw, &direct); err != nil {
		return nil, err
	}
	if direct.URL == "" {
		return nil, fmt.Errorf("heleket create invoice failed: %s", string(raw))
	}
	return &direct, nil
}

func SignBody(body []byte, apiKey string) string {
	encoded := base64.StdEncoding.EncodeToString(body)
	sum := md5.Sum([]byte(encoded + apiKey))
	return hex.EncodeToString(sum[:])
}

func DecodeWebhook(raw []byte) (map[string]any, string, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, "", err
	}
	sign, _ := body["sign"].(string)
	return body, sign, nil
}

// VerifyWebhook validates Heleket webhook signature.
// Heleket signs JSON with sign removed; key order must match the original payload.
func VerifyWebhook(raw []byte, apiKey string) (map[string]any, bool) {
	if gotSign, ok := extractSign(raw); !ok || gotSign == "" || apiKey == "" {
		return nil, false
	} else if VerifyWebhookBody(stripSignField(raw), gotSign, apiKey) {
		body, err := decodeWebhookMap(raw)
		if err != nil {
			return nil, false
		}
		return body, true
	}
	return nil, false
}

func VerifyWebhookBody(payload []byte, gotSign, apiKey string) bool {
	if gotSign == "" || apiKey == "" || len(payload) == 0 {
		return false
	}
	return SignBody(bytes.TrimSpace(payload), apiKey) == gotSign
}

func extractSign(raw []byte) (string, bool) {
	body, err := decodeWebhookMap(raw)
	if err != nil {
		return "", false
	}
	sign, ok := body["sign"].(string)
	return sign, ok && sign != ""
}

func decodeWebhookMap(raw []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body, nil
}

func stripSignField(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return raw
	}
	if stripped := signFieldTrailing.ReplaceAll(raw, []byte("}")); !bytes.Equal(stripped, raw) {
		return bytes.TrimSpace(stripped)
	}
	if stripped := signFieldLeading.ReplaceAll(raw, []byte("{")); !bytes.Equal(stripped, raw) {
		return bytes.TrimSpace(stripped)
	}
	return raw
}

func StatusString(body map[string]any) string {
	if s, ok := body["status"].(string); ok {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

func OrderIDString(body map[string]any) string {
	if s, ok := body["order_id"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func UUIDString(body map[string]any) string {
	if s, ok := body["uuid"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func PaymentAmount(body map[string]any) float64 {
	if v, ok := body["payment_amount"]; ok {
		switch n := v.(type) {
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err == nil {
				return f
			}
		case float64:
			return n
		}
	}
	return 0
}

// InvoiceAmount is the invoice total in the merchant currency (e.g. RUB), not crypto payment_amount.
func InvoiceAmount(body map[string]any) float64 {
	if v, ok := body["amount"]; ok {
		switch n := v.(type) {
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err == nil {
				return f
			}
		case float64:
			return n
		}
	}
	return 0
}

func IsPaidStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "paid_over":
		return true
	default:
		return false
	}
}
