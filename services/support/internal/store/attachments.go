package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Attachment struct {
	ID          string
	TicketID    string
	MessageID   *string
	UploaderID  string
	Filename    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
	CreatedAt   time.Time
}

const maxAttachmentsPerMessage = 5

func (s *Store) SaveAttachment(
	ctx context.Context,
	ticketID, uploaderID, filename, contentType string,
	data io.Reader,
	maxBytes int64,
	attachmentsDir string,
) (*Attachment, error) {
	if !allowedContentType(contentType) {
		return nil, fmt.Errorf("unsupported content type")
	}

	storageKey := filepath.Join(ticketID, randomToken()+"_"+sanitizeFilename(filename))
	absPath := filepath.Join(attachmentsDir, storageKey)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return nil, err
	}
	written, err := io.Copy(f, io.LimitReader(data, maxBytes+1))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(absPath)
		return nil, err
	}
	if closeErr != nil {
		_ = os.Remove(absPath)
		return nil, closeErr
	}
	if written > maxBytes {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("file too large")
	}

	var a Attachment
	err = s.pool.QueryRow(ctx, `
		INSERT INTO support.message_attachments
		  (ticket_id, uploader_id, filename, content_type, size_bytes, storage_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, ticket_id::text, message_id::text, uploader_id::text,
		          filename, content_type, size_bytes, storage_key, created_at
	`, ticketID, uploaderID, filename, contentType, written, storageKey).Scan(
		&a.ID, &a.TicketID, &a.MessageID, &a.UploaderID,
		&a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt,
	)
	if err != nil {
		_ = os.Remove(absPath)
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAttachment(ctx context.Context, attachmentID string) (*Attachment, error) {
	var a Attachment
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, ticket_id::text, message_id::text, uploader_id::text,
		       filename, content_type, size_bytes, storage_key, created_at
		FROM support.message_attachments
		WHERE id = $1
	`, attachmentID).Scan(
		&a.ID, &a.TicketID, &a.MessageID, &a.UploaderID,
		&a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) ListAttachmentsForTicket(ctx context.Context, ticketID string) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, ticket_id::text, message_id::text, uploader_id::text,
		       filename, content_type, size_bytes, storage_key, created_at
		FROM support.message_attachments
		WHERE ticket_id = $1 AND message_id IS NOT NULL
		ORDER BY created_at ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(
			&a.ID, &a.TicketID, &a.MessageID, &a.UploaderID,
			&a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (s *Store) LinkAttachmentsToMessage(
	ctx context.Context,
	ticketID, messageID, uploaderID string,
	attachmentIDs []string,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.linkAttachmentsToMessageTx(ctx, tx, ticketID, messageID, uploaderID, attachmentIDs, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) linkAttachmentsToMessageTx(
	ctx context.Context,
	tx pgx.Tx,
	ticketID, messageID, uploaderID string,
	attachmentIDs []string,
	allowAnyUploader bool,
) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	if len(attachmentIDs) > maxAttachmentsPerMessage {
		return fmt.Errorf("too many attachments")
	}

	for _, id := range attachmentIDs {
		var res pgconn.CommandTag
		var err error
		if allowAnyUploader {
			res, err = tx.Exec(ctx, `
				UPDATE support.message_attachments
				SET message_id = $3
				WHERE id = $1 AND ticket_id = $2 AND message_id IS NULL
			`, id, ticketID, messageID)
		} else {
			res, err = tx.Exec(ctx, `
				UPDATE support.message_attachments
				SET message_id = $3
				WHERE id = $1 AND ticket_id = $2 AND uploader_id = $4 AND message_id IS NULL
			`, id, ticketID, messageID, uploaderID)
		}
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return fmt.Errorf("attachment %s not found or already linked", id)
		}
	}
	return nil
}

func AttachmentPath(attachmentsDir, storageKey string) string {
	return filepath.Join(attachmentsDir, storageKey)
}

func allowedContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return true
	case ct == "application/pdf":
		return true
	case ct == "text/plain":
		return true
	case ct == "application/zip":
		return true
	case ct == "application/x-zip-compressed":
		return true
	default:
		return false
	}
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "file"
	}
	return out
}

func randomToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
