package staffcookie

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sign returns an HMAC-signed staff cookie value for userID (TTL matches refresh cookie).
func Sign(secret, userID string, maxAgeSec int) (string, error) {
	secret = strings.TrimSpace(secret)
	userID = strings.TrimSpace(userID)
	if secret == "" || userID == "" {
		return "", fmt.Errorf("missing staff cookie inputs")
	}
	if maxAgeSec <= 0 {
		maxAgeSec = 86400 * 30
	}
	exp := time.Now().UTC().Add(time.Duration(maxAgeSec) * time.Second).Unix()
	payload := userID + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig, nil
}

// Valid reports whether cookieValue is a valid HMAC-signed staff token.
func Valid(secret, cookieValue string) bool {
	cookieValue = strings.TrimSpace(cookieValue)
	if secret == "" || cookieValue == "" {
		return false
	}
	parts := strings.SplitN(cookieValue, ".", 2)
	if len(parts) != 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload := string(raw)
	seg := strings.SplitN(payload, "|", 2)
	if len(seg) != 2 {
		return false
	}
	expUnix, err := strconv.ParseInt(seg[1], 10, 64)
	if err != nil || time.Now().UTC().Unix() > expUnix {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}
