package heleket

import "testing"

func TestVerifyWebhook_preservesFieldOrder(t *testing.T) {
	apiKey := "test-payment-api-key"
	rawWithoutSign := []byte(`{"type":"payment","uuid":"62f88b36-a9d5-4fa6-aa26-e040c3dbf26d","order_id":"97a75bf8eda5cca41ba9d2e104840fcd","amount":"3.00000000","payment_amount":"3.00000000","status":"paid","txid":"someTxidWith\/Slash"}`)
	sign := SignBody(rawWithoutSign, apiKey)
	rawWithSign := []byte(`{"type":"payment","uuid":"62f88b36-a9d5-4fa6-aa26-e040c3dbf26d","order_id":"97a75bf8eda5cca41ba9d2e104840fcd","amount":"3.00000000","payment_amount":"3.00000000","status":"paid","txid":"someTxidWith\/Slash","sign":"` + sign + `"}`)

	body, ok := VerifyWebhook(rawWithSign, apiKey)
	if !ok {
		t.Fatal("expected webhook signature to verify")
	}
	if OrderIDString(body) != "97a75bf8eda5cca41ba9d2e104840fcd" {
		t.Fatalf("unexpected order_id: %q", OrderIDString(body))
	}
}

func TestVerifyWebhook_rejectsTamperedBody(t *testing.T) {
	apiKey := "test-payment-api-key"
	raw := []byte(`{"type":"payment","order_id":"abc","status":"paid","sign":"deadbeef"}`)
	if _, ok := VerifyWebhook(raw, apiKey); ok {
		t.Fatal("expected invalid signature")
	}
}

func TestOrderIDForInvoice(t *testing.T) {
	got := OrderIDForInvoice("550e8400-e29b-41d4-a716-446655440000")
	want := "550e8400e29b41d4a716446655440000"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if len(got) != 32 {
		t.Fatalf("expected 32 chars, got %d", len(got))
	}
}

func TestIsPaidStatus(t *testing.T) {
	if !IsPaidStatus("paid") || !IsPaidStatus("paid_over") {
		t.Fatal("expected paid statuses")
	}
	if IsPaidStatus("confirm_check") {
		t.Fatal("confirm_check must not credit balance")
	}
}

func TestInvoiceAmount_readsMerchantCurrency(t *testing.T) {
	body := map[string]any{
		"amount":          "250.00",
		"payment_amount":  "3.28000000",
		"currency":        "RUB",
		"payer_currency":  "USDT",
	}
	if got := InvoiceAmount(body); got != 250 {
		t.Fatalf("InvoiceAmount = %v, want 250", got)
	}
	if got := PaymentAmount(body); got != 3.28 {
		t.Fatalf("PaymentAmount = %v, want 3.28", got)
	}
}
