package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/services/telegram-bot/internal/apiclient"
	"github.com/borishru-boop/testVPStrade/services/telegram-bot/internal/config"
	"github.com/borishru-boop/testVPStrade/services/telegram-bot/internal/tgapi"
)

type pendingKind string

const (
	pendingNone            pendingKind = ""
	pendingEmail           pendingKind = "email"
	pendingCode            pendingKind = "code"
	pendingSupportEmail    pendingKind = "support_email"
	pendingSupportMessage  pendingKind = "support_message"
)

type pending struct {
	Kind    pendingKind
	Email   string
	Expires time.Time
}

type Bot struct {
	cfg    config.Config
	tg     *tgapi.Client
	api    *apiclient.Client
	mu     sync.Mutex
	wait   map[int64]pending
	offset int64
}

func New(cfg config.Config) *Bot {
	return &Bot{
		cfg:  cfg,
		tg:   tgapi.New(cfg.BotToken),
		api:  apiclient.New(cfg.AuthURL, cfg.VPSURL, cfg.BillingURL, cfg.SupportURL, cfg.InternalToken),
		wait: map[int64]pending{},
	}
}

func (b *Bot) Run(ctx context.Context) error {
	log.Printf("telegram-bot: polling started")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		timeout := int(b.cfg.PollTimeout.Seconds())
		if timeout < 1 {
			timeout = 25
		}
		updates, err := b.tg.GetUpdates(ctx, b.offset, timeout)
		if err != nil {
			log.Printf("telegram-bot: getUpdates: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			if err := b.handleUpdate(ctx, u); err != nil {
				log.Printf("telegram-bot: handle: %v", err)
			}
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, u tgapi.Update) error {
	if u.CallbackQuery != nil {
		return b.handleCallback(ctx, u.CallbackQuery)
	}
	if u.Message == nil || u.Message.From == nil {
		return nil
	}
	return b.handleMessage(ctx, u.Message)
}

func parseStartPayload(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/start") {
		return ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	// "/start@BotName payload" or "/start@BotName"
	if strings.HasPrefix(rest, "@") {
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[1])
	}
	return rest
}

