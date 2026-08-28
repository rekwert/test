package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/services/support/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Ticket struct {
	ID                 string
	UserID             string
	ClientEmail        string
	Subject            string
	Status             string
	Priority           string
	AssigneeID         *string
	InstanceID         *string
	Category           string
	TelegramChatID     *int64
	SLADueAt           time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastMessageAt      time.Time
	LastMessageByStaff bool
	AnsweredAt         *time.Time
	AutoCloseAt        *time.Time
	IsReturn           bool
}

const ticketSelectSQL = `
	SELECT id::text, COALESCE(user_id::text, ''), client_email, subject, status, priority,
	       assignee_id::text, instance_id::text, category, telegram_chat_id, sla_due_at, created_at, updated_at, last_message_at,
	       last_message_by_staff, answered_at, auto_close_at, is_return
	FROM support.tickets`

type Message struct {
	ID          string
	TicketID    string
	AuthorID    string
	AuthorEmail string
	IsStaff     bool
	Body        string
	CreatedAt   time.Time
	EditedAt    *time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := dbpool.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) migrate(ctx context.Context) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := migrations.FS.ReadFile(e.Name())
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
	}
	return nil
}

func SLADue(priority string, from time.Time) time.Time {
	switch priority {
	case "urgent":
		return from.Add(4 * time.Hour)
	case "high":
		return from.Add(24 * time.Hour)
	case "low":
		return from.Add(72 * time.Hour)
	default:
		return from.Add(48 * time.Hour)
	}
}

func (s *Store) UserOwnsInstance(ctx context.Context, userID, instanceID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM vps.instances
			WHERE id = $1::uuid
			  AND user_id = $2::uuid
			  AND state <> 'deleted'
		)
	`, instanceID, userID).Scan(&ok)
	if err != nil {
		// Invalid UUID etc. → treat as not owned
		if strings.Contains(err.Error(), "invalid input syntax") {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}

func (s *Store) UserHasActiveInstance(ctx context.Context, userID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM vps.instances
			WHERE user_id = $1::uuid
			  AND state <> 'deleted'
		)
	`, userID).Scan(&ok)
	return ok, err
}

func (s *Store) CreateTicket(ctx context.Context, userID, email, subject, message, priority, category, instanceID string) (*Ticket, error) {
	if priority == "" {
		priority = "normal"
	}
	if category == "" {
		category = "other"
	}
	now := time.Now().UTC()
	sla := SLADue(priority, now)

	var instID any
	if instanceID != "" {
		instID = instanceID
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var t Ticket
	err = tx.QueryRow(ctx, `
		INSERT INTO support.tickets (user_id, client_email, subject, priority, sla_due_at, category, instance_id, last_message_by_staff)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false)
		RETURNING id::text, COALESCE(user_id::text, ''), client_email, subject, status, priority,
		          assignee_id::text, instance_id::text, category, telegram_chat_id, sla_due_at, created_at, updated_at, last_message_at,
		          last_message_by_staff, answered_at, auto_close_at, is_return
	`, userID, email, subject, priority, sla, category, instID).Scan(
		&t.ID, &t.UserID, &t.ClientEmail, &t.Subject, &t.Status, &t.Priority,
		&t.AssigneeID, &t.InstanceID, &t.Category, &t.TelegramChatID, &t.SLADueAt, &t.CreatedAt, &t.UpdatedAt, &t.LastMessageAt,
		&t.LastMessageByStaff, &t.AnsweredAt, &t.AutoCloseAt, &t.IsReturn,
	)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO support.ticket_messages (ticket_id, author_id, author_email, is_staff, body)
		VALUES ($1, $2, $3, false, $4)
	`, t.ID, userID, email, message); err != nil {
		return nil, err
	}

	return &t, tx.Commit(ctx)
}

// CreateGuestTicket opens a ticket without user_id (e.g. email verification / pre-login via Telegram).
func (s *Store) CreateGuestTicket(ctx context.Context, email, subject, message, category string, telegramChatID int64) (*Ticket, error) {
	if category == "" {
		category = "login"
	}
	priority := "normal"
	now := time.Now().UTC()
	sla := SLADue(priority, now)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var t Ticket
	err = tx.QueryRow(ctx, `
		INSERT INTO support.tickets (user_id, client_email, subject, priority, sla_due_at, category, telegram_chat_id, last_message_by_staff)
		VALUES (NULL, $1, $2, $3, $4, $5, $6, false)
		RETURNING id::text, COALESCE(user_id::text, ''), client_email, subject, status, priority,
		          assignee_id::text, instance_id::text, category, telegram_chat_id, sla_due_at, created_at, updated_at, last_message_at,
		          last_message_by_staff, answered_at, auto_close_at, is_return
	`, email, subject, priority, sla, category, telegramChatID).Scan(
		&t.ID, &t.UserID, &t.ClientEmail, &t.Subject, &t.Status, &t.Priority,
		&t.AssigneeID, &t.InstanceID, &t.Category, &t.TelegramChatID, &t.SLADueAt, &t.CreatedAt, &t.UpdatedAt, &t.LastMessageAt,
		&t.LastMessageByStaff, &t.AnsweredAt, &t.AutoCloseAt, &t.IsReturn,
	)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO support.ticket_messages (ticket_id, author_id, author_email, is_staff, body)
		VALUES ($1, NULL, $2, false, $3)
	`, t.ID, email, message); err != nil {
		return nil, err
	}

	return &t, tx.Commit(ctx)
}

