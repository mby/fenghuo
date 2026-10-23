package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/store"
)

func securityTestServer(t *testing.T) *Server {
	t.Helper()
	e := setup(t)
	s := New(Deps{Store: e.st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	s.SecurityAudit = NewSecurityAudit(e.st, s.Log)
	return s
}

func securityRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	return r
}

func TestSecuritySessionHandlers(t *testing.T) {
	s := securityTestServer(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for _, token := range []string{"session-secret", "other-secret"} {
		if err := s.Store.CreateSession(ctx, auth.HashToken(token), now, now+3600, "192.0.2.1", "browser"); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/sessions", s.requireAdmin(s.handleListSessions))
	mux.HandleFunc("DELETE /admin/api/sessions/{sessionID}", s.requireAdmin(s.auditAdmin(s.handleRevokeSession)))
	mux.HandleFunc("DELETE /admin/api/sessions", s.requireAdmin(s.auditAdmin(s.handleRevokeOtherSessions)))
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/admin/api/sessions", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatal("sessions accessible without authentication")
	}
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, securityRequest("GET", "/admin/api/sessions"))
	var sessions []store.Session
	if err := json.Unmarshal(out.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions: %+v", sessions)
	}
	var current string
	for _, session := range sessions {
		if session.Current {
			current = session.ID
		}
	}
	if current == "" || strings.Contains(out.Body.String(), "secret") || strings.Contains(out.Body.String(), auth.HashToken("session-secret")) {
		t.Fatal("current detection or credential exclusion failed")
	}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/admin/api/sessions/invalid", 400},
		{"/admin/api/sessions/00000000000000000000000000000000", 404},
		{"/admin/api/sessions", 204},
		{"/admin/api/sessions/" + current, 204},
	} {
		out = httptest.NewRecorder()
		mux.ServeHTTP(out, securityRequest("DELETE", tc.path))
		if out.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, out.Code, out.Body.String())
		}
		if tc.path == "/admin/api/sessions" {
			valid, err := s.Store.SessionValid(ctx, auth.HashToken("session-secret"), now)
			other, _ := s.Store.SessionValid(ctx, auth.HashToken("other-secret"), now)
			if err != nil || !valid || other {
				t.Fatalf("others revocation: %v %v %v", valid, other, err)
			}
		}
	}
	if !strings.Contains(out.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatal("current-session revocation did not clear cookie")
	}
	out = httptest.NewRecorder()
	mux.ServeHTTP(out, securityRequest("GET", "/admin/api/sessions"))
	if out.Code != http.StatusUnauthorized {
		t.Fatal("revoked session still authenticates")
	}
}

func TestSecurityAuditMiddlewareExcludesSecrets(t *testing.T) {
	s := securityTestServer(t)
	var events []store.SecurityAuditEvent
	for _, tc := range []struct {
		method, pattern, path, action, target string
		status                                int
	}{
		{"POST", "POST /admin/api/login", "/admin/api/login?password=query-secret", "login", "", 401},
		{"POST", "POST /admin/api/nodes/{id}/token", "/admin/api/nodes/23/token", "node.token.rotate", "23", 200},
		{"PUT", "PUT /admin/api/nodes/{id}", "/admin/api/nodes/password-secret", "node.update", "", 400},
		{"POST", "POST /admin/api/totp/enable", "/admin/api/totp/enable", "totp.enable", "", 204},
		{"GET", "GET /admin/api/totp/setup", "/admin/api/totp/setup", "", "", 200},
		{"POST", "POST /admin/api/unknown", "/admin/api/unknown?token=secret", "", "", 400},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(tc.pattern, s.auditAdmin(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != "password-body-secret" {
					t.Fatal("middleware read or replaced body")
				}
				w.Header().Set("Set-Cookie", "response-secret")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "totp-response-secret")
			}))
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("password-body-secret"))
			r.Header.Set("Authorization", "Bearer header-secret")
			r.Header.Set("Cookie", "fenghuo_session=cookie-secret")
			r.Header.Set("User-Agent", "user-agent-secret")
			r.Header.Set("X-Forwarded-For", "spoofed-secret")
			r.RemoteAddr = "192.0.2.42:1234"
			mux.ServeHTTP(httptest.NewRecorder(), r)
			select {
			case event := <-s.SecurityAudit.queue:
				events = append(events, event)
				if tc.action == "" {
					t.Fatal("unallowlisted route audited")
				}
				if event.Action != tc.action || event.Target != tc.target || event.Status != tc.status || event.IP != "192.0.2.42" {
					t.Fatalf("event: %+v", event)
				}
				encoded, _ := json.Marshal(event)
				if strings.Contains(string(encoded), "secret") {
					t.Fatalf("secret leaked: %s", encoded)
				}
			default:
				if tc.action != "" {
					t.Fatal("missing audit event")
				}
			}
		})
	}
	if err := s.Store.SaveSecurityAudit(context.Background(), events, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.ListSecurityAudit(context.Background(), store.SecurityAuditFilter{}, time.Now().Unix())
	if err != nil || len(stored.Items) != len(events) {
		t.Fatalf("stored audit: %+v %v", stored, err)
	}
	encoded, _ := json.Marshal(stored)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("stored secret: %s", encoded)
	}
}

func TestSecurityAuditResponseStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    http.HandlerFunc
		want int
	}{
		{"implicit", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }, 200},
		{"empty", func(http.ResponseWriter, *http.Request) {}, 200},
		{"first_header", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403); w.WriteHeader(200) }, 403},
		{"panic", func(http.ResponseWriter, *http.Request) { panic("failure") }, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := securityTestServer(t)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /admin/api/login", s.auditAdmin(tc.h))
			s.recoverer(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/admin/api/login", nil))
			select {
			case event := <-s.SecurityAudit.queue:
				if event.Status != tc.want {
					t.Fatalf("status: %d", event.Status)
				}
			default:
				t.Fatal("missing audit")
			}
		})
	}
}

func TestSecurityRoutesPrivate(t *testing.T) {
	e := setup(t)
	for _, path := range []string{"/admin/api/sessions", "/admin/api/audit"} {
		if response, _ := e.do(t, "GET", e.pub.URL+path, nil, nil); response.StatusCode != http.StatusNotFound {
			t.Fatalf("private route exposed: %s", path)
		}
		if response, _ := e.do(t, "GET", e.adm.URL+path, nil, nil); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("private route unprotected: %s", path)
		}
	}
	for _, path := range []string{"/admin/api/sessions", "/admin/api/sessions/00000000000000000000000000000000"} {
		if response, _ := e.do(t, "DELETE", e.adm.URL+path, nil, nil); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("mutation unprotected: %s", path)
		}
	}
	response, _ := e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("login failed")
	}
	response, _ = e.do(t, "DELETE", e.adm.URL+"/admin/api/sessions", nil, map[string]string{"Sec-Fetch-Site": "cross-site"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("cross-origin session mutation allowed")
	}
}

func TestSecurityAuditFailedLoginAndAuthenticationBoundary(t *testing.T) {
	s := securityTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/api/login", s.auditAdmin(s.handleLogin))
	mux.HandleFunc("PUT /admin/api/settings", s.requireAdmin(s.auditAdmin(s.handlePutSettings)))
	for _, tc := range []struct {
		path, method, body string
		status, events     int
	}{
		{"/admin/api/login", "POST", `{"username":"admin","password":"wrong"}`, 401, 1},
		{"/admin/api/login", "POST", `{"unknown":"secret"}`, 400, 2},
		{"/admin/api/settings", "PUT", `{}`, 401, 2},
	} {
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if out.Code != tc.status || len(s.SecurityAudit.queue) != tc.events {
			t.Fatalf("%s: status=%d events=%d", tc.path, out.Code, len(s.SecurityAudit.queue))
		}
	}
}

func TestSecurityAuditQueueBoundAndShutdown(t *testing.T) {
	s := securityTestServer(t)
	a := s.SecurityAudit
	event := store.SecurityAuditEvent{CreatedAt: time.Now().Unix(), Action: "login", Status: 401}
	for i := 0; i < securityAuditQueueSize+5; i++ {
		if accepted := a.Enqueue(event); accepted != (i < securityAuditQueueSize) {
			t.Fatalf("queue acceptance %d: %v", i, accepted)
		}
	}
	if a.Dropped() != 5 {
		t.Fatalf("overflow not counted: %d", a.Dropped())
	}
	page, err := s.Store.ListSecurityAudit(context.Background(), store.SecurityAuditFilter{}, time.Now().Unix())
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("enqueue wrote database: %+v %v", page, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatal(err)
	}
	total := 0
	var before int64
	for {
		page, err := s.Store.ListSecurityAudit(context.Background(), store.SecurityAuditFilter{Before: before, Limit: 100}, time.Now().Unix())
		if err != nil {
			t.Fatal(err)
		}
		total += len(page.Items)
		before = page.NextBefore
		if before == 0 {
			break
		}
	}
	if total != securityAuditQueueSize {
		t.Fatalf("shutdown lost events: %d", total)
	}
	if a.Enqueue(event) {
		t.Fatal("closed worker accepted event")
	}
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("worker ran twice")
	}
}

func TestSecurityAuditQueryBounds(t *testing.T) {
	s := securityTestServer(t)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", 200}, {"?limit=100", 200}, {"?limit=101", 400}, {"?limit=0", 400},
		{"?before=-1", 400}, {"?before=wat", 400}, {"?action=secret", 400},
		{"?action=%ZZ", 400}, {"?action=login;outcome=failure", 400},
		{"?outcome=failure&action=login", 200}, {"?outcome=wat", 400},
		{"?limit=1&limit=2", 400}, {"?password=secret", 400}, {"?action=" + strings.Repeat("a", 512), 400},
	} {
		out := httptest.NewRecorder()
		s.handleListSecurityAudit(out, httptest.NewRequest("GET", "/admin/api/audit"+tc.query, nil))
		if out.Code != tc.status {
			t.Fatalf("%s: %d", tc.query, out.Code)
		}
	}
}

func TestSecurityAuditConcurrentShutdown(t *testing.T) {
	s := securityTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.SecurityAudit.Run(ctx) }()
	var producers sync.WaitGroup
	for range 8 {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for range 100 {
				s.SecurityAudit.Enqueue(store.SecurityAuditEvent{CreatedAt: time.Now().Unix(), Action: "login", Status: 401})
			}
		}()
	}
	cancel()
	producers.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("audit shutdown blocked")
	}
	count := 0
	var before int64
	for {
		page, err := s.Store.ListSecurityAudit(context.Background(), store.SecurityAuditFilter{Before: before, Limit: 100}, time.Now().Unix())
		if err != nil {
			t.Fatal(err)
		}
		count += len(page.Items)
		before = page.NextBefore
		if before == 0 {
			break
		}
	}
	if uint64(count)+s.SecurityAudit.Dropped() != 800 {
		t.Fatalf("unaccounted events: saved=%d dropped=%d", count, s.SecurityAudit.Dropped())
	}
}
