package mailer

import (
	"strings"
	"testing"
)

func TestBuildBody_transactionalPlainOnly(t *testing.T) {
	m := New(Config{
		From:     "noreply@example.com",
		FromName: "CLOUD HUSTLE",
		ReplyTo:  "support@example.com",
	})
	body := m.buildBody(Message{
		To:            "user@example.com",
		Subject:       "Код подтверждения email",
		Body:          "Код: 123456",
		Transactional: true,
	})
	text := string(body)
	if strings.Contains(text, "Precedence: bulk") {
		t.Fatal("transactional message must not include Precedence: bulk")
	}
	if strings.Contains(text, "Auto-Submitted:") {
		t.Fatal("transactional message should omit Auto-Submitted")
	}
	if strings.Contains(text, "multipart/") {
		t.Fatal("transactional message should be plain text only")
	}
	if !strings.Contains(text, "Content-Type: text/plain") {
		t.Fatal("expected text/plain content type")
	}
	if !strings.Contains(text, "From: CLOUD HUSTLE <noreply@example.com>") {
		t.Fatalf("unexpected From header: %q", text)
	}
	if !strings.Contains(text, "Reply-To: support@example.com") {
		t.Fatal("expected Reply-To header")
	}
}

func TestBuildBody_bulkKeepsPrecedence(t *testing.T) {
	m := New(Config{From: "noreply@example.com"})
	body := m.buildBody(Message{
		To:      "user@example.com",
		Subject: "News",
		Body:    "hello",
	})
	text := string(body)
	if !strings.Contains(text, "Precedence: bulk") {
		t.Fatal("non-transactional message should keep Precedence: bulk")
	}
	if !strings.Contains(text, "Auto-Submitted: auto-generated") {
		t.Fatal("expected Auto-Submitted header")
	}
}