func (b *Bot) handleMessage(ctx context.Context, m *tgapi.Message) error {
	tgID := m.From.ID
	chatID := m.Chat.ID
	text := strings.TrimSpace(m.Text)

	if strings.HasPrefix(text, "/start") {
		b.clearPending(tgID)
		payload := parseStartPayload(text)
		if strings.HasPrefix(payload, "wl") && len(payload) > 2 {
			sess, err := b.api.ConfirmWebLink(ctx, payload, tgID)
			if err != nil {
				msg := "Ссылка привязки недействительна или истекла. Включите Telegram-уведомления ещё раз в настройках на сайте."
				if strings.Contains(err.Error(), "already linked") {
					msg = "Этот Telegram уже привязан к другому аккаунту."
				}
				return b.tg.SendMessage(ctx, chatID, msg, mainKeyboard(false))
			}
			return b.tg.SendMessage(ctx, chatID, "✅ Telegram привязан к <b>"+sess.Email+"</b>.\nУведомления включены.", mainKeyboard(true))
		}
		if payload == "nocode" || payload == "support" {
			return b.startGuestSupport(ctx, chatID, tgID, payload == "nocode")
		}
		if payload != "" {
			log.Printf("telegram-bot: ignored start payload %q from %d", payload, tgID)
		}
		return b.sendMain(ctx, chatID, tgID)
	}

	// Menu / cancel must always win over pending email/ticket dialogs.
	if isMainMenuCommand(text) {
		b.clearPending(tgID)
		if text == "/cancel" || text == "Отмена" || text == "❌ Отмена" {
			return b.tg.SendMessage(ctx, chatID, "Ок, диалог отменён.", mainKeyboard(b.isLinked(ctx, tgID)))
		}
	} else if p, ok := b.getPending(tgID); ok {
		switch p.Kind {
		case pendingEmail:
			email := strings.ToLower(text)
			if !strings.Contains(email, "@") {
				return b.tg.SendMessage(ctx, chatID, "Введите email аккаунта на cloud-hustle.com\nИли нажмите кнопку меню / «Отмена».", mainKeyboard(b.isLinked(ctx, tgID)))
			}
			if err := b.api.RequestLink(ctx, email, tgID); err != nil {
				msg := "Не удалось отправить код. Проверьте email или зарегистрируйтесь на сайте."
				if strings.Contains(err.Error(), "not found") {
					msg = "Аккаунт с таким email не найден. Сначала зарегистрируйтесь: " + b.cfg.SiteURL + "/register"
				}
				return b.tg.SendMessage(ctx, chatID, msg, nil)
			}
			b.setPending(tgID, pending{Kind: pendingCode, Email: email, Expires: time.Now().Add(15 * time.Minute)})
			return b.tg.SendMessage(ctx, chatID, "Код отправлен на <b>"+email+"</b>. Введите 6 цифр из письма.", nil)
		case pendingCode:
			code := strings.TrimSpace(text)
			sess, err := b.api.ConfirmLink(ctx, code, tgID)
			if err != nil {
				return b.tg.SendMessage(ctx, chatID, "Неверный или просроченный код. Нажмите «Привязать аккаунт» ещё раз.", mainKeyboard(false))
			}
			b.clearPending(tgID)
			_ = sess
			return b.tg.SendMessage(ctx, chatID, "✅ Telegram привязан к <b>"+sess.Email+"</b>.\nТеперь доступны серверы и баланс.", mainKeyboard(true))
		case pendingSupportEmail:
			email := strings.ToLower(text)
			if !strings.Contains(email, "@") {
				return b.tg.SendMessage(ctx, chatID, "Введите email, с которым регистрируетесь (например name@mail.ru)\nИли нажмите кнопку меню / «Отмена».", mainKeyboard(b.isLinked(ctx, tgID)))
			}
			b.setPending(tgID, pending{Kind: pendingSupportMessage, Email: email, Expires: time.Now().Add(20 * time.Minute)})
			return b.tg.SendMessage(ctx, chatID, "Опишите проблему одним сообщением (например: не приходит код подтверждения на почту).", nil)
		case pendingSupportMessage:
			return b.finishGuestSupport(ctx, chatID, tgID, p.Email, text)
		}
	}

	switch text {
	case "Тарифы", "📋 Тарифы":
		return b.showTariffs(ctx, chatID)
	case "Мои серверы", "🖥 Мои серверы":
		return b.showServers(ctx, chatID, tgID)
	case "Баланс / оплата", "💳 Баланс / оплата":
		return b.showBalance(ctx, chatID, tgID)
	case "Поддержка", "🆘 Поддержка":
		return b.showSupport(ctx, chatID)
	case "Канал", "📢 Канал":
		return b.tg.SendMessage(ctx, chatID, "Наш канал:", urlKeyboard("Открыть канал", b.cfg.ChannelURL))
	case "Привязать аккаунт", "🔗 Привязать аккаунт":
		b.setPending(tgID, pending{Kind: pendingEmail, Expires: time.Now().Add(15 * time.Minute)})
		return b.tg.SendMessage(ctx, chatID, "Введите email аккаунта Cloud-hustle:", nil)
	case "Главное меню", "/menu":
		return b.sendMain(ctx, chatID, tgID)
	default:
		// Continue open guest ticket conversation from Telegram.
		if id, err := b.api.LookupGuestTicket(ctx, chatID); err == nil && id != "" {
			if err := b.api.AddGuestTicketMessage(ctx, chatID, text); err != nil {
				return b.tg.SendMessage(ctx, chatID, "Не удалось добавить сообщение в тикет. Попробуйте позже или напишите на support@cloud-hustle.com", mainKeyboard(b.isLinked(ctx, tgID)))
			}
			return b.tg.SendMessage(ctx, chatID, "✅ Сообщение добавлено в тикет. Ответим сюда же.", mainKeyboard(b.isLinked(ctx, tgID)))
		}
		return b.sendMain(ctx, chatID, tgID)
	}
}

