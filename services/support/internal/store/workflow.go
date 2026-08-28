package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ProcessExpiredTickets(ctx context.Context) error {
	cfg, _ := s.GetSlotConfig(ctx)
	// Soft-closed ("answered") tickets auto-close after auto_close_at,
	// or after answered_at + auto_close_hours when auto_close_at is missing.
	_, err := s.pool.Exec(ctx, `
		UPDATE support.tickets
		SET status = 'closed', assignee_id = NULL, updated_at = now(),
		    auto_close_at = NULL
		WHERE status = 'answered'
		  AND (
		    (auto_close_at IS NOT NULL AND auto_close_at <= now())
		    OR (
		      answered_at IS NOT NULL
		      AND answered_at + ($1::int * interval '1 hour') <= now()
		    )
		  )
	`, cfg.AutoCloseHours)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE support.tickets
		SET assignee_id = NULL, updated_at = now()
		WHERE status = 'answered'
		  AND answered_at IS NOT NULL
		  AND answered_at + ($1::int * interval '1 hour') <= now()
		  AND assignee_id IS NOT NULL
	`, cfg.RebindHours)
	return err
}

func (s *Store) PromoteReturnPending(ctx context.Context, assigneeID string) (*Ticket, error) {
	cfg, err := s.GetSlotConfig(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.CountSlots(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	if usage.ReturnInProgress >= cfg.MaxReturnSlots {
		return nil, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM support.tickets
		WHERE assignee_id = $1 AND status = 'return_pending'
		ORDER BY last_message_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, assigneeID).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE support.tickets
		SET status = 'in_progress', is_return = true, updated_at = now()
		WHERE id = $1
	`, id); err != nil {
		return nil, err
	}

	t, err := scanTicket(tx.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	return t, tx.Commit(ctx)
}

func (s *Store) FillSlots(ctx context.Context, assigneeID string) ([]*Ticket, error) {
	return s.FillSlotsExcluding(ctx, assigneeID)
}

func (s *Store) FillSlotsExcluding(ctx context.Context, assigneeID string, excludeIDs ...string) ([]*Ticket, error) {
	if err := s.ProcessExpiredTickets(ctx); err != nil {
		return nil, err
	}

	var claimed []*Ticket
	for {
		promoted, err := s.PromoteReturnPending(ctx, assigneeID)
		if err != nil {
			return claimed, err
		}
		if promoted != nil {
			claimed = append(claimed, promoted)
			continue
		}
		break
	}

	shift, _ := s.GetShift(ctx, assigneeID)
	if !shift.OnDuty {
		return claimed, nil
	}

	for {
		t, err := s.claimNextUnlocked(ctx, assigneeID, excludeIDs...)
		if err != nil {
			return claimed, err
		}
		if t == nil {
			break
		}
		claimed = append(claimed, t)
	}
	return claimed, nil
}

