package robokassa

import (
	"strings"
	"testing"
)

func TestPaymentSignature(t *testing.T) {
	got := PaymentSignature("demo", "10.00", "1", "password1")
	if len(got) != 32 {
		t.Fatalf("expected md5 hex, got %q", got)
	}
}

func TestPaymentURLTestModeUsesTestPassword(t *testing.T) {
	c := NewClient("demo", "prod1", "prod2", "test1", "test2", true)
	url := c.PaymentURL(PaymentRequest{InvID: 1, Amount: 10, Description: "x"})
	sigProd := PaymentSignature("demo", "10.00", "1", "prod1")
	sigTest := PaymentSignature("demo", "10.00", "1", "test1")
	if strings.Contains(url, sigProd) {
		t.Fatal("test mode should not use production password1")
	}
	if !strings.Contains(url, sigTest) {
		t.Fatal("test mode should use test password1 in signature")
	}
	if !strings.Contains(url, "IsTest=1") {
		t.Fatal("expected IsTest=1 in payment url")
	}
}

func TestVerifyResult(t *testing.T) {
	outSum := "10.00"
	invID := "1"
	password2 := "password2"
	sig := ResultSignature(outSum, invID, password2)
	if !VerifyResult(outSum, invID, sig, password2) {
		t.Fatal("expected valid result signature")
	}
	if VerifyResult(outSum, invID, "bad", password2) {
		t.Fatal("expected invalid signature to fail")
	}
}
