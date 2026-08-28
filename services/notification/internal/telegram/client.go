package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client sends outbound messages via Bot API (no polling).
type Client struct {
	token      string
	httpClient *http.Client
	base       string
}

func New(token string) *Client {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		base:       "https://api.telegram.org/bot" + token,
	}
}

// SendText sends an HTML message (title + body) to a chat.
func (c *Client) SendText(ctx context.Context, chatID int64, title, body string) error {
	if c == nil || chatID == 0 {
		return nil
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	var text string
	if title != "" {
		text = "<b>" + html.EscapeString(title) + "</b>"
		if body != "" {
			text += "\n\n" + html.EscapeString(body)
		}
	} else {
		text = html.EscapeString(body)
	}
	if text == "" {
		return nil
	}
	// Telegram hard limit ~4096; keep headroom for markup.
	if len(text) > 4000 {
		text = text[:3997] + "..."
	}

	payload, err := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("telegram sendMessage: %s", out.Description)
	}
	return nil
}