func isMainMenuCommand(text string) bool {
	switch text {
	case "Тарифы", "📋 Тарифы",
		"Мои серверы", "🖥 Мои серверы",
		"Баланс / оплата", "💳 Баланс / оплата",
		"Поддержка", "🆘 Поддержка",
		"Канал", "📢 Канал",
		"Привязать аккаунт", "🔗 Привязать аккаунт",
		"Главное меню", "/menu",
		"/cancel", "Отмена", "❌ Отмена":
		return true
	default:
		return false
	}
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgapi.Callback) error {
	_ = b.tg.AnswerCallback(ctx, cb.ID, "")
	chatID := cb.From.ID
	if cb.Message != nil {
		chatID = cb.Message.Chat.ID
	}
	tgID := cb.From.ID
	data := cb.Data

	switch {
	case data == "main":
		b.clearPending(tgID)
		return b.sendMain(ctx, chatID, tgID)
	case data == "servers":
		b.clearPending(tgID)
		return b.showServers(ctx, chatID, tgID)
	case data == "tariffs":
		b.clearPending(tgID)
		return b.showTariffs(ctx, chatID)
	case strings.HasPrefix(data, "tier:"):
		b.clearPending(tgID)
		return b.showTariffTier(ctx, chatID, strings.TrimPrefix(data, "tier:"))
	case data == "balance":
		b.clearPending(tgID)
		return b.showBalance(ctx, chatID, tgID)
	case data == "link":
		b.setPending(tgID, pending{Kind: pendingEmail, Expires: time.Now().Add(15 * time.Minute)})
		return b.tg.SendMessage(ctx, chatID, "Введите email аккаунта Cloud-hustle:", nil)
	case data == "support_new":
		return b.startGuestSupport(ctx, chatID, tgID, false)
	case data == "support_nocode":
		return b.startGuestSupport(ctx, chatID, tgID, true)
	case strings.HasPrefix(data, "srv:"):
		id := strings.TrimPrefix(data, "srv:")
		return b.showServerCard(ctx, chatID, tgID, id)
	case strings.HasPrefix(data, "reboot:"):
		id := strings.TrimPrefix(data, "reboot:")
		return b.rebootServer(ctx, chatID, tgID, id)
	case strings.HasPrefix(data, "creds:"):
		id := strings.TrimPrefix(data, "creds:")
		return b.showCreds(ctx, chatID, tgID, id)
	case strings.HasPrefix(data, "paymethod:"):
		// paymethod:500:sbp
		rest := strings.TrimPrefix(data, "paymethod:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return nil
		}
		amount, _ := strconv.ParseFloat(parts[0], 64)
		return b.startTopup(ctx, chatID, tgID, amount, parts[1])
	case strings.HasPrefix(data, "pay:"):
		amount, _ := strconv.ParseFloat(strings.TrimPrefix(data, "pay:"), 64)
		return b.choosePayMethod(ctx, chatID, tgID, amount)
	}
	return nil
}

func (b *Bot) sendMain(ctx context.Context, chatID, tgID int64) error {
	linked := b.isLinked(ctx, tgID)
	text := "Cloud-hustle\n\nВыберите раздел:"
	if !linked {
		text = "Cloud-hustle\n\nЧтобы управлять серверами, привяжите аккаунт (email с сайта)."
	}
	return b.tg.SendMessage(ctx, chatID, text, mainKeyboard(linked))
}