func (s *Store) claimNextUnlocked(ctx context.Context, assigneeID string, excludeIDs ...string) (*Ticket, error) {
	cfg, err := s.GetSlotConfig(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.CountSlots(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	if usage.NewInProgress >= cfg.MaxNewSlots {
		return nil, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	exclude := make([]string, 0, len(excludeIDs))
	for _, id := range excludeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			exclude = append(exclude, id)
		}
	}

	var id string
	if len(exclude) == 0 {
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM support.tickets
			WHERE status = 'open' AND assignee_id IS NULL
			ORDER BY
			  CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END DESC,
			  sla_due_at ASC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`).Scan(&id)
	} else {
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM support.tickets
			WHERE status = 'open' AND assignee_id IS NULL
			  AND NOT (id = ANY($1::uuid[]))
			ORDER BY
			  CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END DESC,
			  sla_due_at ASC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, exclude).Scan(&id)
	}
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE support.tickets
		SET assignee_id = $2, status = 'in_progress', is_return = false, updated_at = now()
		WHERE id = $1
	`, id, assigneeID); err != nil {
		return nil, err
	}

	t, err := scanTicket(tx.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	return t, tx.Commit(ctx)
}

func (s *Store) StaffReplyAndClose(ctx context.Context, ticketID, authorID, authorEmail, body, mode string, attachmentIDs []string) (*Ticket, error) {
	cfg, err := s.GetSlotConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ticket, err := scanTicket(tx.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1 FOR UPDATE`, ticketID))
	if err != nil {
		return nil, err
	}
	if ticket.Status != "in_progress" && ticket.Status != "return_pending" && ticket.Status != "answered" {
		return nil, pgx.ErrNoRows
	}
	// Soft-closed ("answered") tickets may lose assignee after rebind_hours.
	// Allow staff to follow up by auto-claiming on reply instead of 409.
	if ticket.AssigneeID == nil || *ticket.AssigneeID == "" {
		if ticket.Status != "answered" {
			return nil, pgx.ErrNoRows
		}
		ticket.AssigneeID = &authorID
	}

	prevAssignee := *ticket.AssigneeID
	tookOver := prevAssignee != authorID

	var messageID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO support.ticket_messages (ticket_id, author_id, author_email, is_staff, body)
		VALUES ($1, $2, $3, true, $4)
		RETURNING id::text
	`, ticketID, authorID, authorEmail, body).Scan(&messageID); err != nil {
		return nil, err
	}

	if err := s.linkAttachmentsToMessageTx(ctx, tx, ticketID, messageID, authorID, attachmentIDs, true); err != nil {
		return nil, err
	}

	switch mode {
	case "immediate":
		if _, err := tx.Exec(ctx, `
			UPDATE support.tickets SET
			  status = 'closed',
			  assignee_id = NULL,
			  is_return = false,
			  answered_at = NULL,
			  auto_close_at = NULL,
			  last_message_by_staff = true,
			  updated_at = now(),
			  last_message_at = now()
			WHERE id = $1
		`, ticketID); err != nil {
			return nil, err
		}
	case "answered":
		autoClose := now.Add(time.Duration(cfg.AutoCloseHours) * time.Hour)
		if _, err := tx.Exec(ctx, `
			UPDATE support.tickets SET
			  status = 'answered',
			  assignee_id = $4,
			  is_return = false,
			  answered_at = $2,
			  auto_close_at = $3,
			  last_message_by_staff = true,
			  updated_at = now(),
			  last_message_at = now()
			WHERE id = $1
		`, ticketID, now, autoClose, authorID); err != nil {
			return nil, err
		}
	default:
		return nil, pgx.ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if tookOver {
		_, _ = s.FillSlotsExcluding(ctx, prevAssignee, ticketID)
	}
	_, _ = s.FillSlots(ctx, authorID)
	return s.GetTicket(ctx, ticketID)
}

func (s *Store) handleClientReplyStatus(ctx context.Context, ticket *Ticket) (status string, assigneeID *string, isReturn bool, clearAnswered bool) {
	if ticket.Status == "closed" {
		return "closed", ticket.AssigneeID, false, false
	}
	// Client reply always returns the ticket to the shared queue.
	return "open", nil, false, true
}

func (s *Store) AddClientMessage(ctx context.Context, ticketID, authorID, authorEmail, body string, attachmentIDs []string) error {
	if err := s.ProcessExpiredTickets(ctx); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ticket, err := scanTicket(tx.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1 FOR UPDATE`, ticketID))
	if err != nil {
		return err
	}
	if ticket.Status == "closed" {
		return pgx.ErrNoRows
	}

	prevAssignee := ticket.AssigneeID

	var author any
	if authorID != "" {
		author = authorID
	}
	var messageID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO support.ticket_messages (ticket_id, author_id, author_email, is_staff, body)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id::text
	`, ticketID, author, authorEmail, body).Scan(&messageID); err != nil {
		return err
	}

	if authorID != "" {
		if err := s.linkAttachmentsToMessageTx(ctx, tx, ticketID, messageID, authorID, attachmentIDs, false); err != nil {
			return err
		}
	}

	newStatus, assignee, isReturn, clearAnswered := s.handleClientReplyStatus(ctx, ticket)

	var answeredAt, autoClose any
	if clearAnswered {
		answeredAt = nil
		autoClose = nil
	} else {
		answeredAt = ticket.AnsweredAt
		autoClose = ticket.AutoCloseAt
	}

	if _, err := tx.Exec(ctx, `
		UPDATE support.tickets SET
		  status = $2,
		  assignee_id = $3,
		  is_return = $4,
		  answered_at = $5,
		  auto_close_at = $6,
		  last_message_by_staff = false,
		  updated_at = now(),
		  last_message_at = now()
		WHERE id = $1
	`, ticketID, newStatus, assignee, isReturn, answeredAt, autoClose); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if assignee != nil && (newStatus == "return_pending" || newStatus == "in_progress") {
		_, _ = s.FillSlots(ctx, *assignee)
	}
	if prevAssignee != nil && (assignee == nil || (assignee != nil && *assignee != *prevAssignee)) {
		_, _ = s.FillSlots(ctx, *prevAssignee)
	}
	return nil
}

func (s *Store) ClientCloseTicket(ctx context.Context, ticketID, userID string) (*Ticket, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ticket, err := scanTicket(tx.QueryRow(ctx, ticketSelectSQL+` WHERE id = $1 FOR UPDATE`, ticketID))
	if err != nil {
		return nil, err
	}
	if ticket.UserID != userID {
		return nil, pgx.ErrNoRows
	}
	if ticket.Status != "answered" {
		return nil, pgx.ErrNoRows
	}

	assigneeID := ticket.AssigneeID

	if _, err := tx.Exec(ctx, `
		UPDATE support.tickets SET
		  status = 'closed',
		  assignee_id = NULL,
		  is_return = false,
		  answered_at = NULL,
		  auto_close_at = NULL,
		  updated_at = now()
		WHERE id = $1
	`, ticketID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if assigneeID != nil {
		_, _ = s.FillSlots(ctx, *assigneeID)
	}
	return s.GetTicket(ctx, ticketID)
}

func (s *Store) GetWorkspace(ctx context.Context, assigneeID string) ([]Ticket, error) {
	rows, err := s.pool.Query(ctx, ticketSelectSQL+`
		WHERE assignee_id = $1
		  AND status IN ('in_progress', 'return_pending')
		ORDER BY
		  CASE WHEN status = 'in_progress' THEN 0 ELSE 1 END,
		  is_return ASC,
		  updated_at DESC
	`, assigneeID)
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
