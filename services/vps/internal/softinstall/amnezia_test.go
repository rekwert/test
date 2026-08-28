package softinstall

import "testing"

func TestParseAmneziaBundleOutput(t *testing.T) {
	raw := `installing...
BUNDLE_JSON:{"profile":"amnezia","amnezia":{"vpn_uri":"vpn://abc123","client_name":"default","port":443}}`
	got, err := parseBundleOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "amnezia" {
		t.Fatalf("profile = %q", got.Profile)
	}
	if got.Amnezia.VpnURI != "vpn://abc123" {
		t.Fatalf("vpn uri = %q", got.Amnezia.VpnURI)
	}
}
