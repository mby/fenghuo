package api

import (
	"context"
	"encoding/json"
	"github.com/mby/fenghuo/internal/proto"
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

func TestCardHistoryUsesTwelveMinuteBuckets(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	n := store.Node{Name: "test", TokenHash: "hash", TrafficMode: "sum", TrafficResetDay: 1}
	if err := e.st.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	end := now.Truncate(time.Minute)
	var rows []store.PingRow
	for i := 13; i > 0; i-- {
		rows = append(rows, store.PingRow{NodeID: n.ID, Kind: "icmp", Target: "CT", TS: end.Add(-time.Duration(i) * time.Minute).Unix(), Samples: 2, Successes: 1, SumMS: 100, AvgMS: 100, Loss: 50})
	}
	if err := e.st.SaveBatch(ctx, store.Batch{Pings: rows}); err != nil {
		t.Fatal(err)
	}
	h := hub.New(now)
	h.Reload([]store.Node{n}, nil, now)
	if _, err := h.Ingest("hash", &proto.Report{V: proto.Version, Interval: 60000, Ping: []proto.Ping{{Name: "CT", MS: 18}}}, "private-ip", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	s := &Server{Deps: Deps{Store: e.st, Hub: h, KeepMinute: 24 * time.Hour}}
	req := httptest.NewRequest("GET", "/api/v1/nodes/"+n.UUID+"/ping?range=12m", nil)
	req.SetPathValue("uuid", n.UUID)
	w := httptest.NewRecorder()
	s.handlePing(w, req, true)
	var series []store.PingSeries
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &series) != nil || len(series) != 1 || len(series[0].Points) != 12 {
		t.Fatalf("twelve-minute history: %d %s", w.Code, w.Body.String())
	}
	if series[0].Kind != "icmp" || series[0].Points[0].T != end.Add(-12*time.Minute).Unix() {
		t.Fatalf("window start or protocol: %+v", series[0])
	}
	for _, point := range series[0].Points {
		if point.Samples == nil || *point.Samples != 2 || point.Successes == nil || *point.Successes != 1 || point.MS != 100 {
			t.Fatalf("minute counts: %+v", point)
		}
	}
	w = httptest.NewRecorder()
	s.handleNodes(w, req, true)
	var nodes []publicNode
	if json.Unmarshal(w.Body.Bytes(), &nodes) != nil || len(nodes) != 1 || len(nodes[0].Network) != 1 {
		t.Fatalf("card: %s", w.Body.String())
	}
	probe := nodes[0].Network[0]
	if probe.WindowEnd != end.Unix() || probe.Samples != 1 || len(probe.History) != 1 || probe.History[0].T%60 != 0 {
		t.Fatalf("server-anchored minute window: %+v", probe)
	}
}
