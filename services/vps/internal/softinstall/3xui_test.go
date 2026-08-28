package softinstall

import "testing"

func TestParseBundleOutput(t *testing.T) {
	raw := `installing...
BUNDLE_JSON:{"profile":"3x-ui","panel":{"url":"http://1.2.3.4:2053","username":"admin","password":"secret"},"vless":{"uri":"vless://x","port":443,"sni":"www.5ka.ru","uuid":"u","public_key":"p","short_id":"s","email":"c"},"hysteria2":{"uri":"hy2://x","port":443,"sni":"www.wikipedia.org","password":"pw","email":"c"}}`
	got, err := parseBundleOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Panel.URL != "http://1.2.3.4:2053" {
		t.Fatalf("panel url = %q", got.Panel.URL)
	}
	if got.VLESS.SNI != "www.5ka.ru" {
		t.Fatalf("vless sni = %q", got.VLESS.SNI)
	}
	if got.Hysteria2.Port != 443 {
		t.Fatalf("hy2 port = %d", got.Hysteria2.Port)
	}
	if got.Hysteria2.SNI != "www.wikipedia.org" {
		t.Fatalf("hy2 sni = %q", got.Hysteria2.SNI)
	}
}

func TestParseBundleOutputMissing(t *testing.T) {
	if _, err := parseBundleOutput("no json here"); err == nil {
		t.Fatal("expected error")
	}
}