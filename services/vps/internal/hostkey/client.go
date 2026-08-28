package hostkey

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPClient struct {
	cfg    Config
	client *http.Client
	mu     sync.Mutex
	// Session token from auth/login (not the API key itself).
	session    string
	sessionExp time.Time
}

func NewHTTP(cfg Config) *HTTPClient {
	return &HTTPClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
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

func (h *HTTPClient) do(ctx context.Context, module, action string, extra url.Values) (map[string]any, error) {
	sess, err := h.ensureSession(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("action", action)
	form.Set("token", sess)
	for k, vals := range extra {
		for _, v := range vals {
			form.Add(k, v)
		}
	}
	return h.post(ctx, module, form)
}

// ensureSession exchanges HOSTKEY_API_TOKEN (API key) for a short-lived session token.
func (h *HTTPClient) ensureSession(ctx context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != "" && time.Now().Before(h.sessionExp.Add(-2*time.Minute)) {
		return h.session, nil
	}
	form := url.Values{}
	form.Set("action", "login")
	form.Set("key", h.cfg.Token)
	out, err := h.post(ctx, "auth.php", form)
	if err != nil {
		return "", fmt.Errorf("hostkey auth/login: %w", err)
	}
	auth := loginPayload(out)
	token := stringField(auth, "token")
	if token == "" {
		return "", fmt.Errorf("hostkey auth/login: empty token (check API key, «Любой» scope, and HOSTKEY_INVAPI_URL: RU accounts use https://invapi.hostkey.ru)")
	}
	h.session = token
	h.sessionExp = time.Now().Add(time.Hour)
	if exp := intField(auth, "token_expire"); exp > 0 {
		h.sessionExp = time.Unix(int64(exp), 0)
	}
	return h.session, nil
}

// loginPayload normalizes auth/login responses (.com returns flat fields; .ru nests them under result).
func loginPayload(m map[string]any) map[string]any {
	if stringField(m, "token") != "" {
		return m
	}
	if res, ok := m["result"].(map[string]any); ok && stringField(res, "token") != "" {
		return res
	}
	return m
}

func (h *HTTPClient) post(ctx context.Context, module string, form url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.BaseURL+"/"+module, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("hostkey %s: parse: %w (%s)", module, err, truncate(string(raw), 200))
	}
	if err := apiError(out); err != nil {
		return nil, err
	}
	return out, nil
}

func apiError(m map[string]any) error {
	if m == nil {
		return fmt.Errorf("hostkey: empty response")
	}
	if auth := loginPayload(m); stringField(auth, "token") != "" && stringField(auth, "role") != "" {
		return nil // auth/login success
	}
	switch r := m["result"].(type) {
	case string:
		if strings.EqualFold(r, "OK") {
			return nil
		}
	case float64:
		if int(r) == 0 || int(r) == 1 {
			return nil
		}
	}
	if errMsg, _ := m["error"].(string); errMsg != "" {
		return fmt.Errorf("hostkey: %s", errMsg)
	}
	if msg, _ := m["message"].(string); msg != "" {
		return fmt.Errorf("hostkey: %s", msg)
	}
	if code, ok := m["code"].(float64); ok && code < 0 {
		return fmt.Errorf("hostkey: code %.0f", code)
	}
	if r, ok := m["result"].(float64); ok && r < 0 {
		if errMsg, _ := m["error"].(string); errMsg != "" {
			return fmt.Errorf("hostkey: %s", errMsg)
		}
		return fmt.Errorf("hostkey: result %.0f", r)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (h *HTTPClient) ListPresets(ctx context.Context) ([]Preset, error) {
	out, err := h.do(ctx, "presets.php", "list", nil)
	if err != nil {
		return nil, err
	}
	raw, _ := out["presets"].([]any)
	items := make([]Preset, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		p := parsePreset(m)
		if p.Virtual {
			continue
		}
		if !isDedicatedPreset(p) {
			continue
		}
		items = append(items, p)
	}
	return items, nil
}

func isDedicatedPreset(p Preset) bool {
	st := strings.ToLower(p.ServerType)
	if strings.Contains(st, "dedicated") || strings.Contains(st, "bare") {
		return true
	}
	if p.Tags["bm"] == "true" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(p.Name), "bm.")
}

func (h *HTTPClient) GetPreset(ctx context.Context, presetID int, location string) (*PresetOffer, error) {
	presets, err := h.ListPresets(ctx)
	if err != nil {
		return nil, err
	}
	loc := strings.ToUpper(strings.TrimSpace(location))
	for _, p := range presets {
		if p.ID != presetID {
			continue
		}
		offer := presetOfferAtLocation(p, loc)
		if offer == nil {
			return nil, fmt.Errorf("hostkey: preset %d not available in %s", presetID, loc)
		}
		return offer, nil
	}
	return nil, fmt.Errorf("hostkey: preset %d not found", presetID)
}

func presetOfferAtLocation(p Preset, loc string) *PresetOffer {
	loc = strings.ToUpper(strings.TrimSpace(loc))
	if loc == "" {
		return nil
	}
	found := false
	for _, l := range EffectivePresetLocations(p) {
		if strings.EqualFold(l, loc) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	priceEUR, priceRUB := p.MonthlyEUR, p.MonthlyRUB
	if lp, ok := p.PriceByLoc[loc]; ok {
		if lp.EUR > 0 {
			priceEUR = lp.EUR
		}
		if lp.RUB > 0 {
			priceRUB = lp.RUB
		}
	}
	if priceEUR <= 0 && priceRUB <= 0 {
		return nil
	}
	cp := p
	return &PresetOffer{
		Preset:   cp,
		Location: loc,
		PriceEUR: priceEUR,
		PriceRUB: priceRUB,
	}
}

func (h *HTTPClient) ListStocks(ctx context.Context, location string) ([]StockServer, error) {
	extra := url.Values{}
	if loc := strings.TrimSpace(location); loc != "" && !strings.EqualFold(loc, "all") {
		extra.Set("location", strings.ToUpper(loc))
	}
	out, err := h.do(ctx, "stocks.php", "list", extra)
	if err != nil {
		return nil, err
	}
	raw, _ := out["servers"].([]any)
	items := make([]StockServer, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		s := parseStock(m)
		if s.ID > 0 {
			items = append(items, s)
		}
	}
	return items, nil
}

func (h *HTTPClient) ListOS(ctx context.Context, presetID int) ([]OSImage, error) {
	extra := url.Values{}
	if presetID > 0 {
		extra.Set("preset", strconv.Itoa(presetID))
	}
	out, err := h.do(ctx, "os.php", "list", extra)
	if err != nil {
		return nil, err
	}
	raw, _ := out["os_list"].([]any)
	items := make([]OSImage, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := intField(m, "id")
		name := stringField(m, "name")
		if id <= 0 || name == "" {
			continue
		}
		if !osSupportsBM(m) {
			continue
		}
		items = append(items, OSImage{ID: id, Name: name})
	}
	return items, nil
}

func osSupportsBM(m map[string]any) bool {
	tags, _ := m["tags"].([]any)
	for _, t := range tags {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if stringField(tm, "tag") == "bm" && stringField(tm, "value") == "true" {
			return true
		}
	}
	return false
}

func (h *HTTPClient) OrderInstance(ctx context.Context, req OrderRequest) (*OrderResult, error) {
	form := url.Values{}
	if req.StockID > 0 {
		form.Set("id", strconv.Itoa(req.StockID))
	} else if req.PresetID > 0 {
		form.Set("preset", strconv.Itoa(req.PresetID))
	} else {
		return nil, fmt.Errorf("hostkey: preset or stock id required")
	}
	if loc := strings.TrimSpace(req.Location); loc != "" {
		form.Set("location_name", strings.ToUpper(loc))
	}
	if req.OSID > 0 {
		form.Set("os_id", strconv.Itoa(req.OSID))
	}
	form.Set("root_pass", req.RootPassword)
	if hn := strings.TrimSpace(req.Hostname); hn != "" {
		form.Set("hostname", hn)
	}
	period := strings.TrimSpace(req.DeployPeriod)
	if period == "" {
		period = "monthly"
	}
	form.Set("deploy_period", period)
	form.Set("deploy_notify", "0")
	if req.ExtraIPv4 > 0 {
		form.Set("ipv4_amount", strconv.Itoa(1+req.ExtraIPv4))
	}
	if key := strings.TrimSpace(req.SSHKey); key != "" {
		form.Set("ssh_key", key)
	}
	out, err := h.do(ctx, "eq.php", "order_instance", form)
	if err != nil {
		return nil, err
	}
	return parseOrderResult(out), nil
}

func (h *HTTPClient) GetServer(ctx context.Context, serverID string) (*Server, error) {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(serverID))
	out, err := h.do(ctx, "eq.php", "show", form)
	if err != nil {
		// Fallback to search by id
		return h.getServerSearch(ctx, serverID)
	}
	return parseServerShow(out), nil
}

func (h *HTTPClient) getServerSearch(ctx context.Context, serverID string) (*Server, error) {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(serverID))
	out, err := h.do(ctx, "eq.php", "search", form)
	if err != nil {
		return nil, err
	}
	servers, _ := out["servers"].([]any)
	for _, item := range servers {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strconv.Itoa(intField(m, "id")) == strings.TrimSpace(serverID) {
			return parseServerSearch(m), nil
		}
	}
	return nil, fmt.Errorf("hostkey: server %s not found", serverID)
}

func (h *HTTPClient) Reboot(ctx context.Context, serverID string) error {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(serverID))
	_, err := h.do(ctx, "eq.php", "reboot", form)
	return err
}

