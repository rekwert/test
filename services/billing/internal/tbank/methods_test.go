package tbank

import "testing"

func TestPaymentURLForMethod(t *testing.T) {
	base := "https://securepayments.tinkoff.ru/abc"
	tests := map[string]string{
		MethodTPay:    base + "#t-pay",
		MethodSberPay: base + "#sber-pay",
		MethodSBP:     base + "#sbp",
		MethodCard:    base + "#card",
	}
	for method, want := range tests {
		if got := PaymentURLForMethod(base, method); got != want {
			t.Fatalf("%s: want %q got %q", method, want, got)
		}
	}
}

func TestNormalizeMethod(t *testing.T) {
	got, ok := NormalizeMethod("SBP")
	if !ok || got != MethodSBP {
		t.Fatalf("normalize sbp: %q %v", got, ok)
	}
	_, ok = NormalizeMethod("heleket")
	if ok {
		t.Fatal("heleket should not normalize as tbank method")
	}
}
