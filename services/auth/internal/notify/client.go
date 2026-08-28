package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL       string
	serviceToken  string
	httpClient    *http.Client
}

func New(baseURL, serviceToken string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		serviceToken: strings.TrimSpace(serviceToken),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

type SendRequest struct {
	To       string            `json:"to"`
	Template string            `json:"template"`
	Locale   string            `json:"locale"`
	Data     map[string]string `json:"data"`
}

func (c *Client) Send(ctx context.Context, req SendRequest) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("notification client not configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.serviceToken != "" {
		httpReq.Header.Set("X-Service-Token", c.serviceToken)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification send failed: status %d", resp.StatusCode)
	}
	return nil
}

// OpsAlert posts to the staff Telegram alerts channel via notification service.
func (c *Client) OpsAlert(ctx context.Context, title, body string) error {
	if c == nil || c.baseURL == "" {
		log.Printf("ops alert: notification URL not set (%s)", title)
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
	})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/system/ops-alert", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.serviceToken != "" {
		httpReq.Header.Set("X-Service-Token", c.serviceToken)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ops alert status %d", resp.StatusCode)
	}
	return nil
}