func (b *Bot) showTariffs(ctx context.Context, chatID int64) error {
	text := "<b>Тарифы VPS</b> · NL\n\nВыберите линейку:"
	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{
			{Text: "PROSTO", CallbackData: "tier:prosto"},
			{Text: "Midrange", CallbackData: "tier:midrange"},
			{Text: "HUSTLE", CallbackData: "tier:hustle"},
		},
		{{Text: "Заказать на сайте", URL: b.cfg.SiteURL + "/#tariffs"}},
		{{Text: "« Главное меню", CallbackData: "main"}},
	}}
	return b.tg.SendMessage(ctx, chatID, text, kb)
}

func (b *Bot) showTariffTier(ctx context.Context, chatID int64, tier string) error {
	tier = strings.ToLower(strings.TrimSpace(tier))
	title, blurb := tariffTierMeta(tier)
	if title == "" {
		return b.showTariffs(ctx, chatID)
	}

	plans, err := b.api.ListPlans(ctx)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Не удалось загрузить тарифы. Попробуйте позже.", mainKeyboard(true))
	}

	var bld strings.Builder
	bld.WriteString("<b>" + title + "</b> · NL\n")
	bld.WriteString(blurb + "\n")

	shown := 0
	for _, p := range plans {
		if !p.Active || strings.ToLower(p.Tier) != tier {
			continue
		}
		ram := formatRAM(p.RAMMb)
		bld.WriteString(fmt.Sprintf(
			"\n<b>%s</b>\n%d vCPU · %s · %d GB\n<b>%.0f ₽/мес</b>\n",
			p.Name, p.CPU, ram, p.DiskGB, p.PriceMonthly,
		))
		shown++
	}
	if shown == 0 {
		bld.WriteString("\nПока нет активных тарифов в этой линейке.")
	}

	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "Заказать на сайте", URL: b.cfg.SiteURL + "/#tariffs"}},
		{
			{Text: "« К линейкам", CallbackData: "tariffs"},
			{Text: "« Меню", CallbackData: "main"},
		},
	}}
	return b.tg.SendMessage(ctx, chatID, bld.String(), kb)
}

func tariffTierMeta(tier string) (title, blurb string) {
	switch tier {
	case "prosto":
		return "PROSTO", "Старт для сайтов и лёгких задач"
	case "midrange":
		return "Midrange", "Баланс цены и производительности"
	case "hustle":
		return "HUSTLE", "Больше ресурсов под нагрузку"
	default:
		return "", ""
	}
}

func formatRAM(ramMb int) string {
	if ramMb >= 1024 && ramMb%1024 == 0 {
		return fmt.Sprintf("%d GB RAM", ramMb/1024)
	}
	if ramMb >= 1024 {
		return fmt.Sprintf("%.1f GB RAM", float64(ramMb)/1024)
	}
	return fmt.Sprintf("%d MB RAM", ramMb)
}

func (b *Bot) showServers(ctx context.Context, chatID, tgID int64) error {
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	items, err := b.api.ListInstances(ctx, sess.AccessToken)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Ошибка загрузки серверов: "+err.Error(), mainKeyboard(true))
	}
	if len(items) == 0 {
		kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
			{{Text: "Заказать VPS", URL: b.cfg.SiteURL + "/#tariffs"}},
			{{Text: "« Главное меню", CallbackData: "main"}},
		}}
		return b.tg.SendMessage(ctx, chatID, "У вас пока нет серверов.", kb)
	}
	rows := make([][]tgapi.InlineButton, 0, len(items)+1)
	for _, inst := range items {
		label := inst.PlanName
		if label == "" {
			label = "VPS"
		}
		if inst.Hostname != "" {
			label = inst.Hostname + " · " + label
		}
		if inst.OrderNumber != nil {
			label = fmt.Sprintf("#%d %s", *inst.OrderNumber, label)
		}
		label = fmt.Sprintf("%s [%s]", label, inst.State)
		rows = append(rows, []tgapi.InlineButton{{Text: truncate(label, 60), CallbackData: "srv:" + inst.ID}})
	}
	rows = append(rows, []tgapi.InlineButton{{Text: "« Главное меню", CallbackData: "main"}})
	return b.tg.SendMessage(ctx, chatID, "<b>Мои серверы</b>\nВыберите сервер:", tgapi.InlineKeyboard{InlineKeyboard: rows})
}

