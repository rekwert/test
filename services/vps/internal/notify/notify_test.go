package notify

import (
	"strings"
	"testing"
)

func TestBuildReadyPlainOmitsSSHForWindows(t *testing.T) {
	plain := buildReadyPlain(
		"vps-nikita3",
		"212.102.227.24",
		"secret",
		"https://cloud-hustle.com/vps/servers",
		"support@cloud-hustle.com",
		ReadyProvision,
		false,
	)
	if strings.Contains(plain, "ssh root@") || strings.Contains(plain, "Подключение:") {
		t.Fatalf("windows plain must not include SSH block:\n%s", plain)
	}
	for _, want := range []string{
		"Ваша VPS vps-nikita3 готова к использованию.",
		"IP: 212.102.227.24",
		"Логин: root",
		"Пароль: secret",
		"Панель управления: https://cloud-hustle.com/vps/servers",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in:\n%s", want, plain)
		}
	}
}

func TestBuildReadyPlainIncludesSSHForLinux(t *testing.T) {
	plain := buildReadyPlain("vps-x", "1.2.3.4", "pw", "https://x/y", "a@b.c", ReadyProvision, true)
	if !strings.Contains(plain, "Подключение:\nssh root@1.2.3.4") {
		t.Fatalf("linux plain must include SSH:\n%s", plain)
	}
}

func TestBuildReadyHTMLOmitsSSHForWindows(t *testing.T) {
	htmlBody := buildReadyHTML(
		"vps-nikita3", "212.102.227.24", "secret",
		"https://cloud-hustle.com/vps/servers", "support@cloud-hustle.com",
		"https://cloud-hustle.com/email-logo.png",
		"Ваша VPS <strong>vps-nikita3</strong> развёрнута и готова к использованию.",
		"Аренда виртуальных серверов",
		false,
	)
	if strings.Contains(htmlBody, "ssh root@") || strings.Contains(htmlBody, "Подключение:") {
		t.Fatalf("windows html must not include SSH:\n%s", htmlBody)
	}
}
