package hub

import (
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestApplyCounters(t *testing.T) {
	var tr store.Traffic
	applyCounters(&tr, 1000, 500, 100) // first report: baseline only
	if tr.Up != 0 || tr.Down != 0 {
		t.Fatalf("baseline counted traffic: %+v", tr)
	}
	applyCounters(&tr, 1001, 800, 150) // same boot (jitter)
	if tr.Down != 300 || tr.Up != 50 {
		t.Fatalf("delta: %+v", tr)
	}
	applyCounters(&tr, 1001, 10, 10) // counters went backwards, same boot: rebase
	if tr.Down != 300 || tr.Up != 50 {
		t.Fatalf("rebase: %+v", tr)
	}
	applyCounters(&tr, 5000, 40, 20) // reboot: count since boot
	if tr.Down != 340 || tr.Up != 70 {
		t.Fatalf("reboot: %+v", tr)
	}
}

func TestPeriodStart(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		now  time.Time
		day  int
		want time.Time
	}{
		{time.Date(2026, 3, 15, 10, 0, 0, 0, loc), 1, time.Date(2026, 3, 1, 0, 0, 0, 0, loc)},
		{time.Date(2026, 3, 15, 10, 0, 0, 0, loc), 20, time.Date(2026, 2, 20, 0, 0, 0, 0, loc)},
		{time.Date(2026, 1, 5, 0, 0, 0, 0, loc), 10, time.Date(2025, 12, 10, 0, 0, 0, 0, loc)},
		{time.Date(2026, 3, 20, 0, 0, 0, 0, loc), 20, time.Date(2026, 3, 20, 0, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		if got := PeriodStart(c.now, c.day); got != c.want.Unix() {
			t.Errorf("PeriodStart(%v, %d) = %v, want %v", c.now, c.day, time.Unix(got, 0).UTC(), c.want)
		}
	}
}

func TestIngestAndDrain(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	h := New(start)
	h.Reload([]store.Node{{ID: 1, UUID: "u1", TokenHash: "tok", TrafficResetDay: 1}}, nil, start)

	rep := func(cpu float64, rx uint64) *proto.Report {
		return &proto.Report{V: proto.Version, BootTime: 100, CPU: cpu, NetRx: rx, NetTx: rx,
			Ping: []proto.Ping{{Name: "CT", MS: 10}, {Name: "CU", MS: -1}}}
	}

	if _, err := h.Ingest("nope", rep(1, 0), "1.1.1.1", start); err != ErrUnknownToken {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err := h.Ingest("tok", rep(10, 1000), "1.1.1.1", start); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Ingest("tok", rep(10, 1000), "1.1.1.1", start.Add(100*time.Millisecond)); err != ErrTooFrequent {
		t.Fatalf("too frequent: %v", err)
	}
	if _, err := h.Ingest("tok", rep(30, 3000), "1.1.1.1", start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	v, ok := h.ByUUID("u1", start.Add(4*time.Second))
	if !ok || !v.Online || v.Traffic.Down != 2000 || v.Info.LastIP != "1.1.1.1" {
		t.Fatalf("view: %+v", v)
	}
	if v, _ := h.ByUUID("u1", start.Add(time.Minute)); v.Online {
		t.Fatal("node should be offline after silence")
	}
	if got := len(h.Live(1)); got != 2 {
		t.Fatalf("live len = %d", got)
	}

	// The current minute is still open: nothing to flush yet except info/traffic.
	b := h.Drain(start.Add(10*time.Second), false)
	if len(b.Metrics) != 0 || len(b.Infos) != 1 || len(b.Traffic) != 1 {
		t.Fatalf("early drain: %+v", b)
	}
	b = h.Drain(start.Add(61*time.Second), false)
	if len(b.Metrics) != 1 || b.Metrics[0].CPU != 20 || b.Metrics[0].T != start.Unix() {
		t.Fatalf("minute drain: %+v", b.Metrics)
	}
	if len(b.Pings) != 2 {
		t.Fatalf("ping drain: %+v", b.Pings)
	}
	for _, p := range b.Pings {
		switch p.Target {
		case "CT":
			if p.AvgMS != 10 || p.Loss != 0 {
				t.Fatalf("CT: %+v", p)
			}
		case "CU":
			if p.AvgMS != -1 || p.Loss != 100 {
				t.Fatalf("CU: %+v", p)
			}
		}
	}
}

func TestAdaptiveInterval(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	h := New(start)
	h.Reload([]store.Node{{ID: 1, UUID: "u1", TokenHash: "tok", TrafficResetDay: 1}}, nil, start)
	rep := &proto.Report{V: proto.Version, Interval: 3000, IdleInterval: 30000}

	// Nobody watching: idle, and silence up to 3x the idle interval is fine.
	watched, err := h.Ingest("tok", rep, "", start)
	if err != nil || watched {
		t.Fatalf("idle ingest: watched=%v err=%v", watched, err)
	}
	if v, _ := h.ByUUID("u1", start.Add(80*time.Second)); !v.Online {
		t.Fatal("idle node marked offline before 3 idle intervals")
	}
	if v, _ := h.ByUUID("u1", start.Add(100*time.Second)); v.Online {
		t.Fatal("idle node still online after 3 idle intervals")
	}

	// A viewer arrives: the next report is told to speed up, and the
	// offline threshold follows the normal interval again.
	h.MarkViewed(start.Add(30 * time.Second))
	watched, err = h.Ingest("tok", rep, "", start.Add(31*time.Second))
	if err != nil || !watched {
		t.Fatalf("watched ingest: watched=%v err=%v", watched, err)
	}
	if v, _ := h.ByUUID("u1", start.Add(31*time.Second+20*time.Second)); v.Online {
		t.Fatal("watched node should be offline after 3 normal intervals")
	}
	if h.Watched(start.Add(30*time.Second + viewerWindow)) {
		t.Fatal("viewer window did not expire")
	}
}

func TestRestartGrace(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	h := New(start)
	known := store.Node{ID: 1, UUID: "known", TokenHash: "a", TrafficResetDay: 1, Info: store.NodeInfo{UpdatedAt: 1}}
	fresh := store.Node{ID: 2, UUID: "fresh", TokenHash: "b", TrafficResetDay: 1}
	h.Reload([]store.Node{known, fresh}, nil, start)

	if v, _ := h.ByUUID("known", start.Add(30*time.Second)); !v.Online {
		t.Fatal("a known node must not be offline right after a server restart")
	}
	if v, _ := h.ByUUID("fresh", start.Add(30*time.Second)); v.Online {
		t.Fatal("a node that never reported must not be online")
	}
	if v, _ := h.ByUUID("known", start.Add(restartGrace+time.Second)); v.Online {
		t.Fatal("a silent node must be offline after the grace period")
	}
}

func TestTrafficPeriodReset(t *testing.T) {
	start := time.Date(2026, 5, 31, 23, 59, 0, 0, time.UTC)
	h := New(start)
	h.Reload([]store.Node{{ID: 1, UUID: "u1", TokenHash: "tok", TrafficResetDay: 1}},
		map[int64]store.Traffic{1: {PeriodStart: PeriodStart(start, 1), Up: 5, Down: 5, BootTime: 100}}, start)
	b := h.Drain(start.Add(2*time.Minute), false)
	if len(b.Traffic) != 1 || b.Traffic[0].Up != 0 || b.Traffic[0].PeriodStart != time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("period reset: %+v", b.Traffic)
	}
}
