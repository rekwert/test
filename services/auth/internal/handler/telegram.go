package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/codes"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) internalTokenOK(r *http.Request) bool {
	want := telegramInternalToken()
	if want == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Internal-Token"))
	return got != "" && got == want
}

func telegramInternalToken() string {
	if t := strings.TrimSpace(os.Getenv("TELEGRAM_INTERNAL_TOKEN")); t != "" {
		return t
	}
	if prodenv.IsProduction() {
		return ""
	}
	return strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
}

// TelegramLinkRequest starts email verification to bind Telegram ↔ account.
// Called by telegram-bot with X-Internal-Token.
func (h *Handler) TelegramLinkRequest(w http.ResponseWriter, r *http.Request) {
	if !h.internalTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Email      string `json:"email"`
		TelegramID int64  `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRe.MatchString(req.Email) || req.TelegramID == 0 {
		writeError(w, http.StatusBadRequest, "email and telegram_id required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	if existing, err := h.store.GetTelegramID(r.Context(), user.ID); err == nil && existing != nil && *existing == req.TelegramID {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "already_linked": true})
		return
	}

	code, err := codes.Generate6Digit()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "code generation failed")
		return
	}
	expires := time.Now().Add(h.cfg.VerificationTTL)
	if err := h.store.CreateTelegramLinkCode(r.Context(), user.ID, req.TelegramID, codes.Hash(code), expires); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store code")
		return
	}

	locale := user.Locale
	if locale == "" {
		locale = "ru"
	}
	if err := h.notify.Send(r.Context(), notify.SendRequest{
		To:       user.Email,
		Template: "telegram_link",
		Locale:   locale,
		Data:     map[string]string{"code": code},
	}); err != nil {
		writeError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"expires_in": int(h.cfg.VerificationTTL.Seconds()),
	})
}

// TelegramLinkConfirm verifies the code and stores telegram_id on the user.
func (h *Handler) TelegramLinkConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.internalTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Code       string `json:"code"`
		TelegramID int64  `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if len(req.Code) != 6 || req.TelegramID == 0 {
		writeError(w, http.StatusBadRequest, "code and telegram_id required")
		return
	}

	userID, email, roles, err := h.store.ConfirmTelegramLink(r.Context(), req.TelegramID, codes.Hash(req.Code))
	if err != nil {
		if errors.Is(err, store.ErrLinkCode) {
			writeError(w, http.StatusBadRequest, "invalid or expired code")
			return
		}
		if errors.Is(err, store.ErrTelegramTaken) {
			writeError(w, http.StatusConflict, "telegram already linked")
			return
		}
		writeError(w, http.StatusInternalServerError, "confirm failed")
		return
	}

	access, exp, err := h.tokens.IssueAccess(userID, email, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"user_id":      userID,
		"email":        email,
		"roles":        roles,
		"access_token": access,
		"expires_at":   exp.UTC().Format(time.RFC3339),
	})
}

// TelegramResolve returns the linked user profile (no JWT — use TelegramBotSession).
func (h *Handler) TelegramResolve(w http.ResponseWriter, r *http.Request) {
	if !h.internalTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	raw := strings.TrimSpace(r.PathValue("telegram_id"))
	tgID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || tgID == 0 {
		writeError(w, http.StatusBadRequest, "invalid telegram_id")
		return
	}
	userID, email, roles, err := h.store.GetUserIDByTelegram(r.Context(), tgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not linked")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": userID,
		"email":   email,
		"roles":   roles,
		"linked":  true,
	})
}

// TelegramBotSession issues a short-lived access token for the telegram bot (internal only).
func (h *Handler) TelegramBotSession(w http.ResponseWriter, r *http.Request) {
	if !h.internalTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		TelegramID int64 `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TelegramID == 0 {
		writeError(w, http.StatusBadRequest, "telegram_id required")
		return
	}
	userID, email, roles, err := h.store.GetUserIDByTelegram(r.Context(), req.TelegramID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not linked")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	access, exp, err := h.tokens.IssueAccessWithTTL(userID, email, roles, 15*time.Minute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":      userID,
		"email":        email,
		"roles":        roles,
		"access_token": access,
		"expires_at":   exp.UTC().Format(time.RFC3339),
	})
}

// TelegramUnlink clears telegram_id for the authenticated user.
func (h *Handler) TelegramUnlink(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.store.UnlinkTelegram(r.Context(), claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "unlink failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// TelegramWebLinkStart creates a one-time deep-link for binding Telegram from the website.
func (h *Handler) TelegramWebLinkStart(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	botUser := strings.TrimSpace(strings.TrimPrefix(os.Getenv("TELEGRAM_BOT_USERNAME"), "@"))
	if botUser == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram bot not configured")
		return
	}

	if existing, err := h.store.GetTelegramID(r.Context(), claims.UserID); err == nil && existing != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"already_linked":  true,
			"telegram_linked": true,
		})
		return
	}

	raw, err := codes.GenerateHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	// Telegram start payload: 1-64 chars [A-Za-z0-9_-]
	payload := "wl" + raw
	expires := time.Now().Add(15 * time.Minute)
	if err := h.store.CreateTelegramWebLinkToken(r.Context(), claims.UserID, codes.Hash(payload), expires); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	url := "https://t.me/" + botUser + "?start=" + payload
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"url":        url,
		"expires_in": int(15 * time.Minute / time.Second),
	})
}

// TelegramWebLinkConfirm is called by the bot after /start wl...
func (h *Handler) TelegramWebLinkConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.internalTokenOK(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Token      string `json:"token"`
		TelegramID int64  `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || req.TelegramID == 0 {
		writeError(w, http.StatusBadRequest, "token and telegram_id required")
		return
	}

	userID, email, roles, err := h.store.ConfirmTelegramWebLink(r.Context(), req.TelegramID, codes.Hash(req.Token))
	if err != nil {
		if errors.Is(err, store.ErrLinkCode) {
			writeError(w, http.StatusBadRequest, "invalid or expired token")
			return
		}
		if errors.Is(err, store.ErrTelegramTaken) {
			writeError(w, http.StatusConflict, "telegram already linked")
			return
		}
		writeError(w, http.StatusInternalServerError, "confirm failed")
		return
	}

	access, exp, err := h.tokens.IssueAccess(userID, email, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"user_id":      userID,
		"email":        email,
		"roles":        roles,
		"access_token": access,
		"expires_at":   exp.UTC().Format(time.RFC3339),
	})
}
