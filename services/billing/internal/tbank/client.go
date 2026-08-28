package tbank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	terminalKey string
	password    string
	baseURL     string
	http        *http.Client
}

type InitRequest struct {
	Amount          int64    `json:"Amount"`
	OrderID         string   `json:"OrderId"`
	Description     string   `json:"Description,omitempty"`
	SuccessURL      string   `json:"SuccessURL,omitempty"`
	FailURL         string   `json:"FailURL,omitempty"`
	NotificationURL string   `json:"NotificationURL,omitempty"`
	Receipt         *Receipt `json:"-"`
}

type InitResponse struct {
	Success    bool       `json:"Success"`
	ErrorCode  string     `json:"ErrorCode"`
	Message    string     `json:"Message"`
	Details    string     `json:"Details"`
	PaymentID  flexString `json:"PaymentId"`
	PaymentURL string     `json:"PaymentURL"`
	OrderID    string     `json:"OrderId"`
	Status     string     `json:"Status"`
}

func NewClient(terminalKey, password, baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		terminalKey: terminalKey,
		password:    password,
		baseURL:     baseURL,
		http:        newHTTPClient(),
	}
}

type GetQrRequest struct {
	PaymentID string
	DataType  string // PAYLOAD or IMAGE
}

type GetQrResponse struct {
	Success   bool       `json:"Success"`
	ErrorCode string     `json:"ErrorCode"`
	Message   string     `json:"Message"`
	Details   string     `json:"Details"`
	Data      string     `json:"Data"`
	PaymentID flexString `json:"PaymentId"`
	OrderID   string     `json:"OrderId"`
	Status    string     `json:"Status"`
}

func (c *Client) GetQr(ctx context.Context, req GetQrRequest) (*GetQrResponse, error) {
	dataType := req.DataType
	if dataType == "" {
		dataType = "PAYLOAD"
	}
	payload := map[string]any{
		"TerminalKey": c.terminalKey,
		"PaymentId":   req.PaymentID,
		"DataType":    dataType,
	}
	token, err := Sign(payload, c.password)
	if err != nil {
		return nil, err
	}
	payload["Token"] = token

	var out GetQrResponse
	if err := c.post(ctx, "/GetQr", payload, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		msg := out.Message
		if msg == "" {
			msg = out.Details
		}
		if msg == "" {
			msg = "tbank getqr failed"
		}
		return nil, fmt.Errorf("%s (code %s)", msg, out.ErrorCode)
	}
	return &out, nil
}

func (c *Client) Init(ctx context.Context, req InitRequest) (*InitResponse, error) {
	payload := map[string]any{
		"TerminalKey": c.terminalKey,
		"Amount":      req.Amount,
		"OrderId":     req.OrderID,
	}
	if req.Description != "" {
		payload["Description"] = req.Description
	}
	if req.SuccessURL != "" {
		payload["SuccessURL"] = req.SuccessURL
	}
	if req.FailURL != "" {
		payload["FailURL"] = req.FailURL
	}
	if req.NotificationURL != "" {
		payload["NotificationURL"] = req.NotificationURL
	}
	if req.Receipt != nil {
		if receipt := req.Receipt.toMap(); receipt != nil {
			payload["Receipt"] = receipt
		}
	}

	token, err := Sign(payload, c.password)
	if err != nil {
		return nil, err
	}
	payload["Token"] = token

	var out InitResponse
	if err := c.post(ctx, "/Init", payload, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		msg := out.Message
		if msg == "" {
			msg = out.Details
		}
		if msg == "" {
			msg = "tbank init failed"
		}
		if out.Details != "" && out.Details != msg {
			return nil, fmt.Errorf("%s: %s (code %s)", msg, out.Details, out.ErrorCode)
		}
		return nil, fmt.Errorf("%s (code %s)", msg, out.ErrorCode)
	}
	return &out, nil
}

func (c *Client) post(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("tbank http %d: %s", res.StatusCode, string(raw))
	}
	return json.Unmarshal(raw, out)
}

func PaymentIDString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return fmt.Sprint(v)
	}
}

func StatusString(body map[string]any) string {
	if s, ok := body["Status"].(string); ok {
		return s
	}
	return ""
}

func SuccessBool(body map[string]any) bool {
	switch v := body["Success"].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}
