package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/apikey"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/clientmeta"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/codes"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/config"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/cookies"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/csrf"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/password"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/ratelimit"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/tokens"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type Handler struct {
	store   *store.Store
	tokens  *tokens.Manager
	notify  *notify.Client
	cfg     config.Config
	limiter *ratelimit.LoginLimiter
	grace   *ratelimit.RefreshGrace
}

func New(s *store.Store, t *tokens.Manager, n *notify.Client, cfg config.Config, limiter *ratelimit.LoginLimiter, grace *ratelimit.RefreshGrace) *Handler {
	return &Handler{store: s, tokens: t, notify: n, cfg: cfg, limiter: limiter, grace: grace}
}

type authResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	SessionID    string    `json:"session_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	User         userDTO   `json:"user"`
}

type userDTO struct {
	ID               string   `json:"id"`
	Email            string   `json:"email"`
	DisplayName      string   `json:"display_name"`
	Roles            []string `json:"roles"`
	EmailVerified    bool     `json:"email_verified"`
	Locale           string   `json:"locale"`
	NotifyEmail      bool     `json:"notify_email"`
	NotifyTelegram   bool     `json:"notify_telegram"`
	TelegramLinked   bool     `json:"telegram_linked"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type messageResponse struct {
	Message string `json:"message"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		Locale       string `json:"locale"`
		ReferralCode string `json:"referral_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Locale = normalizeLocale(req.Locale, r.Header.Get("Accept-Language"))
	if !emailRe.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if isBlockedEmailDomain(req.Email, h.cfg.BlockedEmailDomains) {
		writeError(w, http.StatusBadRequest, "email domain not allowed")
		return
	}
	ip := clientmeta.ClientIP(r)
	if h.limiter != nil {
		ok, err := h.limiter.AllowRegister(r.Context(), ip)
		if err != nil {
			if prodenv.IsProduction() {
				writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
				return
			}
		} else if !ok {
			writeError(w, http.StatusTooManyRequests, "too many registration attempts")
			return
		}
	} else if prodenv.IsProduction() {
		writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
		return
	}
	if err := password.Validate(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	user, err := h.store.CreateUser(r.Context(), req.Email, hash, req.Locale)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "create user failed")
		return
	}

	if _, err := h.store.EnsureReferralCode(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "referral setup failed")
		return
	}

	referralCode := strings.TrimSpace(req.ReferralCode)
	if referralCode != "" {
		referrerID, err := h.store.GetUserIDByReferralCode(r.Context(), referralCode)
		if err == nil && referrerID != user.ID {
			_ = h.store.CreateReferralRegistration(r.Context(), referrerID, user.ID)
		}
	}

	if err := h.sendEmailVerification(r.Context(), user, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send verification email")
		return
	}

	_ = h.store.InsertAuditLog(r.Context(), user.ID, "user.register", "user", user.ID, nil, clientmeta.Connection(r))

	h.writeTokens(r, r.Context(), w, http.StatusCreated, user, clientmeta.FromRequest(r, "registration"))
}