func (s *Store) OpenGuestTicketByChat(ctx context.Context, telegramChatID int64) (*Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, ticketSelectSQL+`
		WHERE telegram_chat_id = $1
		  AND user_id IS NULL
		  AND status <> 'closed'
		ORDER BY last_message_at DESC
		LIMIT 1
	`, telegramChatID))
}

func scanTicket(row pgx.Row) (*Ticket, error) {
	var t Ticket
	err := row.Scan(
		&t.ID, &t.UserID, &t.ClientEmail, &t.Subject, &t.Status, &t.Priority,
		&t.AssigneeID, &t.InstanceID, &t.Category, &t.TelegramChatID, &t.SLADueAt, &t.CreatedAt, &t.UpdatedAt, &t.LastMessageAt,
		&t.LastMessageByStaff, &t.AnsweredAt, &t.AutoCloseAt, &t.IsReturn,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListUserTickets(ctx context.Context, userID string) ([]Ticket, error) {
	rows, err := s.pool.Query(ctx, ticketSelectSQL+`
		WHERE user_id = $1
		ORDER BY last_message_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *t)
	}
	return items, rows.Err()
}

func (s *Store) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1`, ticketID))
}

func (s *Store) ListMessages(ctx context.Context, ticketID string) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, ticket_id::text, COALESCE(author_id::text, ''), author_email, is_staff, body, created_at, edited_at
		FROM support.ticket_messages
		WHERE ticket_id = $1
		ORDER BY created_at ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.TicketID, &m.AuthorID, &m.AuthorEmail, &m.IsStaff, &m.Body, &m.CreatedAt, &m.EditedAt); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (s *Store) UpdateMessage(ctx context.Context, ticketID, messageID, body string) (*Message, error) {
	body = NormalizeMessageBody(body)
	if IsEmptyMessageBody(body) {
		return nil, fmt.Errorf("empty body")
	}
	var m Message
	err := s.pool.QueryRow(ctx, `
		UPDATE support.ticket_messages
		SET body = $3, edited_at = now()
		WHERE id = $1::uuid AND ticket_id = $2::uuid
		RETURNING id::text, ticket_id::text, COALESCE(author_id::text, ''), author_email, is_staff, body, created_at, edited_at
	`, messageID, ticketID, body).Scan(
		&m.ID, &m.TicketID, &m.AuthorID, &m.AuthorEmail, &m.IsStaff, &m.Body, &m.CreatedAt, &m.EditedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) DeleteMessage(ctx context.Context, ticketID, messageID string) error {
	res, err := s.pool.Exec(ctx, `
		DELETE FROM support.ticket_messages
		WHERE id = $1::uuid AND ticket_id = $2::uuid
	`, messageID, ticketID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListStaffTickets(ctx context.Context, filter, statusFilter, assigneeID string) ([]Ticket, error) {
	if err := s.ProcessExpiredTickets(ctx); err != nil {
		return nil, err
	}
	if !validStaffTicketFilter(filter) {
		filter = ""
	}
	if statusFilter != "" && !validTicketStatus(statusFilter) {
		return nil, ErrInvalidTicketStatus
	}

	var (
		rows pgx.Rows
		err  error
	)
	switch filter {
	case "queue":
		rows, err = s.pool.Query(ctx, ticketSelectSQL+`
			WHERE status = 'open' AND assignee_id IS NULL
			AND ($1 = '' OR status = $1)
			ORDER BY
			  CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END DESC,
			  sla_due_at ASC, created_at ASC
			LIMIT 200
		`, statusFilter)
	case "mine":
		rows, err = s.pool.Query(ctx, ticketSelectSQL+`
			WHERE assignee_id = $1 AND status NOT IN ('resolved', 'closed', 'answered')
			AND ($2 = '' OR status = $2)
			ORDER BY
			  CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END DESC,
			  sla_due_at ASC, updated_at DESC
			LIMIT 200
		`, assigneeID, statusFilter)
	default:
		rows, err = s.pool.Query(ctx, ticketSelectSQL+`
			WHERE ($1 = '' OR status = $1)
			ORDER BY last_message_at DESC
			LIMIT 200
		`, statusFilter)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *t)
	}
	return items, rows.Err()
}

func (s *Store) ClaimNext(ctx context.Context, assigneeID string) (*Ticket, error) {
	if err := s.ProcessExpiredTickets(ctx); err != nil {
		return nil, err
	}
	shift, err := s.GetShift(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	if !shift.OnDuty {
		return nil, nil
	}
	_, _ = s.PromoteReturnPending(ctx, assigneeID)
	return s.claimNextUnlocked(ctx, assigneeID)
}

func (s *Store) TakeTicket(ctx context.Context, ticketID, assigneeID string) (*Ticket, error) {
	cfg, err := s.GetSlotConfig(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.CountSlots(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	if usage.NewInProgress >= cfg.MaxNewSlots {
		return nil, pgx.ErrNoRows
	}

	res, err := s.pool.Exec(ctx, `
		UPDATE support.tickets
		SET assignee_id = $2,
		    status = CASE WHEN status = 'answered' THEN 'answered' ELSE 'in_progress' END,
		    is_return = false,
		    updated_at = now()
		WHERE id = $1
		  AND assignee_id IS NULL
		  AND status IN ('open', 'answered')
	`, ticketID, assigneeID)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return s.GetTicket(ctx, ticketID)
}

func (s *Store) ReleaseTicket(ctx context.Context, ticketID, staffID string) (*Ticket, error) {
	ticket, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.AssigneeID == nil || *ticket.AssigneeID == "" {
		return nil, pgx.ErrNoRows
	}
	if ticket.Status == "closed" || ticket.Status == "resolved" || ticket.Status == "answered" {
		return nil, pgx.ErrNoRows
	}

	prevAssignee := *ticket.AssigneeID
	res, err := s.pool.Exec(ctx, `
		UPDATE support.tickets
		SET assignee_id = NULL,
		    status = 'open',
		    is_return = false,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND assignee_id IS NOT NULL
		  AND status NOT IN ('closed', 'resolved', 'answered')
	`, ticketID)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	// Refill other queue tickets for the previous assignee, but never the one
	// just released — otherwise FillSlots immediately reclaims it.
	_, _ = s.FillSlotsExcluding(ctx, prevAssignee, ticketID)
	_ = staffID
	return s.GetTicket(ctx, ticketID)
}

func (s *Store) UpdateTicket(ctx context.Context, ticketID, status, priority string) (*Ticket, error) {
	if status == "" && priority == "" {
		return s.GetTicket(ctx, ticketID)
	}
	var sla *time.Time
	if priority != "" {
		due := SLADue(priority, time.Now().UTC())
		sla = &due
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE support.tickets SET
		  status = COALESCE(NULLIF($2, ''), status),
		  priority = COALESCE(NULLIF($3, ''), priority),
		  sla_due_at = COALESCE($4, sla_due_at),
		  assignee_id = CASE WHEN NULLIF($2, '') IN ('closed', 'resolved') THEN NULL ELSE assignee_id END,
		  is_return = CASE WHEN NULLIF($2, '') IN ('closed', 'resolved') THEN false ELSE is_return END,
		  updated_at = now()
		WHERE id = $1
	`, ticketID, status, priority, sla)
	if err != nil {
		return nil, err
	}
	return s.GetTicket(ctx, ticketID)
}
