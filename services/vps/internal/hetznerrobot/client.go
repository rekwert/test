package hetznerrobot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	cfg    Config
	client *http.Client
}

func NewHTTP(cfg Config) *HTTPClient {
	return &HTTPClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func NewFromEnv() Client {
	cfg := LoadConfig()
	if !cfg.Enabled {
		return NewMock()
	}
	return NewHTTP(cfg)
}

func (h *HTTPClient) do(ctx context.Context, method, path string, form url.Values) ([]byte, int, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, h.cfg.BaseURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(h.cfg.User, h.cfg.Password)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return raw, resp.StatusCode, fmt.Errorf("hetzner robot %s %s: %d %s", method, path, resp.StatusCode, truncate(string(raw), 300))
	}
	return raw, resp.StatusCode, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (h *HTTPClient) ListMarketProducts(ctx context.Context) ([]MarketProduct, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/order/server_market/product", nil)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Product map[string]any `json:"product"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]MarketProduct, 0, len(rows))
	for _, row := range rows {
		p := parseMarketProduct(row.Product)
		p.Source = "market"
		out = append(out, p)
	}
	return out, nil
}

func (h *HTTPClient) GetMarketProduct(ctx context.Context, productID int) (*MarketProduct, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/order/server_market/product/"+strconv.Itoa(productID), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Product map[string]any `json:"product"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	p := parseMarketProduct(wrap.Product)
	p.Source = "market"
	return &p, nil
}

func (h *HTTPClient) ListServerProducts(ctx context.Context) ([]ServerProduct, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/order/server/product", nil)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Product map[string]any `json:"product"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]ServerProduct, 0, len(rows))
	for _, row := range rows {
		out = append(out, parseServerProduct(row.Product))
	}
	return out, nil
}

// OrderMarket places a server-market order.
// Hetzner requires either password or authorized_key (fingerprint of a key already in Robot).
func (h *HTTPClient) OrderMarket(ctx context.Context, productID int, addons []string, password, authorizedKey string) (*Transaction, error) {
	form := url.Values{}
	form.Set("product_id", strconv.Itoa(productID))
	for _, a := range addons {
		form.Add("addon[]", a)
	}
	if pass := strings.TrimSpace(password); pass != "" {
		form.Set("password", pass)
	}
	key := strings.TrimSpace(authorizedKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("HETZNER_ORDER_AUTHORIZED_KEY"))
	}
	if key != "" {
		form.Add("authorized_key[]", key)
	}
	raw, _, err := h.do(ctx, http.MethodPost, "/order/server_market/transaction", form)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Transaction map[string]any `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	tx := parseTransaction(wrap.Transaction)
	return &tx, nil
}

func (h *HTTPClient) GetTransaction(ctx context.Context, id string) (*Transaction, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/order/server_market/transaction/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Transaction map[string]any `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	tx := parseTransaction(wrap.Transaction)
	return &tx, nil
}

func (h *HTTPClient) OrderServerAddon(ctx context.Context, serverNumber int, productID, reason string) (*Transaction, error) {
	if productID == "" {
		productID = "additional_ipv4"
	}
	if reason == "" {
		reason = "VPS"
	}
	form := url.Values{}
	form.Set("server_number", strconv.Itoa(serverNumber))
	form.Set("product_id", productID)
	form.Set("reason", reason)
	raw, _, err := h.do(ctx, http.MethodPost, "/order/server_addon/transaction", form)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Transaction map[string]any `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	tx := parseTransaction(wrap.Transaction)
	return &tx, nil
}

func (h *HTTPClient) GetAddonTransaction(ctx context.Context, id string) (*Transaction, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/order/server_addon/transaction/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Transaction map[string]any `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	tx := parseTransaction(wrap.Transaction)
	return &tx, nil
}

func (h *HTTPClient) GetServer(ctx context.Context, serverNumber string) (*Server, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/server/"+url.PathEscape(serverNumber), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Server map[string]any `json:"server"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	s := parseServer(wrap.Server)
	return &s, nil
}

func (h *HTTPClient) Reset(ctx context.Context, serverNumber, resetType string) error {
	if resetType == "" {
		resetType = "hw"
	}
	form := url.Values{}
	form.Set("type", resetType)
	_, _, err := h.do(ctx, http.MethodPost, "/reset/"+url.PathEscape(serverNumber), form)
	return err
}

