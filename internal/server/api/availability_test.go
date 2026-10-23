package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestAvailabilityPermissionsAndRange(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "availability.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n := store.Node{Name: "private", TokenHash: "secret-hash", Note: "private-note", TrafficMode: "sum", TrafficResetDay: 1}
	if err := st.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(ctx, auth.HashToken("session"), time.Now().Unix(), time.Now().Add(time.Hour).Unix(), "", ""); err != nil {
		t.Fatal(err)
	}
	h := hub.New(time.Now())
	s := New(Deps{Store: st, Hub: h, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, tc := range []struct {
		name, query                     string
		hidden, public, mounted, cookie bool
		status                          int
	}{
		{"public default", "", false, true, false, false, 200},
		{"seven days", "?range=7d", false, true, false, false, 200},
		{"thirty days", "?range=30d", false, true, false, false, 200},
		{"ninety days", "?range=90d", false, true, false, false, 200},
		{"hidden", "", true, true, false, false, 404},
		{"public disabled", "", false, false, false, false, 404},
		{"admin sees hidden", "", true, false, true, true, 200},
		{"public listener ignores session", "", true, true, false, true, 404},
		{"invalid range", "?range=1h", false, true, false, false, 400},
		{"empty range", "?range=", false, true, false, false, 400},
		{"duplicate range", "?range=7d&range=30d", false, true, false, false, 400},
		{"unknown argument", "?days=7", false, true, false, false, 400},
		{"malformed escape", "?range=%zz", false, true, false, false, 400},
		{"hidden checked first", "?range=bad", true, true, false, false, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n.Hidden = tc.hidden
			h.Reload([]store.Node{n}, nil, time.Now())
			settings := st.Settings()
			settings.PublicEnabled = tc.public
			if err := st.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/nodes/{uuid}/availability", s.viewer(tc.mounted, s.handleAvailability))
			req := httptest.NewRequest("GET", "/api/v1/nodes/"+n.UUID+"/availability"+tc.query, nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == 200 {
				var summary store.AvailabilitySummary
				if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
					t.Fatal(err)
				}
				if summary.UptimePercent != nil || summary.UnknownSeconds != summary.To-summary.From {
					t.Fatal("unobserved node was not unknown")
				}
				for _, secret := range []string{n.UUID, n.Name, n.TokenHash, n.Note, "last_ip"} {
					if strings.Contains(w.Body.String(), secret) {
						t.Fatalf("private field leaked: %s", secret)
					}
				}
			}
		})
	}
}
