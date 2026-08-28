package tbank

import "strings"

const (
	MethodSBP     = "sbp"
	MethodTPay    = "tpay"
	MethodSberPay = "sberpay"
	MethodCard    = "card"
)

func NormalizeMethod(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", MethodCard:
		return MethodCard, true
	case MethodSBP, MethodTPay, MethodSberPay:
		return strings.ToLower(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
}

// PaymentURLForMethod appends a hash route understood by the T-Bank hosted form.
func PaymentURLForMethod(baseURL, method string) string {
	if baseURL == "" {
		return baseURL
	}
	switch method {
	case MethodTPay:
		return baseURL + "#t-pay"
	case MethodSberPay:
		return baseURL + "#sber-pay"
	case MethodSBP:
		return baseURL + "#sbp"
	case MethodCard:
		return baseURL + "#card"
	default:
		return baseURL
	}
}