func (h *Handler) notifyRegistrationPromo(email string) {
	if h.notify == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	limit := h.cfg.RegistrationPromoLimit
	if limit <= 0 {
		limit = 200
	}

	n, err := h.store.CountUsersSince(ctx, h.cfg.RegistrationPromoStartedAt)
	if err != nil {
		log.Printf("registration promo count: %v", err)
		n = 0
	}

	body := fmt.Sprintf(
		"Подтвердил email: новых пользователей %d/%d\ne-mail: %s\nпредложи PROSTO-1 (7 дней бесплатно)",
		n, limit, email,
	)
	if err := h.notify.OpsAlert(ctx, "Новая регистрация", body); err != nil {
		log.Printf("registration promo alert: %v", err)
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	ip := clientmeta.ClientIP(r)
	if h.limiter != nil {
		ok, err := h.limiter.Allow(r.Context(), ip, req.Email)
		if err != nil {
			if prodenv.IsProduction() {
				writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
				return
			}
		} else if !ok {
			h.logLoginAttempt(r, req.Email, "", "rate_limited", false)
			writeError(w, http.StatusTooManyRequests, "too many login attempts")
			return
		}
	} else if prodenv.IsProduction() {
		writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if store.IsNotFound(err) {
			h.logLoginAttempt(r, req.Email, "", "invalid_credentials", false)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.logLoginAttempt(r, req.Email, user.ID, "invalid_credentials", false)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !user.EmailVerified {
		if err := h.sendEmailVerification(r.Context(), user, false); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to send verification email")
			return
		}
	}

	_ = h.store.InsertAuditLog(r.Context(), user.ID, "user.login", "user", user.ID, nil, clientmeta.Connection(r))
	h.logLoginAttempt(r, req.Email, user.ID, "", true)

	h.writeTokens(r, r.Context(), w, http.StatusOK, user, clientmeta.FromRequest(r, "password"))
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !csrf.RequireBrowserHeader(r) {
		writeError(w, http.StatusForbidden, "csrf check failed")
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		req.RefreshToken = cookies.RefreshFromRequest(r)
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	hash := tokens.HashRefresh(req.RefreshToken)
	userID, prevMeta, ok, err := h.store.ConsumeRefreshToken(r.Context(), hash)
	if err != nil || !ok {
		if uid, _, revoked, lookupErr := h.store.GetRefreshToken(r.Context(), hash); lookupErr == nil && revoked {
			if graceUID, graceOK := h.grace.UserID(r.Context(), hash); graceOK && graceUID == uid {
				user, userErr := h.store.GetUserByID(r.Context(), uid)
				if userErr == nil {
					meta, _ := h.store.GetRefreshTokenMeta(r.Context(), hash)
					if meta.AuthMethod == "" {
						meta = prevMeta
					}
					meta = clientmeta.FromRequest(r, meta.AuthMethod)
					h.writeTokens(r, r.Context(), w, http.StatusOK, user, meta)
					return
				}
			}
			if revErr := h.store.RevokeAllUserRefreshTokens(r.Context(), uid); revErr != nil {
				log.Printf("refresh reuse revoke all: %v", revErr)
			} else {
				log.Printf("refresh token reuse detected user=%s — all sessions revoked", uid)
				_ = h.store.InsertAuditLog(r.Context(), uid, "auth.refresh.reuse", "user", uid, nil, clientmeta.Connection(r))
				_ = h.store.InsertAuditLog(r.Context(), uid, "auth.sessions.revoke_all", "user", uid,
					map[string]any{"reason": "refresh_token_reuse"}, clientmeta.Connection(r))
			}
		}
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	h.grace.Remember(r.Context(), hash, userID)

	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	meta := clientmeta.FromRequest(r, prevMeta.AuthMethod)
	h.writeTokens(r, r.Context(), w, http.StatusOK, user, meta)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if !csrf.RequireBrowserHeader(r) {
		writeError(w, http.StatusForbidden, "csrf check failed")
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	refresh := strings.TrimSpace(req.RefreshToken)
	if refresh == "" {
		refresh = cookies.RefreshFromRequest(r)
	}
	var userID, sessionID string
	if refresh != "" {
		userID, sessionID, _ = h.store.RevokeRefreshToken(r.Context(), tokens.HashRefresh(refresh))
	}
	if userID == "" {
		if claims, err := h.bearerClaims(r); err == nil {
			userID = claims.UserID
			sessionID = strings.TrimSpace(r.Header.Get("X-Session-Id"))
		}
	}
	if userID != "" {
		meta := map[string]any{}
		if sessionID != "" {
			meta["session_id"] = sessionID
		}
		_ = h.store.InsertAuditLog(r.Context(), userID, "auth.logout", "user", userID, meta, clientmeta.Connection(r))
	}
	cookies.ClearRefreshCookie(w, r)
	cookies.ClearStaffCookie(w, r)
	writeJSON(w, http.StatusOK, messageResponse{Message: "logged out"})
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentID := strings.TrimSpace(r.Header.Get("X-Session-Id"))
	items, err := h.store.ListUserSessions(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	out := make([]map[string]any, 0, len(items))
	for _, sess := range items {
		out = append(out, map[string]any{
			"id":             sess.ID,
			"browser":        sess.Browser,
			"os":             sess.OS,
			"device":         sess.DeviceType,
			"method":         sess.AuthMethod,
			"ip":             sess.IP,
			"logged_in_at":   sess.LoggedInAt.UTC().Format(time.RFC3339),
			"last_active_at": sess.LastActiveAt.UTC().Format(time.RFC3339),
			"current":        sess.ID == currentID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID := r.PathValue("id")
	if err := h.store.RevokeSession(r.Context(), claims.UserID, sessionID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "auth.session.revoke", "session", sessionID,
		map[string]any{"user_id": claims.UserID}, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, messageResponse{Message: "session revoked"})
}

func (h *Handler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	currentID := strings.TrimSpace(r.Header.Get("X-Session-Id"))
	if currentID == "" {
		writeError(w, http.StatusBadRequest, "X-Session-Id header required")
		return
	}
	n, err := h.store.RevokeOtherSessions(r.Context(), claims.UserID, currentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "auth.sessions.revoke_others", "user", claims.UserID,
		map[string]any{"revoked_count": n, "kept_session_id": currentID}, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, messageResponse{Message: "other sessions revoked"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	if ak, ok := h.apiKeyIdentity(r); ok {
		user, err := h.store.GetUserByID(r.Context(), ak.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":              user.ID,
			"email":           user.Email,
			"display_name":    user.DisplayName,
			"roles":           user.Roles,
			"email_verified":  user.EmailVerified,
			"locale":          user.Locale,
			"notify_email":    user.NotifyEmail,
			"notify_telegram": user.NotifyTelegram,
			"telegram_linked": user.TelegramLinked,
			"api_key_scopes":  ak.Scopes,
		})
		return
	}

	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, toUserDTO(user))
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		DisplayName      *string `json:"display_name"`
		NotifyEmail      *bool   `json:"notify_email"`
		NotifyTelegram   *bool   `json:"notify_telegram"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	hasUpdate := false
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" || len(name) > 64 {
			writeError(w, http.StatusBadRequest, "display_name must be 1-64 characters")
			return
		}
		if err := h.store.UpdateDisplayName(r.Context(), claims.UserID, name); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		hasUpdate = true
	}
	if req.NotifyEmail != nil || req.NotifyTelegram != nil {
		if req.NotifyTelegram != nil && *req.NotifyTelegram {
			tgID, err := h.store.GetTelegramID(r.Context(), claims.UserID)
			if err != nil || tgID == nil {
				writeError(w, http.StatusConflict, "telegram_not_linked")
				return
			}
		}
		if err := h.store.UpdateNotificationPrefs(r.Context(), claims.UserID, req.NotifyEmail, req.NotifyTelegram); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		meta := map[string]any{}
		if req.NotifyEmail != nil {
			meta["notify_email"] = *req.NotifyEmail
		}
		if req.NotifyTelegram != nil {
			meta["notify_telegram"] = *req.NotifyTelegram
		}
		_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "user.notify_prefs", "user", claims.UserID, meta, clientmeta.Connection(r))
		hasUpdate = true
	}
	if !hasUpdate {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(user))
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := password.Validate(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}

	hash, err := password.Hash(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if err := h.store.UpdatePassword(r.Context(), claims.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if err := h.store.RevokeAllUserRefreshTokens(r.Context(), claims.UserID); err != nil {
		log.Printf("change password revoke sessions user=%s: %v", claims.UserID, err)
	} else {
		_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "auth.sessions.revoke_all", "user", claims.UserID,
			map[string]any{"reason": "password_change"}, clientmeta.Connection(r))
	}
	cookies.ClearStaffCookie(w, r)
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "user.change_password", "user", claims.UserID, nil, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, messageResponse{Message: "password updated"})
}

func (h *Handler) ListSSHKeys(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := h.store.ListSSHKeys(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		items = append(items, map[string]any{
			"id":         k.ID,
			"name":       k.Name,
			"public_key": k.PublicKey,
			"created_at": k.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": items})
}

func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := h.store.ListAPIKeys(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		row := map[string]any{
			"id":         k.ID,
			"name":       k.Name,
			"prefix":     k.KeyPrefix,
			"scopes":     k.Scopes,
			"created_at": k.CreatedAt.UTC().Format(time.RFC3339),
		}
		if k.LastUsedAt != nil {
			row["last_used_at"] = k.LastUsedAt.UTC().Format(time.RFC3339)
		}
		items = append(items, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": items})
}

func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	scopes, err := store.NormalizeAPIKeyScopes(req.Scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	n, err := h.store.CountActiveAPIKeys(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	if n >= store.MaxAPIKeysPerUser {
		writeError(w, http.StatusConflict, "api key limit reached")
		return
	}

	raw, prefix, hash, err := store.NewAPIKeySecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	key, err := h.store.CreateAPIKey(r.Context(), claims.UserID, req.Name, prefix, hash, scopes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save api key")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "user.api_key_create", "api_key", key.ID, map[string]any{
		"name":   key.Name,
		"scopes": key.Scopes,
	}, clientmeta.Connection(r))

	writeJSON(w, http.StatusCreated, map[string]any{
		"key": map[string]any{
			"id":         key.ID,
			"name":       key.Name,
			"prefix":     key.KeyPrefix,
			"scopes":     key.Scopes,
			"secret":     raw,
			"created_at": key.CreatedAt.UTC().Format(time.RFC3339),
		},
		"message": "copy the secret now — it will not be shown again",
	})
}

func (h *Handler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keyID := r.PathValue("id")
	if err := h.store.RevokeAPIKey(r.Context(), claims.UserID, keyID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	_ = h.store.InsertAuditLog(r.Context(), claims.UserID, "user.api_key_revoke", "api_key", keyID, nil, clientmeta.Connection(r))
	writeJSON(w, http.StatusOK, messageResponse{Message: "revoked"})
}

func (h *Handler) CreateSSHKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.PublicKey = strings.TrimSpace(req.PublicKey)
	if req.Name == "" || len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !store.ValidSSHPublicKey(req.PublicKey) {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}

	key, err := h.store.CreateSSHKey(r.Context(), claims.UserID, req.Name, req.PublicKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save key")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key": map[string]any{
			"id":         key.ID,
			"name":       key.Name,
			"public_key": key.PublicKey,
			"created_at": key.CreatedAt.UTC().Format(time.RFC3339),
		},
	})
}

func (h *Handler) DeleteSSHKey(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keyID := r.PathValue("id")
	if err := h.store.DeleteSSHKey(r.Context(), claims.UserID, keyID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete key")
		return
	}
	writeJSON(w, http.StatusOK, messageResponse{Message: "deleted"})
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Code) != 6 {
		writeError(w, http.StatusBadRequest, "6-digit code required")
		return
	}

	err = h.store.VerifyEmailCode(r.Context(), claims.UserID, codes.Hash(req.Code))
	if err != nil {
		if store.IsTooManyAttempts(err) {
			writeError(w, http.StatusTooManyRequests, "too many attempts")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	user, _ := h.store.GetUserByID(r.Context(), claims.UserID)
	if user != nil {
		go h.notifyRegistrationPromo(user.Email)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "email verified",
		"user":    toUserDTO(user),
	})
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	claims, err := h.bearerClaims(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.EmailVerified {
		writeJSON(w, http.StatusOK, messageResponse{Message: "email already verified"})
		return
	}

	if err := h.sendEmailVerification(r.Context(), user, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send email")
		return
	}
	writeJSON(w, http.StatusOK, messageResponse{Message: "verification code sent"})
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email  string `json:"email"`
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Locale = normalizeLocale(req.Locale, r.Header.Get("Accept-Language"))
	ip := clientmeta.ClientIP(r)
	if h.limiter != nil {
		ok, err := h.limiter.AllowForgotPassword(r.Context(), ip)
		if err != nil {
			if prodenv.IsProduction() {
				writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
				return
			}
		} else if !ok {
			writeError(w, http.StatusTooManyRequests, "too many reset requests")
			return
		}
	} else if prodenv.IsProduction() {
		writeError(w, http.StatusServiceUnavailable, "rate limit unavailable")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err == nil {
		_ = h.sendPasswordReset(r.Context(), user, req.Locale)
	}

	writeJSON(w, http.StatusOK, messageResponse{
		Message: "if the email exists, a reset code was sent",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if len(req.Code) != 6 {
		writeError(w, http.StatusBadRequest, "invalid code or password")
		return
	}
	if err := password.Validate(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	if err := h.store.ResetPasswordWithCode(r.Context(), user.ID, codes.Hash(req.Code), hash); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}
	if err := h.store.RevokeAllUserRefreshTokens(r.Context(), user.ID); err != nil {
		log.Printf("reset password revoke sessions user=%s: %v", user.ID, err)
	}
	cookies.ClearStaffCookie(w, r)
	_ = h.store.InsertAuditLog(r.Context(), user.ID, "user.reset_password", "user", user.ID, nil, clientmeta.Connection(r))

	writeJSON(w, http.StatusOK, messageResponse{Message: "password updated"})
}

func (h *Handler) sendEmailVerification(ctx context.Context, user *store.User, force bool) error {
	if !force {
		stale, err := h.store.EmailVerificationIsStale(ctx, user.ID, 2*time.Minute)
		if err != nil {
			return err
		}
		if !stale {
			return nil
		}
	}
	code, err := codes.Generate6Digit()
	if err != nil {
		return err
	}
	expires := time.Now().Add(h.cfg.VerificationTTL)
	if err := h.store.SaveEmailVerification(ctx, user.ID, codes.Hash(code), expires); err != nil {
		return err
	}
	return h.notify.Send(ctx, notify.SendRequest{
		To:       user.Email,
		Template: "email_verify",
		Locale:   user.Locale,
		Data:     map[string]string{"code": code},
	})
}

func (h *Handler) sendPasswordReset(ctx context.Context, user *store.User, locale string) error {
	if locale == "" {
		locale = user.Locale
	}
	code, err := codes.Generate6Digit()
	if err != nil {
		return err
	}
	expires := time.Now().Add(h.cfg.PasswordResetTTL)
	if err := h.store.SavePasswordReset(ctx, user.ID, codes.Hash(code), expires); err != nil {
		return err
	}
	return h.notify.Send(ctx, notify.SendRequest{
		To:       user.Email,
		Template: "password_reset",
		Locale:   locale,
		Data:     map[string]string{"code": code},
	})
}

func (h *Handler) writeTokens(r *http.Request, ctx context.Context, w http.ResponseWriter, status int, user *store.User, meta store.SessionMeta) {
	access, exp, err := h.tokens.IssueAccess(user.ID, user.Email, user.Roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}

	rawRefresh, hash, refreshExp, err := h.tokens.NewRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	sessionID, err := h.store.SaveRefreshToken(ctx, user.ID, hash, refreshExp, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token save failed")
		return
	}

	cookies.SetRefreshCookie(w, r, rawRefresh, int(h.cfg.RefreshTTL.Seconds()))
	if isStaff(user.Roles) {
		cookies.SetStaffCookie(w, r, user.ID, h.cfg.JWTSecret, int(h.cfg.RefreshTTL.Seconds()))
	} else {
		cookies.ClearStaffCookie(w, r)
	}

	resp := authResponse{
		AccessToken: access,
		SessionID:   sessionID,
		ExpiresAt:   exp,
		TokenType:   "Bearer",
		User:        toUserDTO(user),
	}
	if !prodenv.IsProduction() {
		resp.RefreshToken = rawRefresh
	}
	writeJSON(w, status, resp)
}

func toUserDTO(user *store.User) userDTO {
	return userDTO{
		ID:             user.ID,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		Roles:          user.Roles,
		EmailVerified:  user.EmailVerified,
		Locale:         user.Locale,
		NotifyEmail:    user.NotifyEmail,
		NotifyTelegram: user.NotifyTelegram,
		TelegramLinked: user.TelegramLinked,
	}
}

func (h *Handler) logLoginAttempt(r *http.Request, email, userID, reason string, success bool) {
	conn := clientmeta.Connection(r)
	_ = h.store.InsertLoginAttempt(r.Context(), store.LoginAttempt{
		Email:         email,
		UserID:        userID,
		Success:       success,
		FailureReason: reason,
		IP:            conn.IP,
		ClientPort:    conn.ClientPort,
		ServerPort:    conn.ServerPort,
		UserAgent:     r.UserAgent(),
	})
}

func isBlockedEmailDomain(email string, blocked []string) bool {
	if len(blocked) == 0 {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range blocked {
		if domain == d {
			return true
		}
	}
	return false
}

func normalizeLocale(locale, acceptLanguage string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "ru" || locale == "en" {
		return locale
	}
	if strings.HasPrefix(strings.ToLower(acceptLanguage), "ru") {
		return "ru"
	}
	return "en"
}

func (h *Handler) bearerClaims(r *http.Request) (*tokens.Claims, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, errors.New("missing bearer")
	}
	return h.tokens.ParseAccess(strings.TrimPrefix(auth, "Bearer "))
}

func (h *Handler) apiKeyIdentity(r *http.Request) (*store.APIKeyIdentity, bool) {
	raw := apikey.TokenFromRequest(r.Header.Get("Authorization"), r.Header.Get("X-Api-Key"))
	if raw == "" || !apikey.IsAPIKeyToken(raw) {
		return nil, false
	}
	ak, err := h.store.ResolveAPIKey(r.Context(), raw)
	if err != nil {
		return nil, false
	}
	return ak, true
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
