package clientmeta

import (
	"net/http"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/clientip"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/ua"
)

func FromRequest(r *http.Request, authMethod string) store.SessionMeta {
	parsed := ua.Parse(r.UserAgent())
	ep := clientip.ClientEndpoint(r)
	if authMethod == "" {
		authMethod = "password"
	}
	return store.SessionMeta{
		IP:         ep.IP,
		ClientPort: ep.ClientPort,
		ServerPort: ep.ServerPort,
		UserAgent:  r.UserAgent(),
		Browser:    parsed.Browser,
		OS:         parsed.OS,
		DeviceType: parsed.DeviceType,
		AuthMethod: authMethod,
	}
}

func Connection(r *http.Request) store.ConnectionMeta {
	ep := clientip.ClientEndpoint(r)
	return store.ConnectionMeta{
		IP:         ep.IP,
		ClientPort: ep.ClientPort,
		ServerPort: ep.ServerPort,
	}
}

func ClientIP(r *http.Request) string {
	return clientip.ClientIP(r)
}