func (b *Bot) showServerCard(ctx context.Context, chatID, tgID int64, id string) error {
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	inst, err := b.api.GetInstance(ctx, sess.AccessToken, id)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Сервер не найден.", nil)
	}
	ip := inst.IPAddress
	if ip == "" {
		ip = "—"
	}
	host := inst.Hostname
	if host == "" {
		host = "vps"
	}
	plan := inst.PlanName
	if plan == "" {
		plan = "—"
	}
	billing := inst.NextBillingAt
	if billing == "" {
		billing = "—"
	}
	text := fmt.Sprintf(
		"<b>%s</b>\nТариф: %s\nСтатус: <b>%s</b>\nБиллинг: %s\nIP: <code>%s</code>\nРегион: %s\nСлед. списание: %s",
		host, plan, inst.State, inst.BillingStatus, ip, inst.Region, billing,
	)
	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "🔄 Перезагрузка", CallbackData: "reboot:" + id}},
		{{Text: "🔑 Доступы", CallbackData: "creds:" + id}},
		{{Text: "💳 Продлить / баланс", CallbackData: "balance"}},
		{{Text: "Открыть в ЛК", URL: b.cfg.SiteURL + "/vps/servers/" + id}},
		{{Text: "« К списку", CallbackData: "servers"}},
	}}
	return b.tg.SendMessage(ctx, chatID, text, kb)
}

func (b *Bot) rebootServer(ctx context.Context, chatID, tgID int64, id string) error {
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	if err := b.api.Reboot(ctx, sess.AccessToken, id); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "provisioning") {
			msg = "Сервер ещё создаётся (нет IP). Перезагрузка будет доступна, когда статус станет «Работает» и появится IP."
		} else {
			msg = "Не удалось перезагрузить: " + msg
		}
		return b.tg.SendMessage(ctx, chatID, msg, nil)
	}
	return b.tg.SendMessage(ctx, chatID, "✅ Команда перезагрузки отправлена.", tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "К карточке", CallbackData: "srv:" + id}},
		{{Text: "« Список", CallbackData: "servers"}},
	}})
}

func (b *Bot) showCreds(ctx context.Context, chatID, tgID int64, id string) error {
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	creds, err := b.api.Credentials(ctx, sess.AccessToken, id)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Доступы недоступны: "+err.Error()+"\nОбычно нужны, когда сервер в статусе running.", nil)
	}
	text := fmt.Sprintf(
		"<b>Доступы</b> (не пересылайте третьим лицам)\n\nIP: <code>%s</code>\nSSH: <code>%d</code>\nЛогин: <code>%s</code>\nПароль: <code>%s</code>",
		creds.IPAddress, creds.SSHPort, creds.Username, creds.RootPassword,
	)
	return b.tg.SendMessage(ctx, chatID, text, tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "« Назад", CallbackData: "srv:" + id}},
	}})
}

func (b *Bot) showBalance(ctx context.Context, chatID, tgID int64) error {
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	bal, err := b.api.GetBalance(ctx, sess.AccessToken)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Не удалось получить баланс: "+err.Error(), nil)
	}
	cur := bal.Currency
	if cur == "" {
		cur = "RUB"
	}
	text := fmt.Sprintf("<b>Баланс</b>: %.2f %s\n\nПополнение:", bal.Balance, cur)
	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "500 ₽", CallbackData: "pay:500"}, {Text: "1000 ₽", CallbackData: "pay:1000"}},
		{{Text: "2000 ₽", CallbackData: "pay:2000"}, {Text: "5000 ₽", CallbackData: "pay:5000"}},
		{{Text: "Открыть биллинг на сайте", URL: b.cfg.SiteURL + "/vps/billing"}},
		{{Text: "« Главное меню", CallbackData: "main"}},
	}}
	return b.tg.SendMessage(ctx, chatID, text, kb)
}

