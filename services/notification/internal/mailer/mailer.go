package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"unicode"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Pass     string
	From     string
	FromName string
	ReplyTo  string
	TLS      bool
	SSL      bool
}

type Mailer struct {
	cfg Config
}

func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

type Message struct {
	To            string
	Subject       string
	Body          string // plain text
	HTML          string // optional HTML alternative
	Transactional bool   // OTP / account mail — omit bulk/list headers
	// Inline images referenced from HTML as cid:<ContentID>.
	Inline []InlinePart
}

type InlinePart struct {
	ContentID   string // e.g. "email-logo" → src="cid:email-logo"
	ContentType string // e.g. "image/png"
	Data        []byte
}

func (m *Mailer) Mode() string {
	if m.cfg.SSL {
		return "smtp-ssl"
	}
	if m.cfg.TLS {
		return "smtp-starttls"
	}
	if m.cfg.User != "" {
		return "smtp-auth"
	}
	return "smtp-plain"
}

func (m *Mailer) Send(msg Message) error {
	if m.cfg.SSL {
		return m.sendImplicitTLS(msg)
	}
	if m.cfg.TLS {
		return m.sendStartTLS(msg)
	}
	return m.sendPlain(msg)
}

func (m *Mailer) sendPlain(msg Message) error {
	addr := fmt.Sprintf("%s:%s", m.cfg.Host, m.cfg.Port)
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}
	return smtp.SendMail(addr, auth, m.cfg.From, []string{msg.To}, m.buildBody(msg))
}

func (m *Mailer) sendImplicitTLS(msg Message) error {
	addr := fmt.Sprintf("%s:%s", m.cfg.Host, m.cfg.Port)
	tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if m.cfg.User != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(msg.To); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(m.buildBody(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (m *Mailer) sendStartTLS(msg Message) error {
	addr := fmt.Sprintf("%s:%s", m.cfg.Host, m.cfg.Port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
			return err
		}
	}

	if m.cfg.User != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(msg.To); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(m.buildBody(msg)); err != nil {
		return err
	}
	return w.Close()
}

func (m *Mailer) buildBody(msg Message) []byte {
	subject := encodeSubject(msg.Subject)
	headers := []string{
		"From: " + formatFromHeader(m.cfg.FromName, m.cfg.From),
		"To: " + msg.To,
		"Subject: " + subject,
		"MIME-Version: 1.0",
	}
	if msg.Transactional {
		if replyTo := strings.TrimSpace(m.cfg.ReplyTo); replyTo != "" {
			headers = append(headers, "Reply-To: "+replyTo)
		}
	} else {
		// Prevent vacation / mailing-list auto-replies from bouncing into our inbox.
		headers = append(headers,
			"Auto-Submitted: auto-generated",
			"X-Auto-Response-Suppress: All",
			"Precedence: bulk",
		)
	}

	plain := msg.Body
	html := strings.TrimSpace(msg.HTML)
	if html == "" {
		headers = append(headers,
			"Content-Type: text/plain; charset=UTF-8",
			"Content-Transfer-Encoding: 8bit",
			"",
			plain,
		)
		return []byte(strings.Join(headers, "\r\n"))
	}

	altBoundary := "cloudhustle_alt_boundary"
	var alt strings.Builder
	alt.WriteString("--" + altBoundary + "\r\n")
	alt.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	alt.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	alt.WriteString(plain)
	alt.WriteString("\r\n\r\n")
	alt.WriteString("--" + altBoundary + "\r\n")
	alt.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	alt.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	alt.WriteString(html)
	alt.WriteString("\r\n\r\n")
	alt.WriteString("--" + altBoundary + "--\r\n")

	var b strings.Builder
	b.WriteString(strings.Join(headers, "\r\n"))
	b.WriteString("\r\n")

	if len(msg.Inline) == 0 {
		b.WriteString("Content-Type: multipart/alternative; boundary=\"" + altBoundary + "\"\r\n\r\n")
		b.WriteString(alt.String())
		return []byte(b.String())
	}

	relBoundary := "cloudhustle_rel_boundary"
	b.WriteString("Content-Type: multipart/related; boundary=\"" + relBoundary + "\"\r\n\r\n")
	b.WriteString("--" + relBoundary + "\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + altBoundary + "\"\r\n\r\n")
	b.WriteString(alt.String())
	b.WriteString("\r\n")
	for _, part := range msg.Inline {
		cid := strings.TrimSpace(part.ContentID)
		ctype := strings.TrimSpace(part.ContentType)
		if cid == "" || ctype == "" || len(part.Data) == 0 {
			continue
		}
		b.WriteString("--" + relBoundary + "\r\n")
		b.WriteString("Content-Type: " + ctype + "\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString("Content-ID: <" + cid + ">\r\n")
		b.WriteString("Content-Disposition: inline; filename=\"" + cid + ".png\"\r\n\r\n")
		b.WriteString(wrapBase64(part.Data))
		b.WriteString("\r\n")
	}
	b.WriteString("--" + relBoundary + "--\r\n")
	return []byte(b.String())
}

func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	const line = 76
	var b strings.Builder
	for len(encoded) > line {
		b.WriteString(encoded[:line])
		b.WriteString("\r\n")
		encoded = encoded[line:]
	}
	if encoded != "" {
		b.WriteString(encoded)
		b.WriteString("\r\n")
	}
	return b.String()
}

func encodeSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	needsEncode := false
	for _, r := range s {
		if r > unicode.MaxASCII {
			needsEncode = true
			break
		}
	}
	if !needsEncode {
		return s
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func formatFromHeader(name, addr string) string {
	addr = strings.TrimSpace(addr)
	name = strings.TrimSpace(name)
	if addr == "" {
		return name
	}
	if name == "" || strings.EqualFold(name, addr) {
		return addr
	}
	return encodeSubject(name) + " <" + addr + ">"
}
