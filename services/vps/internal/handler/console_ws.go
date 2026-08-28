package handler

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/openstack"
	"github.com/gorilla/websocket"
)

func (h *Handler) consoleProxyWSURL(r *http.Request, instanceID string) string {
	site := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/")
	var origin string
	switch {
	case site != "":
		origin = strings.Replace(strings.Replace(site, "https://", "wss://", 1), "http://", "ws://", 1)
	case r.TLS != nil:
		origin = "wss://" + r.Host
	default:
		origin = "ws://" + r.Host
	}
	return origin + "/api/v1/instances/" + instanceID + "/console/ws"
}

// redactURL removes query strings that may contain session tokens from logs.
func redactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "?"); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

func (h *Handler) InstanceConsoleWS(w http.ResponseWriter, r *http.Request) {
	instanceID := strings.TrimSpace(r.PathValue("id"))
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}

	wsToken := consoleWSTokenFromRequest(r)
	if wsToken == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, ok := verifyConsoleWSToken(h.jwtSecret, wsToken, instanceID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if _, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID); err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if !h.requireBillingAccess(w, r, instanceID) {
		return
	}

	externalID, err := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	session, err := h.hv().GetConsole(r.Context(), externalID)
	if err != nil {
		log.Printf("console ws %s: %v", instanceID, err)
		writeError(w, http.StatusBadGateway, "console unavailable")
		return
	}
	upstreamURL := strings.TrimSpace(session.URL)
	if upstreamURL == "" {
		writeError(w, http.StatusBadGateway, "console unavailable")
		return
	}

	clientConn, err := func() (*websocket.Conn, error) {
		upgrader := websocket.Upgrader{
			CheckOrigin: consoleOriginAllowed,
		}
		if wsToken != "" {
			upgrader.Subprotocols = []string{"vps-ws." + wsToken}
		}
		return upgrader.Upgrade(w, r, nil)
	}()
	if err != nil {
		log.Printf("console ws upgrade %s (origin=%q): %v", instanceID, r.Header.Get("Origin"), err)
		return
	}
	defer clientConn.Close()

	dialer := websocket.Dialer{
		TLSClientConfig:   openstack.TLSConfig(),
		HandshakeTimeout:  15 * time.Second,
		EnableCompression: false,
		ReadBufferSize:    32768,
		WriteBufferSize:   32768,
		Subprotocols:      []string{"binary"},
	}
	upstreamHdr := http.Header{}
	if site := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/"); site != "" {
		upstreamHdr.Set("Origin", site)
	}
	upstreamConn, resp, err := dialer.Dial(upstreamURL, upstreamHdr)
	if err != nil {
		log.Printf("console ws dial %s (%s): %v", instanceID, redactURL(upstreamURL), err)
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream unavailable"))
		return
	}
	defer upstreamConn.Close()

	errCh := make(chan error, 2)
	relay := func(from, to *websocket.Conn) {
		for {
			msgType, msg, err := from.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if err := to.WriteMessage(msgType, msg); err != nil {
				errCh <- err
				return
			}
		}
	}
	go relay(clientConn, upstreamConn)
	go relay(upstreamConn, clientConn)
	<-errCh
}

func consoleWSTokenFromRequest(r *http.Request) string {
	if t := strings.TrimSpace(r.URL.Query().Get("ws_token")); t != "" {
		return t
	}
	proto := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Protocol"))
	for _, part := range strings.Split(proto, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "vps-ws.") {
			return strings.TrimPrefix(part, "vps-ws.")
		}
	}
	return ""
}
