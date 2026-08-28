package handler

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/support/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/support/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ticketID := r.PathValue("id")
	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		log.Printf("get ticket: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}
	if !authn.IsStaff(claims.Roles) && ticket.UserID != claims.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if ticket.Status == "closed" {
		writeError(w, http.StatusConflict, "ticket is closed")
		return
	}

	if err := r.ParseMultipartForm(h.maxAttachmentBytes + 1024*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	if header.Size > h.maxAttachmentBytes {
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectContentType(header.Filename)
	}

	filename := header.Filename
	if filename == "" {
		filename = "attachment"
	}

	attachment, err := h.store.SaveAttachment(
		r.Context(),
		ticketID,
		claims.UserID,
		filename,
		contentType,
		io.LimitReader(file, h.maxAttachmentBytes+1),
		h.maxAttachmentBytes,
		h.attachmentsDir,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported content type") {
			writeError(w, http.StatusBadRequest, "unsupported file type")
			return
		}
		if strings.Contains(err.Error(), "file too large") {
			writeError(w, http.StatusBadRequest, "file too large")
			return
		}
		log.Printf("save attachment: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save attachment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"attachment": attachmentJSON(*attachment)})
}

func (h *Handler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	claims, err := h.claims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ticketID := r.PathValue("id")
	attachmentID := r.PathValue("attachmentId")

	ticket, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load ticket")
		return
	}
	if !authn.IsStaff(claims.Roles) && ticket.UserID != claims.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	attachment, err := h.store.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load attachment")
		return
	}
	if attachment.TicketID != ticketID {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	path := store.AttachmentPath(h.attachmentsDir, attachment.StorageKey)
	f, err := os.Open(path)
	if err != nil {
		log.Printf("open attachment: %v", err)
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Disposition", contentDisposition(attachment.Filename, attachment.ContentType))
	w.Header().Set("Content-Length", strconv.FormatInt(attachment.SizeBytes, 10))
	http.ServeContent(w, r, attachment.Filename, attachment.CreatedAt, f)
}

func attachmentJSON(a store.Attachment) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"ticket_id":    a.TicketID,
		"filename":     a.Filename,
		"content_type": a.ContentType,
		"size_bytes":   a.SizeBytes,
		"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func contentDisposition(filename, contentType string) string {
	safe := strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, filename)
	if safe == "" {
		safe = "attachment"
	}
	kind := "attachment"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		kind = "inline"
	}
	return kind + `; filename="` + safe + `"`
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
