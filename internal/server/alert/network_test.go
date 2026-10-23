package alert

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestNetworkValue(t *testing.T) {
	good := hub.NetworkStat{Target: "good", AvgMS: 10, FailureRate: 0, Count: 2, SuccessCount: 2, Fresh: true, LatencyFresh: true}
	bad := hub.NetworkStat{Target: "bad", AvgMS: 200, FailureRate: 50, Count: 2, SuccessCount: 1, Fresh: true, LatencyFresh: true}
	failed := hub.NetworkStat{Target: "failed", AvgMS: -1, FailureRate: 100, Count: 2, Fresh: true}
	stale := hub.NetworkStat{Target: "stale", AvgMS: 300, Count: 2, SuccessCount: 2, Fresh: false}
	staleSuccess := hub.NetworkStat{Target: "old-success", AvgMS: 10, Count: 3, SuccessCount: 1, Fresh: true, LatencyFresh: false}
	for _, tc := range []struct {
		name, kind, target string
		stats              []hub.NetworkStat
		want               float64
		known              bool
	}{
		{"named healthy", KindLatency, "good", []hub.NetworkStat{good, bad}, 10, true},
		{"worst latency", KindLatency, "", []hub.NetworkStat{good, bad}, 200, true},
		{"worst failure", KindPacketLoss, "", []hub.NetworkStat{good, bad, failed}, 100, true},
		{"missing", KindLatency, "missing", []hub.NetworkStat{good}, -1, false},
		{"stale named", KindLatency, "stale", []hub.NetworkStat{stale}, -1, false},
		{"all failed latency unknown", KindLatency, "failed", []hub.NetworkStat{failed}, -1, false},
		{"stale prevents recovery", KindLatency, "", []hub.NetworkStat{good, stale}, 10, false},
		{"failed prevents latency recovery", KindLatency, "", []hub.NetworkStat{good, failed}, 10, false},
		{"known breach despite stale", KindLatency, "", []hub.NetworkStat{bad, stale}, 200, true},
		{"threshold breach despite stale", KindPacketLoss, "", []hub.NetworkStat{bad, stale}, 50, true},
		{"stale success is not healthy latency", KindLatency, "", []hub.NetworkStat{staleSuccess}, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, value, known := networkValue(store.Rule{Kind: tc.kind, Target: tc.target, Threshold: 50}, tc.stats)
			if value != tc.want || known != tc.known {
				t.Fatalf("value=%v known=%v", value, known)
			}
		})
	}
}

func TestNetworkInclusiveThreshold(t *testing.T) {
	for _, kind := range []string{KindLatency, KindPacketLoss} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			n := store.Node{Name: "node", TokenHash: "tok", TrafficResetDay: 1, TrafficMode: "sum"}
			if err := st.CreateNode(ctx, &n); err != nil {
				t.Fatal(err)
			}
			r := store.Rule{Name: kind, Kind: kind, Threshold: 100, Enabled: true}
			if err := st.CreateRule(ctx, &r); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			rec := &recorder{}
			e := New(hub.New(now), st, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
			v := hub.View{Node: n, Online: true, Status: &proto.Report{}, Network: []hub.NetworkStat{
				{Target: "edge", AvgMS: 100, Count: 1, SuccessCount: 1, Fresh: true, LatencyFresh: true, ContinuousSince: now},
			}}
			if kind == KindPacketLoss {
				v.Network[0].AvgMS = -1
				v.Network[0].SuccessCount = 0
				v.Network[0].FailureRate = 100
				v.Network[0].LatencyFresh = false
			}
			e.eval(ctx, r, v, now)
			msgs := rec.take()
			if len(msgs) != 1 || !msgs[0].Firing {
				t.Fatalf("equality must fire: %+v", msgs)
			}
			e.eval(ctx, r, v, now.Add(time.Second))
			if msgs := rec.take(); len(msgs) != 0 {
				t.Fatalf("equality resolved: %+v", msgs)
			}
			v.Network[0].AvgMS = 99
			v.Network[0].FailureRate = 99
			v.Network[0].Count = 100
			v.Network[0].SuccessCount = 1
			e.eval(ctx, r, v, now.Add(2*time.Second))
			msgs = rec.take()
			if len(msgs) != 1 || msgs[0].Firing {
				t.Fatalf("below must recover: %+v", msgs)
			}
		})
	}
}