func (h *HTTPClient) PowerOn(ctx context.Context, serverID string) error {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(serverID))
	_, err := h.do(ctx, "eq.php", "on", form)
	return err
}

func (h *HTTPClient) PowerOff(ctx context.Context, serverID string) error {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(serverID))
	_, err := h.do(ctx, "eq.php", "off", form)
	return err
}

func (h *HTTPClient) Reinstall(ctx context.Context, req ReinstallRequest) (*OrderResult, error) {
	form := url.Values{}
	form.Set("id", strings.TrimSpace(req.ServerID))
	if loc := strings.TrimSpace(req.Location); loc != "" {
		form.Set("location_name", strings.ToUpper(loc))
	}
	if req.OSID > 0 {
		form.Set("os_id", strconv.Itoa(req.OSID))
	}
	form.Set("root_pass", req.RootPassword)
	if hn := strings.TrimSpace(req.Hostname); hn != "" {
		form.Set("hostname", hn)
	}
	form.Set("deploy_notify", "0")
	if key := strings.TrimSpace(req.SSHKey); key != "" {
		form.Set("ssh_key", key)
	}
	out, err := h.do(ctx, "eq.php", "order_instance", form)
	if err != nil {
		return nil, err
	}
	return parseOrderResult(out), nil
}

func parsePreset(m map[string]any) Preset {
	p := Preset{
		ID:          intField(m, "id"),
		Name:        stringField(m, "name"),
		Description: stringField(m, "description"),
		CPU:         intField(m, "cpu"),
		RAMGB:       intField(m, "ram"),
		HDD:         stringField(m, "hdd"),
		GPU:         stringField(m, "gpu"),
		ServerType:  stringField(m, "server_type"),
		Virtual:     intField(m, "virtual") == 1,
		MonthlyEUR:  floatField(m, "monthly_com"),
		MonthlyRUB:  floatField(m, "monthly_ru"),
		Available:   intField(m, "available"),
		PriceByLoc:  map[string]LocationPrice{},
		Tags:        map[string]string{},
	}
	if locs := stringField(m, "locations"); locs != "" {
		for _, part := range strings.Split(locs, ",") {
			if s := strings.TrimSpace(part); s != "" {
				p.Locations = append(p.Locations, strings.ToUpper(s))
			}
		}
	}
	if pm, ok := m["price"].(map[string]any); ok {
		for loc, raw := range pm {
			lm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			lp := LocationPrice{
				EUR: floatField(lm, "EUR"),
				RUB: floatField(lm, "RUR"),
				USD: floatField(lm, "USD"),
			}
			if lp.RUB <= 0 {
				lp.RUB = floatField(lm, "RUB")
			}
			p.PriceByLoc[strings.ToUpper(loc)] = lp
		}
	}
	if tags, ok := m["tags"].([]any); ok {
		for _, t := range tags {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			tag := stringField(tm, "tag")
			val := stringField(tm, "value")
			if tag != "" {
				p.Tags[tag] = val
				if tag == "web_cpu_info" && val == "" {
					p.Tags[tag] = stringField(tm, "extra")
				}
			}
			if tag == "web_cpu_info" {
				extra := stringField(tm, "extra")
				if extra != "" {
					p.Tags["cpu_info"] = extra
				}
			}
		}
	}
	return p
}

