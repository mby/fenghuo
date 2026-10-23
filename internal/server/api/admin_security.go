package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/store"
)

const securityAuditQueueSize = 1024

// Only the worker writes audit data. Overflow is counted and logged without
// request data; failed batches remain bounded and are retried on the next tick.
// Its independent context is canceled after HTTP shutdown so in-flight actions
// are included in the final bounded flush before storage closes.
type SecurityAudit struct {
	store   *store.Store
	log     *slog.Logger
	queue   chan store.SecurityAuditEvent
	mu      sync.RWMutex
	stopped bool
	running atomic.Bool
	dropped atomic.Uint64
}

func NewSecurityAudit(st *store.Store, log *slog.Logger) *SecurityAudit {
	if log == nil {
		log = slog.Default()
	}
	return &SecurityAudit{store: st, log: log, queue: make(chan store.SecurityAuditEvent, securityAuditQueueSize)}
}

func (a *SecurityAudit) Dropped() uint64 { return a.dropped.Load() }

func (a *SecurityAudit) Enqueue(event store.SecurityAuditEvent) bool {
	e, ok := store.NormalizeSecurityAudit(event)
	if !ok {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.stopped {
		a.dropped.Add(1)
		return false
	}
	select {
	case a.queue <- e:
		return true
	default:
		a.dropped.Add(1)
		return false
	}
}

func (a *SecurityAudit) Run(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return fmt.Errorf("security audit worker already started")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	pending := make([]store.SecurityAuditEvent, 0, 128)
	flush := func() error {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.store.SaveSecurityAudit(flushCtx, pending, time.Now().Unix())
	}
	if err := flush(); err != nil {
		a.log.Error("security audit maintenance failed", "err", err)
	}
	ticks := 0
	var reportedDrops uint64
	reportDrops := func() {
		if dropped := a.Dropped(); dropped != reportedDrops {
			a.log.Warn("security audit queue overflow", "dropped", dropped-reportedDrops)
			reportedDrops = dropped
		}
	}
	defer reportDrops()
	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			a.stopped = true
			a.mu.Unlock()
			for {
				select {
				case e := <-a.queue:
					pending = append(pending, e)
				default:
					return flush()
				}
			}
		case <-ticker.C:
			ticks++
			for len(pending) < 128 {
				select {
				case e := <-a.queue:
					pending = append(pending, e)
				default:
					goto drained
				}
			}
		drained:
			if len(pending) > 0 || ticks%60 == 0 {
				if err := flush(); err != nil {
					a.log.Error("security audit flush failed", "err", err)
				} else {
					pending = pending[:0]
				}
			}
			reportDrops()
		}
	}
}

type securityAuditWriter struct {
	http.ResponseWriter
	status int
}

func (w *securityAuditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *securityAuditWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *securityAuditWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Route patterns, never URLs or user-supplied strings, select audit actions.
func securityAction(pattern string) (action, targetKey string) {
	switch pattern {
	case "POST /admin/api/login":
		return "login", ""
	case "POST /admin/api/logout":
		return "logout", ""
	case "POST /admin/api/password":
		return "password.change", ""
	case "POST /admin/api/totp/enable":
		return "totp.enable", ""
	case "POST /admin/api/totp/disable":
		return "totp.disable", ""
	case "PUT /admin/api/settings":
		return "settings.update", ""
	case "POST /admin/api/nodes":
		return "node.create", ""
	case "PUT /admin/api/nodes/{id}":
		return "node.update", "id"
	case "DELETE /admin/api/nodes/{id}":
		return "node.delete", "id"
	case "POST /admin/api/nodes/{id}/token":
		return "node.token.rotate", "id"
	case "POST /admin/api/rules":
		return "rule.create", ""
	case "PUT /admin/api/rules/{id}":
		return "rule.update", "id"
	case "DELETE /admin/api/rules/{id}":
		return "rule.delete", "id"
	case "POST /admin/api/channels":
		return "channel.create", ""
	case "PUT /admin/api/channels/{id}":
		return "channel.update", "id"
	case "DELETE /admin/api/channels/{id}":
		return "channel.delete", "id"
	case "POST /admin/api/channels/{id}/test":
		return "channel.test", "id"
	case "DELETE /admin/api/sessions/{sessionID}":
		return "session.revoke", "sessionID"
	case "DELETE /admin/api/sessions":
		return "session.revoke_others", ""
	}
	return "", ""
}

func (s *Server) auditAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action, targetKey := securityAction(r.Pattern)
		if s.SecurityAudit == nil || action == "" {
			h(w, r)
			return
		}
		event := store.SecurityAuditEvent{CreatedAt: time.Now().Unix(), Action: action, IP: s.clientIP(r)}
		if targetKey != "" {
			event.Target = r.PathValue(targetKey)
		}
		out := &securityAuditWriter{ResponseWriter: w}
		completed := false
		defer func() {
			event.Status = out.status
			if event.Status == 0 {
				event.Status = http.StatusOK
				if !completed {
					event.Status = http.StatusInternalServerError
				}
			}
			s.SecurityAudit.Enqueue(event)
		}()
		h(out, r)
		completed = true
	}
}

func currentSessionHash(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return auth.HashToken(c.Value)
	}
	return ""
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.Store.ListSessions(r.Context(), currentSessionHash(r), time.Now().Unix())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sessionID")
	if !store.ValidSessionID(id) {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := s.Store.RevokeSession(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	// Revoking the current session is allowed. A stale cookie cannot authenticate.
	if !s.sessionOK(r) {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteStrictMode})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.RevokeOtherSessions(r.Context(), currentSessionHash(r)); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSecurityAudit(w http.ResponseWriter, r *http.Request) {
	// Bound parsing work as well as database work for this paginated endpoint.
	if len(r.URL.RawQuery) > 512 {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	for key, values := range q {
		if len(values) != 1 || (key != "action" && key != "outcome" && key != "before" && key != "limit") {
			writeError(w, http.StatusBadRequest, "bad_request")
			return
		}
	}
	f := store.SecurityAuditFilter{Action: q.Get("action"), Outcome: q.Get("outcome"), Limit: 25}
	if f.Action != "" && !store.ValidSecurityAction(f.Action) || f.Outcome != "" && f.Outcome != "success" && f.Outcome != "failure" {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if q.Has("before") {
		v, err := strconv.ParseInt(q.Get("before"), 10, 64)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "bad_request")
			return
		}
		f.Before = v
	}
	if q.Has("limit") {
		v, err := strconv.Atoi(q.Get("limit"))
		if err != nil || v < 1 || v > store.SecurityAuditPageLimit {
			writeError(w, http.StatusBadRequest, "bad_request")
			return
		}
		f.Limit = v
	}
	page, err := s.Store.ListSecurityAudit(r.Context(), f, time.Now().Unix())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
