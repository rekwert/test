package tbank

import "testing"

func TestNewTopupReceipt(t *testing.T) {
	r := NewTopupReceipt("user@example.com", "usn_income", "none", "Пополнение баланса VPS", 10000)
	if r == nil {
		t.Fatal("expected receipt")
	}
	m := r.toMap()
	if m["Email"] != "user@example.com" {
		t.Fatalf("email: %v", m["Email"])
	}
	items, ok := m["Items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items: %#v", m["Items"])
	}
	if items[0]["Amount"] != int64(10000) {
		t.Fatalf("amount: %v", items[0]["Amount"])
	}
}

func TestNewTopupReceiptNilWithoutTaxation(t *testing.T) {
	if NewTopupReceipt("user@example.com", "", "none", "item", 100) != nil {
		t.Fatal("expected nil")
	}
}
