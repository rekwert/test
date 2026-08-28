package softinstall

import "testing"

func TestParseClaudeCodeBundleOutput(t *testing.T) {
	raw := `installing...
BUNDLE_JSON:{"profile":"claude-code","panel":{"url":"http://1.2.3.4:7681/","username":"dev","password":"secret"},"claude":{"login_hint":"claude login","start_command":"claude"}}`
	got, err := parseBundleOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "claude-code" {
		t.Fatalf("profile = %q", got.Profile)
	}
	if got.Panel.URL != "http://1.2.3.4:7681/" {
		t.Fatalf("panel url = %q", got.Panel.URL)
	}
	if got.Claude.StartCommand != "claude" {
		t.Fatalf("start command = %q", got.Claude.StartCommand)
	}
}
