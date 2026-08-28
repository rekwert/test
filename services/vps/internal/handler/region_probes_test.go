package handler

import "testing"

func TestRegionProbeURL(t *testing.T) {
	t.Setenv("REGION_PROBE_URLS", "nl=https://panel.example.com/ping,fi=https://fi.example.com/ping")

	got := regionProbeURL("nl", "10.0.0.1")
	if got != "https://panel.example.com/ping" {
		t.Fatalf("env override: got %q", got)
	}
	got = regionProbeURL("de", "185.84.224.84")
	if got != "" {
		t.Fatalf("vf_ip must not be exposed as probe_url: got %q", got)
	}
}

func TestParseRegionProbeMap(t *testing.T) {
	m := parseRegionProbeMap(" nl=https://a.test/ , de=https://b.test/x ")
	if m["nl"] != "https://a.test/" || m["de"] != "https://b.test/x" {
		t.Fatalf("unexpected map: %#v", m)
	}
}
