// Package api implements the HTTP interface: the agent report endpoint, the
// read-only public API and the admin API.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mby/fenghuo/internal/server/alert"
	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/store"
)

type Deps struct {
	Store      *store.Store
	Hub        *hub.Hub
	Alerts     *alert.Engine
	Notify     *notify.Notifier
	Web        http.Handler
	Trusted    []netip.Prefix
	KeepMinute time.Duration
	KeepHour   time.Duration
	DataDir    string
	Log        *slog.Logger
}

type Server struct {
	Deps
	SecurityAudit *SecurityAudit
	loginIP       *auth.Limiter
	loginAll      *auth.Limiter
	lastTOTP      atomic.Uint64
}

func New(d Deps) *Server {
	return &Server{
		Deps:          d,
		SecurityAudit: NewSecurityAudit(d.Store, d.Log),
		loginIP:       auth.NewLimiter(10, 15*time.Minute),
		loginAll:      auth.NewLimiter(100, 15*time.Minute),
	}
}

// PublicHandler serves agent reports and the read-only status page. When
// withAdmin is set the admin API is mounted on the same listener.
func (s *Server) PublicHandler(withAdmin bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/report", s.handleReport)
	s.mountPublic(mux, withAdmin)
	if withAdmin {
		s.mountAdmin(mux)
	}
	s.mountFallbacks(mux)
	return s.wrap(mux)
}

// AdminHandler serves the admin API and the full UI. It does not accept agent reports.
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	s.mountPublic(mux, true)
	s.mountAdmin(mux)
	s.mountFallbacks(mux)
	return s.wrap(mux)
}

func (s *Server) mountFallbacks(mux *http.ServeMux) {
	notFound := func(w http.ResponseWriter, _ *http.Request) { writeError(w, http.StatusNotFound, "not_found") }
	mux.HandleFunc("/api/", notFound)
	mux.HandleFunc("/admin/api/", notFound)
	mux.Handle("/", s.Web)
}

func (s *Server) wrap(h http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	h = cop.Handler(h)
	return securityHeaders(s.recoverer(h))
}

const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/admin/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Log.Error("panic", "path", r.URL.Path, "err", v)
				writeError(w, http.StatusInternalServerError, "internal_error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the peer address, honouring X-Forwarded-For only when the
// direct peer is a trusted proxy.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !s.trusted(peer) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		if !s.trusted(a) {
			return a.Unmap().String()
		}
	}
	return peer.Unmap().String()
}

func (s *Server) trusted(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range s.Trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (s *Server) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, err := netip.ParseAddr(host)
	return err == nil && s.trusted(peer) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeErrorMsg(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

// decodeJSON strictly decodes a size-limited JSON body.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "bad_request", err.Error())
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeErrorMsg(w, http.StatusBadRequest, "bad_request", "unexpected data after JSON body")
		return false
	}
	return true
}
