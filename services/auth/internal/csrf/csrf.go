package csrf

import (
	"net/http"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

const HeaderName = "X-Requested-With"
const HeaderValue = "XMLHttpRequest"

// RequireBrowserHeader rejects cross-site cookie posts without a custom header in production.
func RequireBrowserHeader(r *http.Request) bool {
	if !prodenv.IsProduction() {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get(HeaderName)), HeaderValue)
}
