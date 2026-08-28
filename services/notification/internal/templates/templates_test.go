package templates

import (
	"strings"
	"testing"
)

func TestIsTransactional(t *testing.T) {
	for _, name := range []string{"email_verify", "password_reset", "telegram_link"} {
		if !IsTransactional(name) {
			t.Fatalf("%s should be transactional", name)
		}
	}
	if IsTransactional("instance_ready") {
		t.Fatal("instance_ready should not be transactional")
	}
}

func TestRender_emailVerifyPlainOnly(t *testing.T) {
	subject, body, html, err := Render("email_verify", "ru", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Код подтверждения email" {
		t.Fatalf("subject = %q", subject)
	}
	if body == "" {
		t.Fatal("expected plain body")
	}
	if html != "" {
		t.Fatal("transactional verify should not render HTML")
	}
	if !strings.Contains(body, "123456") {
		t.Fatal("body should contain code")
	}
}