func parseStock(m map[string]any) StockServer {
	s := StockServer{
		ID:          intField(m, "id"),
		Name:        stringField(m, "name"),
		Location:    strings.ToUpper(stringField(m, "location")),
		Description: stringField(m, "description"),
		CPU:         stringField(m, "cpu"),
		RAMGB:       intField(m, "ram"),
		DiskGB:      intField(m, "disk"),
		Status:      stringField(m, "status"),
	}
	if specs, ok := m["specs"].(map[string]any); ok {
		if s.CPU == "" {
			s.CPU = stringField(specs, "cpu")
		}
		if s.RAMGB <= 0 {
			s.RAMGB = intField(specs, "ram")
		}
		if s.DiskGB <= 0 {
			s.DiskGB = intField(specs, "disk")
		}
	}
	if price, ok := m["price"].(map[string]any); ok {
		s.PriceEUR = floatField(price, "EUR")
		s.PriceRUB = floatField(price, "RUR")
		if s.PriceRUB <= 0 {
			s.PriceRUB = floatField(price, "RUB")
		}
	}
	return s
}

func parseOrderResult(m map[string]any) *OrderResult {
	res := &OrderResult{
		Callback:     stringField(m, "callback"),
		DeployStatus: stringField(m, "deploy_status"),
		Status:       stringField(m, "status"),
	}
	res.ServerID = intField(m, "id")
	return res
}

