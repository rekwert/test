package worker

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/notification/internal/assets"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/mailer"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/store"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/templates"
	sharedredis "github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
)

var emailLogoSrcRe = regexp.MustCompile(`(?i)(src=["'])[^"']*email-logo\.png(["'])`)

const redisChannel = "notification:email"

type EmailProcessor struct {
	store  *store.Store
	mailer *mailer.Mailer
	redis  *sharedredis.Client
}

func NewEmailProcessor(st *store.Store, m *mailer.Mailer, rdb *sharedredis.Client) *EmailProcessor {
	return &EmailProcessor{store: st, mailer: m, redis: rdb}
}

func (p *EmailProcessor) Run(ctx context.Context, pollInterval time.Duration) {
	wake := make(chan struct{}, 16)
	if p.redis != nil {
		go p.subscribe(ctx, wake)
	}

	process := func() {
		n, err := p.processBatch(ctx)
		if err != nil {
			log.Printf("notification worker: %v", err)
			return
		}
		if n > 0 {
			log.Printf("notification worker: sent %d emails", n)
		}
	}

	process()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			process()
		case <-ticker.C:
			process()
		}
	}
}

func (p *EmailProcessor) subscribe(ctx context.Context, wake chan<- struct{}) {
	pubsub := p.redis.Subscribe(ctx, redisChannel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg != nil {
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (p *EmailProcessor) NotifyQueued(ctx context.Context) {
	if p.redis == nil {
		return
	}
	_ = p.redis.Publish(ctx, redisChannel, "1")
}

func (p *EmailProcessor) EnqueueAndSignal(ctx context.Context, userID *string, to, subject, body, template string, metadata json.RawMessage) (int64, error) {
	id, err := p.store.EnqueueEmail(ctx, userID, to, subject, body, template, metadata)
	if err != nil {
		return 0, err
	}
	if p.redis != nil {
		_ = p.redis.Publish(ctx, redisChannel, "1")
	}
	return id, nil
}

func (p *EmailProcessor) processBatch(ctx context.Context) (int, error) {
	items, err := p.store.FetchPendingDeliveries(ctx, 30)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, d := range items {
		if d.UserID != nil && *d.UserID != "" {
			ok, err := p.store.UserWantsEmail(ctx, *d.UserID)
			if err == nil && !ok {
				_ = p.store.MarkDeliverySent(ctx, d.ID)
				continue
			}
		}
		to := strings.TrimSpace(d.ToEmail)
		if to == "" {
			_ = p.store.MarkDeliveryFailed(ctx, d.ID, "missing to_email")
			continue
		}
		subject, body, htmlBody, err := p.render(d)
		if err != nil {
			_ = p.store.MarkDeliveryFailed(ctx, d.ID, err.Error())
			continue
		}
		msg := mailer.Message{
			To:            to,
			Subject:       subject,
			Body:          body,
			HTML:          htmlBody,
			Transactional: templates.IsTransactional(d.Template),
		}
		if htmlBody != "" && !msg.Transactional {
			msg.HTML, msg.Inline = attachEmailLogo(htmlBody)
		}
		if err := p.mailer.Send(msg); err != nil {
			_ = p.store.MarkDeliveryFailed(ctx, d.ID, err.Error())
			continue
		}
		if err := p.store.MarkDeliverySent(ctx, d.ID); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (p *EmailProcessor) render(d store.Delivery) (subject, body, htmlBody string, err error) {
	var meta map[string]string
	_ = json.Unmarshal(d.Metadata, &meta)
	if templates.IsTransactional(d.Template) {
		htmlBody = ""
	} else {
		htmlBody = strings.TrimSpace(meta["html_body"])
	}

	if d.Template != "" && d.Body == "" {
		code := meta["code"]
		locale := meta["locale"]
		if locale == "" {
			locale = "ru"
		}
		subject, body, renderedHTML, err := templates.Render(d.Template, locale, code)
		if err != nil {
			return "", "", "", err
		}
		if htmlBody == "" && !templates.IsTransactional(d.Template) {
			htmlBody = renderedHTML
		}
		return subject, body, htmlBody, nil
	}
	subject = d.Subject
	body = d.Body
	if subject == "" {
		subject = "Уведомление"
	}
	// Prefer stored HTML; if missing and we still have template+code, re-render.
	if htmlBody == "" && d.Template != "" && meta["code"] != "" {
		locale := meta["locale"]
		if locale == "" {
			locale = "ru"
		}
		if _, _, renderedHTML, rerr := templates.Render(d.Template, locale, meta["code"]); rerr == nil {
			htmlBody = renderedHTML
		}
	}
	return subject, body, htmlBody, nil
}

// attachEmailLogo rewrites hosted/cid logo refs to an embedded PNG part so mail
// clients show the mark even when the public site asset is missing/undeployed.
func attachEmailLogo(htmlBody string) (string, []mailer.InlinePart) {
	cidSrc := "cid:" + assets.EmailLogoCID
	out := htmlBody
	if strings.Contains(out, "email-logo") || strings.Contains(out, cidSrc) {
		out = emailLogoSrcRe.ReplaceAllString(out, `${1}`+cidSrc+`${2}`)
		out = strings.ReplaceAll(out, `src="cid:email-logo"`, `src="`+cidSrc+`"`)
		out = strings.ReplaceAll(out, `src='cid:email-logo'`, `src='`+cidSrc+`'`)
	}
	if !strings.Contains(out, cidSrc) {
		return htmlBody, nil
	}
	if len(assets.EmailLogo) == 0 {
		return out, nil
	}
	return out, []mailer.InlinePart{{
		ContentID:   assets.EmailLogoCID,
		ContentType: "image/png",
		Data:        assets.EmailLogo,
	}}
}
