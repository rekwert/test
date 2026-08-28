package accesslog

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/clientip"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	Method     string
	Path       string
	StatusCode int
	ClientIP   string
	ClientPort *int
	ServerPort *int
	DurationMS int
	UserAgent  string
}

type Logger struct {
	pool   *pgxpool.Pool
	ch     chan Entry
	done   chan struct{}
	once   sync.Once
	public int
}

func New(pool *pgxpool.Pool) *Logger {
	public := 443
	if p := strings.TrimSpace(os.Getenv("GATEWAY_PUBLIC_PORT")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			public = n
		}
	}
	return &Logger{
		pool:   pool,
		ch:     make(chan Entry, 2048),
		done:   make(chan struct{}),
		public: public,
	}
}

func (l *Logger) Start(ctx context.Context) {
	go l.worker(ctx)
}

func (l *Logger) Close() {
	l.once.Do(func() {
		close(l.ch)
		<-l.done
	})
}

func (l *Logger) worker(ctx context.Context) {
	defer close(l.done)
	for e := range l.ch {
		l.insert(ctx, e)
	}
}

func (l *Logger) insert(ctx context.Context, e Entry) {
	if l.pool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var ipArg any
	if e.ClientIP != "" {
		ipArg = e.ClientIP
	}
	_, err := l.pool.Exec(ctx, `
		INSERT INTO auth.http_access_log (
			method, path, status_code, client_ip, client_port, server_port, duration_ms, user_agent
		) VALUES ($1, $2, $3, $4::inet, $5, $6, $7, NULLIF($8, ''))
	`, e.Method, e.Path, e.StatusCode, ipArg, e.ClientPort, e.ServerPort, e.DurationMS, e.UserAgent)
	if err != nil {
		log.Printf("access log insert: %v", err)
	}
}

func (l *Logger) log(e Entry) {
	select {
	case l.ch <- e:
	default:
		log.Printf("access log: queue full, dropping %s %s", e.Method, e.Path)
	}
}

func (l *Logger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		ep := clientip.ClientEndpoint(r)
		serverPort := ep.ServerPort
		if serverPort == nil && l.public > 0 {
			serverPort = &l.public
		}

		next.ServeHTTP(rw, r)

		l.log(Entry{
			Method:     r.Method,
			Path:       sanitizePath(r.URL.Path),
			StatusCode: rw.status,
			ClientIP:   ep.IP,
			ClientPort: ep.ClientPort,
			ServerPort: serverPort,
			DurationMS: int(time.Since(start).Milliseconds()),
			UserAgent:  truncate(r.UserAgent(), 512),
		})
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func skipPath(path string) bool {
	switch path {
	case "/health", "/ready", "/api/v1/health":
		return true
	}
	return false
}

func sanitizePath(path string) string {
	path = truncate(strings.TrimSpace(path), 512)
	if path == "" {
		return "/"
	}
	return path
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
