package tbank

import (
	"encoding/json"
	"testing"
)

func TestSignInitExampleFromDocs(t *testing.T) {
	params := map[string]any{
		"TerminalKey": "MerchantTerminalKey",
		"Amount":      19200,
		"OrderId":     "00000",
		"Description": "Подарочная карта на 1000 рублей",
	}
	got, err := Sign(params, "11111111111111")
	if err != nil {
		t.Fatal(err)
	}
	want := "72dd466f8ace0a37a1f740ce5fb78101712bc0665d91a8108c7c8a0ccd426db2"
	if got != want {
		t.Fatalf("token mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestSignNotificationExampleFromDocs(t *testing.T) {
	password := "11111111111"
	want := "1c0964277d0213349243065a0d5b838b8e90d2d25f740d0f2767836e710e80c8"

	t.Run("string_values", func(t *testing.T) {
		params := map[string]any{
			"TerminalKey": "1234567890DEMO",
			"OrderId":     "000000",
			"Success":     "true",
			"Status":      "AUTHORIZED",
			"PaymentId":   "0000000",
			"ErrorCode":   "0",
			"Amount":      "1111",
			"CardId":      "000000",
			"Pan":         "200000******0000",
			"ExpDate":     "1111",
			"RebillId":    "000000",
		}
		got, err := Sign(params, password)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("token mismatch\nwant: %s\ngot:  %s", want, got)
		}
	})

	t.Run("json_number_style", func(t *testing.T) {
		params := map[string]any{
			"TerminalKey": "1234567890DEMO",
			"OrderId":     "000000",
			"Success":     true,
			"Status":      "AUTHORIZED",
			"PaymentId":   json.Number("0000000"),
			"ErrorCode":   "0",
			"Amount":      json.Number("1111"),
			"CardId":      json.Number("000000"),
			"Pan":         "200000******0000",
			"ExpDate":     "1111",
			"RebillId":    "000000",
		}
		got, err := Sign(params, password)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("token mismatch\nwant: %s\ngot:  %s", want, got)
		}
	})
}
