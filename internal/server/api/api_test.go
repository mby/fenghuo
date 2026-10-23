package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/alert"
	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/store"
)

type env struct {
	pub, adm *httptest.Server
	client   *http.Client
	st       *store.Store
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.CreateAdmin(ctx, "admin", auth.HashPassword("secret-password")); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New(time.Now())
	n := notify.New(st, false, log)
	al := alert.New(h, st, n, log)
	if err := al.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Store: st, Hub: h, Alerts: al, Notify: n, Log: log, DataDir: dir, KeepMinute: 7 * 24 * time.Hour,
		Web:     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "spa") }),
		Trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	})
	e := &env{pub: httptest.NewServer(s.PublicHandler(false)), adm: httptest.NewServer(s.AdminHandler()), st: st}
	t.Cleanup(e.pub.Close)
	t.Cleanup(e.adm.Close)
	jar, _ := cookiejar.New(nil)
	e.client = &http.Client{Jar: jar}
	return e
}

func (e *env) do(t *testing.T, method, url string, body any, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestEndToEnd(t *testing.T) {
	e := setup(t)

	// Admin API is not reachable on the public listener.
	if resp, _ := e.do(t, "POST", e.pub.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin login on public listener: %d", resp.StatusCode)
	}
	// Reports are not accepted on the admin listener.
	if resp, _ := e.do(t, "POST", e.adm.URL+"/api/v1/report", proto.Report{V: 1}, map[string]string{"Authorization": "Bearer x"}); resp.StatusCode == http.StatusNoContent {
		t.Fatal("admin listener accepted a report")
	}

	if resp, _ := e.do(t, "GET", e.adm.URL+"/admin/api/nodes", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin request: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "wrong"}, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}
	resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	if c := resp.Header.Get("Set-Cookie"); !strings.Contains(c, "HttpOnly") || !strings.Contains(c, "SameSite=Strict") {
		t.Fatalf("cookie flags: %s", c)
	}

	// Cross-origin state-changing requests are rejected.
	if resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{"name": "x"}, map[string]string{"Sec-Fetch-Site": "cross-site"}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin request: %d", resp.StatusCode)
	}

	resp, body := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{"name": "tokyo", "group": "jp"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create node: %d %s", resp.StatusCode, body)
	}
	var created struct {
		Node  adminNode `json:"node"`
		Token string    `json:"token"`
	}
	json.Unmarshal(body, &created)
	if !strings.HasPrefix(created.Token, "fh_") {
		t.Fatalf("token: %q", created.Token)
	}
	_, body = e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{"name": "hidden", "hidden": true}, nil)
	var hidden struct {
		Token string `json:"token"`
	}
	json.Unmarshal(body, &hidden)

	rep := proto.Report{V: proto.Version, CPU: 42, MemTotal: 100, MemUsed: 50, DiskTotal: 10, DiskUsed: 1, BootTime: 1,
		Info: &proto.Info{OS: "Debian 13", Arch: "amd64", CPUCores: 2}}
	hdr := map[string]string{"Authorization": "Bearer " + created.Token, "X-Forwarded-For": "203.0.113.9"}
	if resp, _ := e.do(t, "POST", e.pub.URL+"/api/v1/report", rep, map[string]string{"Authorization": "Bearer nope"}); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}
	if resp, b := e.do(t, "POST", e.pub.URL+"/api/v1/report", map[string]any{"v": 1, "evil": true}, hdr); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field: %d %s", resp.StatusCode, b)
	}
	// Nobody has viewed the dashboard yet, so the agent may slow down.
	if resp, b := e.do(t, "POST", e.pub.URL+"/api/v1/report", rep, hdr); resp.StatusCode != proto.StatusIdle || len(b) != 0 {
		t.Fatalf("report: %d %s", resp.StatusCode, b)
	}
	e.do(t, "POST", e.pub.URL+"/api/v1/report", rep, map[string]string{"Authorization": "Bearer " + hidden.Token})

	// Public view: no hidden nodes, no IPs.
	resp, body = e.do(t, "GET", e.pub.URL+"/api/v1/nodes", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public nodes: %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "203.0.113.9") || strings.Contains(string(body), "hidden") {
		t.Fatalf("public response leaks data: %s", body)
	}
	var nodes []publicNode
	json.Unmarshal(body, &nodes)
	if len(nodes) != 1 || !nodes[0].Online || nodes[0].Status.CPU != 42 || nodes[0].Info.OS != "" || nodes[0].Visibility.System {
		t.Fatalf("public nodes: %s", body)
	}

	// Admin view includes hidden nodes and the IP from the trusted proxy header.
	_, body = e.do(t, "GET", e.adm.URL+"/admin/api/nodes", nil, nil)
	var all []adminNode
	json.Unmarshal(body, &all)
	if len(all) != 2 || all[0].LastIP != "203.0.113.9" {
		t.Fatalf("admin nodes: %s", body)
	}

	if resp, b := e.do(t, "GET", e.pub.URL+"/api/v1/nodes/"+created.Node.UUID+"/metrics?range=live", nil, nil); resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"cpu":42`) {
		t.Fatalf("live metrics: %d %s", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "GET", e.pub.URL+"/api/v1/nodes/"+created.Node.UUID+"/metrics?range=24h", nil, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("history: %d", resp.StatusCode)
	}

	// Disabling the public page hides data from anonymous visitors only.
	settings := e.st.Settings()
	settings.PublicEnabled = false
	if resp, b := e.do(t, "PUT", e.adm.URL+"/admin/api/settings", settings, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("settings: %d %s", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "GET", e.pub.URL+"/api/v1/nodes", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("public disabled: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", e.adm.URL+"/api/v1/nodes", nil, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin view with public disabled: %d", resp.StatusCode)
	}

	// The dashboard was just viewed, so the agent is told to keep the normal interval.
	time.Sleep(600 * time.Millisecond)
	if resp, _ := e.do(t, "POST", e.pub.URL+"/api/v1/report", rep, hdr); resp.StatusCode != proto.StatusWatched {
		t.Fatalf("report while watched: %d", resp.StatusCode)
	}

	// Rotating the token invalidates the old one.
	resp, _ = e.do(t, "POST", e.adm.URL+"/admin/api/nodes/"+itoa(created.Node.ID)+"/token", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate: %d", resp.StatusCode)
	}
	time.Sleep(900 * time.Millisecond)
	if resp, _ := e.do(t, "POST", e.pub.URL+"/api/v1/report", rep, hdr); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token after rotation: %d", resp.StatusCode)
	}

	if resp, b := e.do(t, "POST", e.adm.URL+"/admin/api/rules", store.Rule{Name: "off", Kind: "offline", Duration: 60, Enabled: true}, nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rule: %d %s", resp.StatusCode, b)
	}
	if resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/rules", store.Rule{Name: "bad", Kind: "cpu", Threshold: 500}, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid rule: %d", resp.StatusCode)
	}

	if resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/logout", nil, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", e.adm.URL+"/admin/api/me", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := setup(t)
	var last int
	for range 12 {
		resp, _ := e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "nope"}, nil)
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := setup(t)
	resp, _ := e.do(t, "GET", e.pub.URL+"/", nil, nil)
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