func (b *Bot) choosePayMethod(ctx context.Context, chatID, tgID int64, amount float64) error {
	if amount < 100 {
		amount = 500
	}
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	methods, err := b.api.PaymentMethods(ctx, sess.AccessToken)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Не удалось загрузить способы оплаты: "+err.Error(), nil)
	}

	labels := map[string]string{
		"sbp":          "СБП",
		"tpay":         "T-Pay",
		"sberpay":      "SberPay",
		"card":         "Карта РФ (T-Bank)",
		"card_foreign": "Visa / Mastercard (зарубежные)",
		"heleket":      "Криптовалюта",
	}

	var rows [][]tgapi.InlineButton
	enabled := 0
	for _, m := range methods {
		if !m.Enabled {
			continue
		}
		label := labels[m.ID]
		if label == "" {
			label = m.ID
		}
		amt := strconv.FormatFloat(amount, 'f', 0, 64)
		rows = append(rows, []tgapi.InlineButton{{
			Text:         label,
			CallbackData: "paymethod:" + amt + ":" + m.ID,
		}})
		enabled++
	}
	if enabled == 0 {
		return b.tg.SendMessage(ctx, chatID, "Сейчас нет доступных способов оплаты. Откройте биллинг на сайте.", tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
			{{Text: "Биллинг", URL: b.cfg.SiteURL + "/vps/billing"}},
			{{Text: "« Назад", CallbackData: "balance"}},
		}})
	}
	rows = append(rows, []tgapi.InlineButton{{Text: "« Назад", CallbackData: "balance"}})
	return b.tg.SendMessage(ctx, chatID, fmt.Sprintf("Сумма: <b>%.0f ₽</b>\nВыберите способ оплаты:", amount), tgapi.InlineKeyboard{InlineKeyboard: rows})
}

func (b *Bot) startTopup(ctx context.Context, chatID, tgID int64, amount float64, method string) error {
	if amount < 100 {
		amount = 500
	}
	if method == "" {
		method = "card"
	}
	sess, err := b.api.ResolveTelegram(ctx, tgID)
	if err != nil {
		return b.needLink(ctx, chatID)
	}
	url, err := b.api.Topup(ctx, sess.AccessToken, amount, method)
	if err != nil {
		return b.tg.SendMessage(ctx, chatID, "Оплата временно недоступна: "+err.Error()+"\nПопробуйте в ЛК: "+b.cfg.SiteURL+"/vps/billing", nil)
	}
	return b.tg.SendMessage(ctx, chatID, fmt.Sprintf("Счёт на %.0f ₽ создан. Оплатите по ссылке:", amount), urlKeyboard("Оплатить", url))
}

func (b *Bot) showSupport(ctx context.Context, chatID int64) error {
	text := "Поддержка Cloud-hustle\n\nМожно создать тикет прямо здесь (даже без входа в ЛК) — ответ придёт в этот чат.\nИли откройте тикеты в личном кабинете."
	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "✍️ Создать тикет в боте", CallbackData: "support_new"}},
		{{Text: "Не приходит код", CallbackData: "support_nocode"}},
		{{Text: "Открыть тикеты в ЛК", URL: b.cfg.SupportURLPublic}},
		{{Text: "« Главное меню", CallbackData: "main"}},
	}}
	return b.tg.SendMessage(ctx, chatID, text, kb)
}

func (b *Bot) startGuestSupport(ctx context.Context, chatID, tgID int64, noCode bool) error {
	if id, err := b.api.LookupGuestTicket(ctx, chatID); err == nil && id != "" {
		msg := "У вас уже есть открытый тикет. Напишите сообщение — оно добавится в обращение."
		if noCode {
			msg = "У вас уже есть открытый тикет по регистрации. Напишите сюда детали — добавим в обращение."
		}
		return b.tg.SendMessage(ctx, chatID, msg, mainKeyboard(b.isLinked(ctx, tgID)))
	}
	b.setPending(tgID, pending{Kind: pendingSupportEmail, Expires: time.Now().Add(20 * time.Minute)})
	hint := "Создание тикета.\n\nВведите email, с которым работаете на сайте:"
	if noCode {
		hint = "Не приходит код подтверждения?\n\nВведите email, на который регистрировались:"
	}
	return b.tg.SendMessage(ctx, chatID, hint, nil)
}