func parseServerShow(m map[string]any) *Server {
	s := &Server{}
	if sd, ok := m["server_data"].(map[string]any); ok {
		s.ID = intField(sd, "id")
		s.Hostname = stringField(sd, "hostname")
		s.Status = stringField(sd, "status")
	}
	s.IPs = parseIPList(m["IP"])
	if len(s.IPs) == 0 {
		s.IPs = parseIPList(m["ip"])
	}
	if len(s.IPs) > 0 {
		s.IP = s.IPs[0]
	}
	if loc, ok := m["location"].(map[string]any); ok {
		s.Location = stringField(loc, "dc_name")
	}
	return s
}

func parseServerSearch(m map[string]any) *Server {
	s := &Server{
		ID:       intField(m, "id"),
		Hostname: stringField(m, "hostname"),
		Status:   stringField(m, "status"),
	}
	s.IPs = parseIPList(m["ip"])
	if len(s.IPs) == 0 {
		s.IPs = parseIPList(m["IP"])
	}
	if len(s.IPs) > 0 {
		s.IP = s.IPs[0]
	}
	if loc, ok := m["location"].(map[string]any); ok {
		if dc := stringField(loc, "dc_name"); dc != "" {
			s.Location = dc
		}
	}
	return s
}

func parseIPList(raw any) []string {
	switch t := raw.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			switch v := item.(type) {
			case string:
				if ip := strings.TrimSpace(v); ip != "" {
					out = append(out, ip)
				}
			case map[string]any:
				if ip := stringField(v, "IP"); ip != "" {
					out = append(out, ip)
				}
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
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
	default:
		return 0
	}
}

// RegionFromLocation maps Hostkey location codes to portal region ids.
func RegionFromLocation(loc string) string {
	switch strings.ToUpper(strings.TrimSpace(loc)) {
	case "NL":
		return "nl"
	case "DE":
		return "de"
	case "FI":
		return "fi"
	case "UK":
		return "gb"
	case "US":
		return "us"
	case "FR":
		return "fr"
	case "ES":
		return "es"
	case "PL":
		return "pl"
	case "RU":
		return "ru"
	case "IS":
		return "is"
	case "TR":
		return "tr"
	case "IT":
		return "it"
	case "CH":
		return "ch"
	default:
		return strings.ToLower(loc)
	}
}

// ParseExternalProductID splits "119:DE" into preset 119 and location DE.
func ParseExternalProductID(ext string) (presetID int, location string, stockID int, source string) {
	ext = strings.TrimSpace(ext)
	if strings.HasPrefix(ext, "stock:") {
		stockID, _ = strconv.Atoi(strings.TrimPrefix(ext, "stock:"))
		return 0, "", stockID, "stock"
	}
	parts := strings.SplitN(ext, ":", 2)
	if len(parts) == 2 {
		presetID, _ = strconv.Atoi(parts[0])
		return presetID, strings.ToUpper(parts[1]), 0, "preset"
	}
	presetID, _ = strconv.Atoi(ext)
	return presetID, "", 0, "preset"
}

func FormatExternalProductID(presetID int, location string) string {
	if location != "" {
		return fmt.Sprintf("%d:%s", presetID, strings.ToUpper(location))
	}
	return strconv.Itoa(presetID)
}

func FormatStockExternalProductID(stockID int) string {
	return fmt.Sprintf("stock:%d", stockID)
}

// ResolveOSID maps a stored os_template_id to numeric Hostkey os_id.
func ResolveOSID(osTemplateID string, images []OSImage) int {
	osTemplateID = strings.TrimSpace(osTemplateID)
	if osTemplateID == "" {
		return defaultOSID(images)
	}
	if n, err := strconv.Atoi(osTemplateID); err == nil && n > 0 {
		return n
	}
	lower := strings.ToLower(osTemplateID)
	for _, img := range images {
		if strings.EqualFold(img.Name, osTemplateID) {
			return img.ID
		}
		if strings.Contains(strings.ToLower(img.Name), lower) {
			return img.ID
		}
	}
	return defaultOSID(images)
}

func defaultOSID(images []OSImage) int {
	prefs := []string{"ubuntu 24", "ubuntu 22", "debian 12", "debian 11", "rocky 9", "alma 9"}
	for _, pref := range prefs {
		for _, img := range images {
			if strings.Contains(strings.ToLower(img.Name), pref) {
				return img.ID
			}
		}
	}
	if len(images) > 0 {
		return images[0].ID
	}
	return 0
}

func IsReadyStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "rent" || s == "active" || s == "power_on" || s == "power_off"
}
