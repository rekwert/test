package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	authURL       string
	vpsURL        string
	billingURL    string
	supportURL    string
	internalToken string
	http          *http.Client
}

func New(authURL, vpsURL, billingURL, supportURL, internalToken string) *Client {
	return &Client{
		authURL:       authURL,
		vpsURL:        vpsURL,
		billingURL:    billingURL,
		supportURL:    supportURL,
		internalToken: internalToken,
		http:          &http.Client{Timeout: 20 * time.Second},
	}
}

type Session struct {
	UserID      string
	Email       string
	AccessToken string
}

func (c *Client) ResolveTelegram(ctx context.Context, telegramID int64) (*Session, error) {
	var profile struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Error  string `json:"error"`
	}
	err := c.do(ctx, http.MethodGet, c.authURL+fmt.Sprintf("/telegram/by-id/%d", telegramID), c.internalToken, "", nil, &profile)
	if err != nil {
		return nil, err
	}
	if profile.UserID == "" {
		return nil, fmt.Errorf("not linked")
	}
	var sess struct {
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	err = c.do(ctx, http.MethodPost, c.authURL+"/telegram/bot-session", c.internalToken, "", map[string]any{
		"telegram_id": telegramID,
	}, &sess)
	if err != nil {
		return nil, err
	}
	if sess.AccessToken == "" {
		if sess.Error != "" {
			return nil, fmt.Errorf("%s", sess.Error)
		}
		return nil, fmt.Errorf("not linked")
	}
	return &Session{UserID: sess.UserID, Email: sess.Email, AccessToken: sess.AccessToken}, nil
}

func (c *Client) RequestLink(ctx context.Context, email string, telegramID int64) error {
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	err := c.do(ctx, http.MethodPost, c.authURL+"/telegram/link/request", c.internalToken, "", map[string]any{
		"email":       email,
		"telegram_id": telegramID,
	}, &out)
	if err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("%s", out.Error)
	}
	return nil
}

