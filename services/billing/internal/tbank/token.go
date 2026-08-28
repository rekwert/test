package tbank

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

var tokenExclude = map[string]struct{}{
	"Token":   {},
	"Receipt": {},
	"DATA":    {},
	"Data":    {},
	"Shops":   {},
}

func Sign(params map[string]any, password string) (string, error) {
	keys := make([]string, 0, len(params)+1)
	for k, v := range params {
		if !shouldIncludeTokenField(k, v) {
			continue
		}
		keys = append(keys, k)
	}
	keys = append(keys, "Password")
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		if k == "Password" {
			b.WriteString(password)
			continue
		}
		b.WriteString(tokenScalarString(params[k]))
	}

	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", sum), nil
}

func shouldIncludeTokenField(key string, v any) bool {
	if _, skip := tokenExclude[key]; skip {
		return false
	}
	if key == "Password" || v == nil {
		return false
	}
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

func tokenScalarString(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case float64:
		if t == math.Trunc(t) && !math.IsNaN(t) && !math.IsInf(t, 0) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return tokenScalarString(float64(t))
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func VerifyToken(body map[string]any, password string) (bool, error) {
	got, _ := body["Token"].(string)
	if got == "" {
		return false, fmt.Errorf("missing token")
	}
	want, err := Sign(body, password)
	if err != nil {
		return false, err
	}
	return got == want, nil
}

func TokenFieldKeys(body map[string]any) string {
	keys := make([]string, 0, len(body))
	for k, v := range body {
		if shouldIncludeTokenField(k, v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func DecodeWebhook(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	return body, nil
}