func (h *HTTPClient) EnableRescue(ctx context.Context, serverNumber, osName string) (string, error) {
	if osName == "" {
		osName = "linux"
	}
	form := url.Values{}
	form.Set("os", osName)
	raw, _, err := h.do(ctx, http.MethodPost, "/boot/"+url.PathEscape(serverNumber)+"/rescue", form)
	if err != nil {
		return "", err
	}
	var wrap struct {
		Rescue map[string]any `json:"rescue"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return "", err
	}
	return stringField(wrap.Rescue, "password"), nil
}

func (h *HTTPClient) ActivateLinux(ctx context.Context, serverNumber, dist, lang string) (string, error) {
	if lang == "" {
		lang = "en"
	}
	form := url.Values{}
	form.Set("dist", dist)
	form.Set("lang", lang)
	raw, _, err := h.do(ctx, http.MethodPost, "/boot/"+url.PathEscape(serverNumber)+"/linux", form)
	if err != nil {
		return "", err
	}
	var wrap struct {
		Linux map[string]any `json:"linux"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return "", err
	}
	return stringField(wrap.Linux, "password"), nil
}

func (h *HTTPClient) ListLinuxDist(ctx context.Context, serverNumber string) ([]string, error) {
	raw, _, err := h.do(ctx, http.MethodGet, "/boot/"+url.PathEscape(serverNumber)+"/linux", nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Linux map[string]any `json:"linux"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	return stringSlice(wrap.Linux["dist"]), nil
}

func parseMarketProduct(m map[string]any) MarketProduct {
	return MarketProduct{
		ID:            intField(m, "id"),
		Name:          stringField(m, "name"),
		Description:   stringSlice(m["description"]),
		CPU:           stringField(m, "cpu"),
		CPUBenchmark:  intField(m, "cpu_benchmark"),
		MemoryGB:      intField(m, "memory_size"),
		DiskGB:        intField(m, "hdd_size"),
		DiskCount:     intField(m, "hdd_count"),
		DiskText:      stringField(m, "hdd_text"),
		Datacenter:    stringField(m, "datacenter"),
		NetworkSpeed:  stringField(m, "network_speed"),
		Traffic:       stringField(m, "traffic"),
		PriceEUR:      floatField(m, "price"),
		PriceSetupEUR: floatField(m, "price_setup"),
		FixedPrice:    boolField(m, "fixed_price"),
		Dist:          stringSlice(m["dist"]),
		Addons:        addonIDs(m["orderable_addons"]),
	}
}

func parseServerProduct(m map[string]any) ServerProduct {
	id := stringField(m, "id")
	if id == "" {
		id = strconv.Itoa(intField(m, "id"))
	}
	mem := intField(m, "memory_size")
	if mem == 0 {
		mem = intField(m, "memory")
	}
	return ServerProduct{
		ID:            id,
		Name:          stringField(m, "name"),
		Description:   stringSlice(m["description"]),
		CPU:           firstString(stringSlice(m["description"])),
		MemoryGB:      mem,
		DiskGB:        intField(m, "disk_size"),
		Datacenter:    firstString(stringSlice(m["location"])),
		PriceEUR:      floatField(m, "price"),
		PriceSetupEUR: floatField(m, "price_setup"),
		Dist:          stringSlice(m["dist"]),
		Location:      stringSlice(m["location"]),
	}
}

func parseTransaction(m map[string]any) Transaction {
	tx := Transaction{
		ID:     stringField(m, "id"),
		Status: stringField(m, "status"),
	}
	if v, ok := m["server_number"]; ok && v != nil {
		n := intField(m, "server_number")
		if n > 0 {
			tx.ServerNumber = &n
		}
	}
	if s := stringField(m, "server_ip"); s != "" {
		tx.ServerIP = &s
	}
	if prod, ok := m["product"].(map[string]any); ok {
		tx.ProductID = intField(prod, "id")
		tx.ProductName = stringField(prod, "name")
	}
	return tx
}

func parseServer(m map[string]any) Server {
	s := Server{
		Number:    intField(m, "server_number"),
		Name:      stringField(m, "server_name"),
		IP:        stringField(m, "server_ip"),
		IPv6Net:   stringField(m, "server_ipv6_net"),
		Product:   stringField(m, "product"),
		DC:        stringField(m, "dc"),
		Status:    stringField(m, "status"),
		Cancelled: boolField(m, "cancelled"),
	}
	s.IPs = stringSlice(m["ip"])
	if s.IP != "" {
		found := false
		for _, ip := range s.IPs {
			if ip == s.IP {
				found = true
				break
			}
		}
		if !found {
			s.IPs = append([]string{s.IP}, s.IPs...)
		}
	}
	return s
}

func addonIDs(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id := stringField(m, "id"); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func floatField(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return n
	case json.Number:
		n, _ := t.Float64()
		return n
	default:
		return 0
	}
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func stringSlice(raw any) []string {
	switch t := raw.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return t
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{strings.TrimSpace(t)}
	default:
		return nil
	}
}

func firstString(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