func TestNetworkAlertLifecycle(t *testing.T) {
	for _, kind := range []string{KindLatency, KindPacketLoss} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			start := time.Now()
			n := store.Node{Name: "node", TokenHash: "tok", TrafficMode: "sum", TrafficResetDay: 1}
			if err := st.CreateNode(ctx, &n); err != nil {
				t.Fatal(err)
			}
			r := store.Rule{Name: kind, Kind: kind, Target: "edge", Threshold: 50, Duration: 20, Enabled: true}
			if err := st.CreateRule(ctx, &r); err != nil {
				t.Fatal(err)
			}
			h := hub.New(start)
			h.Reload([]store.Node{n}, nil, start)
			rec := &recorder{}
			e := New(h, st, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := e.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			bad := 200.0
			if kind == KindPacketLoss {
				bad = -1
			}
			report := func(offset time.Duration, pings []proto.Ping) {
				t.Helper()
				if _, err := h.Ingest("tok", &proto.Report{Interval: 10000, Ping: pings}, "", start.Add(offset)); err != nil {
					t.Fatal(err)
				}
			}
			probe := func(ms float64) []proto.Ping { return []proto.Ping{{Name: "edge", MS: ms}} }
			report(0, probe(bad))
			e.Evaluate(ctx, start)
			report(10*time.Second, nil)
			e.Evaluate(ctx, start.Add(10*time.Second))
			if got := rec.take(); len(got) != 0 {
				t.Fatalf("early firing: %+v", got)
			}
			report(20*time.Second, nil)
			e.Evaluate(ctx, start.Add(20*time.Second))
			got := rec.take()
			if len(got) != 1 || !got[0].Firing {
				t.Fatalf("missing firing: %+v", got)
			}
			if kind == KindPacketLoss && !strings.Contains(got[0].Text, "TCP connection failure rate") {
				t.Fatalf("wrong label: %s", got[0].Text)
			}
			// Continue receiving reports while probes disappear.
			for sec := 30; sec <= 150; sec += 10 {
				report(time.Duration(sec)*time.Second, nil)
				e.Evaluate(ctx, start.Add(time.Duration(sec)*time.Second))
			}
			if got := rec.take(); len(got) != 0 {
				t.Fatalf("stale resolved/fired: %+v", got)
			}
			if _, ok := e.pending[key{r.ID, n.ID}]; ok {
				t.Fatal("stale duration retained")
			}
			// Restarted engine retains firing state, but must wait for fresh data.
			if err := e.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			e.Evaluate(ctx, start.Add(151*time.Second))
			if got := rec.take(); len(got) != 0 {
				t.Fatalf("restart resolved: %+v", got)
			}
			report(160*time.Second, probe(10))
			e.Evaluate(ctx, start.Add(160*time.Second))
			got = rec.take()
			if len(got) != 1 || got[0].Firing {
				t.Fatalf("fresh recovery: %+v", got)
			}
		})
	}
}

func TestNetworkPendingCleared(t *testing.T) {
	for _, reason := range []string{"missing", "stale", "offline", "continuity gap"} {
		t.Run(reason, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			now := time.Now()
			e := New(hub.New(now), st, &recorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := store.Rule{ID: 1, Kind: KindLatency, Threshold: 50, Duration: 20, Target: "edge"}
			v := hub.View{Node: store.Node{ID: 1}, Online: true, Status: &proto.Report{}, Network: []hub.NetworkStat{
				{Target: "edge", AvgMS: 100, Count: 1, SuccessCount: 1, Fresh: true, LatencyFresh: true, ContinuousSince: now},
			}}
			e.eval(context.Background(), r, v, now)
			switch reason {
			case "missing":
				v.Network = nil
			case "stale":
				v.Network[0].Fresh = false
			case "offline":
				v.Online = false
			case "continuity gap":
				v.Network[0].ContinuousSince = now.Add(25 * time.Second)
			}
			e.eval(context.Background(), r, v, now.Add(30*time.Second))
			pending, ok := e.pending[key{1, 1}]
			if reason == "continuity gap" {
				if !ok || !pending.Equal(now.Add(30*time.Second)) {
					t.Fatalf("gap did not restart duration: %v", pending)
				}
			} else if ok {
				t.Fatalf("%s retained duration", reason)
			}
		})
	}
}