func (c *Client) ConfirmLink(ctx context.Context, code string, telegramID int64) (*Session, error) {
	var out struct {
		OK          bool   `json:"ok"`
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	err := c.do(ctx, http.MethodPost, c.authURL+"/telegram/link/confirm", c.internalToken, "", map[string]any{
		"code":        code,
		"telegram_id": telegramID,
	}, &out)
	if err != nil {
		return nil, err
	}
	if !out.OK || out.AccessToken == "" {
		if out.Error != "" {
			return nil, fmt.Errorf("%s", out.Error)
		}
		return nil, fmt.Errorf("confirm failed")
	}
	return &Session{UserID: out.UserID, Email: out.Email, AccessToken: out.AccessToken}, nil
}

func (c *Client) ConfirmWebLink(ctx context.Context, token string, telegramID int64) (*Session, error) {
	var out struct {
		OK          bool   `json:"ok"`
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	err := c.do(ctx, http.MethodPost, c.authURL+"/telegram/link/web/confirm", c.internalToken, "", map[string]any{
		"token":       token,
		"telegram_id": telegramID,
	}, &out)
	if err != nil {
		return nil, err
	}
	if !out.OK || out.AccessToken == "" {
		if out.Error != "" {
			return nil, fmt.Errorf("%s", out.Error)
		}
		return nil, fmt.Errorf("confirm failed")
	}
	return &Session{UserID: out.UserID, Email: out.Email, AccessToken: out.AccessToken}, nil
}

type Plan struct {
	Name         string  `json:"name"`
	Tier         string  `json:"tier"`
	CPU          int     `json:"cpu"`
	RAMMb        int     `json:"ram_mb"`
	DiskGB       int     `json:"disk_gb"`
	PriceMonthly float64 `json:"price_monthly"`
	Active       bool    `json:"active"`
}

func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	var out struct {
		Plans []Plan `json:"plans"`
	}
	if err := c.do(ctx, http.MethodGet, c.vpsURL+"/plans", "", "", nil, &out); err != nil {
		return nil, err
	}
	return out.Plans, nil
}

type Instance struct {
	ID            string  `json:"id"`
	Hostname      string  `json:"hostname"`
	State         string  `json:"state"`
	IPAddress     string  `json:"ip_address"`
	Region        string  `json:"region"`
	PlanName      string  `json:"plan_name"`
	BillingStatus string  `json:"billing_status"`
	NextBillingAt string  `json:"next_billing_at"`
	OrderNumber   *int64  `json:"order_number"`
	CPU           int     `json:"cpu"`
	RAMMb         int     `json:"ram_mb"`
	DiskGB        int     `json:"disk_gb"`
}

func (c *Client) ListInstances(ctx context.Context, token string) ([]Instance, error) {
	var out struct {
		Instances []Instance `json:"instances"`
	}
	if err := c.do(ctx, http.MethodGet, c.vpsURL+"/instances", "", token, nil, &out); err != nil {
		return nil, err
	}
	return out.Instances, nil
}

func (c *Client) GetInstance(ctx context.Context, token, id string) (*Instance, error) {
	var out Instance
	if err := c.do(ctx, http.MethodGet, c.vpsURL+"/instances/"+id, "", token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Reboot(ctx context.Context, token, id string) error {
	return c.do(ctx, http.MethodPost, c.vpsURL+"/instances/"+id+"/reboot", "", token, map[string]any{}, nil)
}

type Credentials struct {
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ip_address"`
	Username     string `json:"username"`
	RootPassword string `json:"root_password"`
	SSHPort      int    `json:"ssh_port"`
}

func (c *Client) Credentials(ctx context.Context, token, id string) (*Credentials, error) {
	var out Credentials
	if err := c.do(ctx, http.MethodGet, c.vpsURL+"/instances/"+id+"/credentials", "", token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type Balance struct {
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

func (c *Client) GetBalance(ctx context.Context, token string) (*Balance, error) {
	var out Balance
	if err := c.do(ctx, http.MethodGet, c.billingURL+"/balance", "", token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Topup(ctx context.Context, token string, amount float64, method string) (paymentURL string, err error) {
	var out struct {
		PaymentURL string `json:"payment_url"`
		Error      string `json:"error"`
	}
	body := map[string]any{"amount": amount}
	if method != "" {
		body["method"] = method
	}
	if err := c.do(ctx, http.MethodPost, c.billingURL+"/topup", "", token, body, &out); err != nil {
		return "", err
	}
	if out.PaymentURL == "" {
		if out.Error != "" {
			return "", fmt.Errorf("%s", out.Error)
		}
		return "", fmt.Errorf("no payment_url")
	}
	return out.PaymentURL, nil
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
}

func (c *Client) PaymentMethods(ctx context.Context, token string) ([]PaymentMethod, error) {
	var out struct {
		Methods []PaymentMethod `json:"methods"`
	}
	if err := c.do(ctx, http.MethodGet, c.billingURL+"/payment-methods", "", token, nil, &out); err != nil {
		return nil, err
	}
	return out.Methods, nil
}

func (c *Client) CreateGuestTicket(ctx context.Context, email, subject, message string, chatID int64) (string, error) {
	var out struct {
		Ticket struct {
			ID string `json:"id"`
		} `json:"ticket"`
	}
	payload := map[string]any{
		"email":            email,
		"subject":          subject,
		"message":          message,
		"category":         "login",
		"telegram_chat_id": chatID,
	}
	if err := c.do(ctx, http.MethodPost, c.supportURL+"/internal/tickets/guest", c.internalToken, "", payload, &out); err != nil {
		return "", err
	}
	return out.Ticket.ID, nil
}

func (c *Client) AddGuestTicketMessage(ctx context.Context, chatID int64, message string) error {
	payload := map[string]any{
		"telegram_chat_id": chatID,
		"message":          message,
	}
	return c.do(ctx, http.MethodPost, c.supportURL+"/internal/tickets/guest/messages", c.internalToken, "", payload, nil)
}

func (c *Client) LookupGuestTicket(ctx context.Context, chatID int64) (string, error) {
	var out struct {
		Ticket *struct {
			ID string `json:"id"`
		} `json:"ticket"`
	}
	url := fmt.Sprintf("%s/internal/tickets/guest?telegram_chat_id=%d", c.supportURL, chatID)
	if err := c.do(ctx, http.MethodGet, url, c.internalToken, "", nil, &out); err != nil {
		return "", err
	}
	if out.Ticket == nil || out.Ticket.ID == "" {
		return "", fmt.Errorf("no open ticket")
	}
	return out.Ticket.ID, nil
}

func (c *Client) do(ctx context.Context, method, rawURL, internalToken, bearer string, payload, dest any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if internalToken != "" {
		req.Header.Set("X-Internal-Token", internalToken)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var er struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &er)
		if er.Error != "" {
			return fmt.Errorf("%s", er.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	if dest == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dest)
}
