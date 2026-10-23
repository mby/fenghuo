package api

import (
	"context"
	"encoding/json"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLongProbeHistoryUsesHours(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	n := store.Node{Name: "test", TokenHash: "hash", TrafficMode: "sum", TrafficResetDay: 1}
	if err := e.st.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ts := now.Add(-3 * 24 * time.Hour).Truncate(time.Hour).Unix()
	if err := e.st.SaveBatch(ctx, store.Batch{Pings: []store.PingRow{{NodeID: n.ID, Target: "上海电信", TS: ts, AvgMS: 25, Loss: 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := e.st.RollupHours(ctx, ts, ts+3600); err != nil {
		t.Fatal(err)
	}
	if err := e.st.Cleanup(ctx, now.Add(-24*time.Hour).Unix(), 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	h := hub.New(now)
	h.Reload([]store.Node{n}, nil, now)
	s := &Server{Deps: Deps{Store: e.st, Hub: h, KeepMinute: 24 * time.Hour, KeepHour: 90 * 24 * time.Hour}}
	for _, tc := range []struct {
		period string
		count  int
	}{{"24h", 0}, {"30d", 1}, {"90d", 1}} {
		req := httptest.NewRequest("GET", "/api/v1/nodes/"+n.UUID+"/ping?range="+tc.period, nil)
		req.SetPathValue("uuid", n.UUID)
		w := httptest.NewRecorder()
		s.handlePing(w, req, false)
		var series []store.PingSeries
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &series) != nil || len(series) != tc.count {
			t.Fatalf("%s: %d %s", tc.period, w.Code, w.Body.String())
		}
	}
}