func (b *Bot) finishGuestSupport(ctx context.Context, chatID, tgID int64, email, message string) error {
	message = strings.TrimSpace(message)
	if len([]rune(message)) < 5 {
		return b.tg.SendMessage(ctx, chatID, "Напишите чуть подробнее (хотя бы несколько слов).", nil)
	}
	subject := "Обращение из Telegram"
	lower := strings.ToLower(message)
	if strings.Contains(lower, "код") || strings.Contains(lower, "подтвержд") || strings.Contains(lower, "verify") {
		subject = "Не приходит код подтверждения email"
	}
	id, err := b.api.CreateGuestTicket(ctx, email, subject, message, chatID)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			_ = b.api.AddGuestTicketMessage(ctx, chatID, message)
			b.clearPending(tgID)
			return b.tg.SendMessage(ctx, chatID, "Сообщение добавлено в уже открытый тикет. Ответим в этот чат.", mainKeyboard(b.isLinked(ctx, tgID)))
		}
		return b.tg.SendMessage(ctx, chatID, "Не удалось создать тикет: "+err.Error()+"\nНапишите на support@cloud-hustle.com", mainKeyboard(b.isLinked(ctx, tgID)))
	}
	b.clearPending(tgID)
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return b.tg.SendMessage(ctx, chatID, "✅ Тикет создан (#"+short+"…). Ответ поддержки придёт в этот чат.\nМожете писать сюда дополнительные детали.", mainKeyboard(b.isLinked(ctx, tgID)))
}

func (b *Bot) needLink(ctx context.Context, chatID int64) error {
	kb := tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: "🔗 Привязать аккаунт", CallbackData: "link"}},
		{{Text: "Регистрация", URL: b.cfg.SiteURL + "/register"}},
	}}
	return b.tg.SendMessage(ctx, chatID, "Сначала привяжите Telegram к аккаунту.", kb)
}

func (b *Bot) isLinked(ctx context.Context, tgID int64) bool {
	_, err := b.api.ResolveTelegram(ctx, tgID)
	return err == nil
}

func mainKeyboard(linked bool) tgapi.ReplyKeyboard {
	rows := [][]tgapi.KeyboardButton{
		{{Text: "📋 Тарифы"}},
	}
	if linked {
		rows = append(rows,
			[]tgapi.KeyboardButton{{Text: "🖥 Мои серверы"}, {Text: "💳 Баланс / оплата"}},
		)
	} else {
		rows = append(rows, []tgapi.KeyboardButton{{Text: "🔗 Привязать аккаунт"}})
	}
	rows = append(rows,
		[]tgapi.KeyboardButton{{Text: "🆘 Поддержка"}, {Text: "📢 Канал"}},
	)
	return tgapi.ReplyKeyboard{Keyboard: rows, ResizeKeyboard: true}
}

func urlKeyboard(text, rawURL string) tgapi.InlineKeyboard {
	return tgapi.InlineKeyboard{InlineKeyboard: [][]tgapi.InlineButton{
		{{Text: text, URL: rawURL}},
		{{Text: "« Главное меню", CallbackData: "main"}},
	}}
}

func (b *Bot) setPending(tgID int64, p pending) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wait[tgID] = p
}

func (b *Bot) getPending(tgID int64) (pending, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.wait[tgID]
	if !ok {
		return pending{}, false
	}
	if time.Now().After(p.Expires) {
		delete(b.wait, tgID)
		return pending{}, false
	}
	return p, true
}

func (b *Bot) clearPending(tgID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.wait, tgID)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
