package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const consoleWSTokenTTL = 15 * time.Minute

func (h *Handler) InstanceConsoleWSToken(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	if _, err := h.store.GetInstanceExternalID(r.Context(), instanceID); err != nil {
		writeError(w, http.StatusConflict, "console unavailable")
		return
	}
	token, err := issueConsoleWSToken(h.jwtSecret, userID, instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ws_token":   token,
		"expires_in": int(consoleWSTokenTTL.Seconds()),
	})
}

func issueConsoleWSToken(secret, userID, instanceID string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("missing jwt secret")
	}
	exp := time.Now().Add(consoleWSTokenTTL).Unix()
	payload := userID + "|" + instanceID + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig, nil
}

func verifyConsoleWSToken(secret, token, instanceID string) (userID string, ok bool) {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(secret) == "" {
		return "", false
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	seg := strings.Split(string(raw), "|")
	if len(seg) != 3 || seg[1] != instanceID {
		return "", false
	}
	exp, err := strconv.ParseInt(seg[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(string(raw)))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", false
	}
	return seg[0], true
}
