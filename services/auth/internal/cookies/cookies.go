package cookies

import (
	"net/http"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/staffcookie"
)

const RefreshCookieName = "vps_refresh"
const StaffCookieName = "vps_staff"

func refreshCookiePath() string {
	return "/"
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.EqualFold(os.Getenv("AUTH_COOKIE_SECURE"), "true")
}

func sameSiteMode() http.SameSite {
	if prodenv.IsProduction() {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}

func SetRefreshCookie(w http.ResponseWriter, r *http.Request, token string, maxAgeSec int) {
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     refreshCookiePath(),
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: sameSiteMode(),
	})
}

func ClearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: sameSiteMode(),
	})
}

func RefreshFromRequest(r *http.Request) string {
	c, err := r.Cookie(RefreshCookieName)
	if err != nil || c == nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func SetStaffCookie(w http.ResponseWriter, r *http.Request, userID, hmacSecret string, maxAgeSec int) {
	if userID == "" || strings.TrimSpace(hmacSecret) == "" {
		return
	}
	val, err := staffcookie.Sign(hmacSecret, userID, maxAgeSec)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     StaffCookieName,
		Value:    val,
		Path:     refreshCookiePath(),
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: sameSiteMode(),
	})
}

func ClearStaffCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     StaffCookieName,
		Value:    "",
		Path:     refreshCookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: sameSiteMode(),
	})
}
