package hub

import (
	"fmt"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestNetworkWindow(t *testing.T) {
	start := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name  string
		age   time.Duration
		fresh bool
		count int
	}{
		{"fresh", time.Second, true, 3},
		{"fresh boundary", NetworkFreshFor, true, 3},
		{"stale", NetworkFreshFor + time.Second, false, 3},
		{"expired", NetworkWindow + time.Second, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &node{}
			for i, ms := range []float64{10, -1, 30} {
				n.addNetwork([]proto.Ping{{Name: "edge", MS: ms}}, start.Add(time.Duration(i)*time.Second))
			}
			n.addNetwork(nil, start.Add(3*time.Second))
			stats := n.networkStats(start.Add(2*time.Second + tc.age))
			if len(stats) != 1 || stats[0].Fresh != tc.fresh || stats[0].Count != tc.count {
				t.Fatalf("%+v", stats)
			}
			if tc.count > 0 && (stats[0].AvgMS != 20 || stats[0].SuccessCount != 2 || stats[0].FailureRate != 100.0/3) {
				t.Fatalf("averages: %+v", stats[0])
			}
		})
	}
}

func TestNetworkHistoryBuckets(t *testing.T) {
	start := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name          string
		values        []float64
		wantMS        float64
		wantSuccesses int
	}{
		{"mixed", []float64{10, -1, 30}, 20, 2},
		{"all failed", []float64{-1, -1, -1}, -1, 0},
		{"zero latency", []float64{0, 0, 0}, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &node{}
			for i, ms := range tc.values {
				n.addNetwork([]proto.Ping{{Name: "CT", MS: ms}}, start.Add(time.Duration(i)*time.Second))
			}
			n.addNetwork([]proto.Ping{{Name: "CT", MS: 50}}, start.Add(time.Minute))
			s := n.networkStats(start.Add(time.Minute))[0]
			if len(s.History) != 2 || s.History[0].T != start.Unix() || s.History[0].Samples != 3 || s.History[0].Successes != tc.wantSuccesses || s.History[0].AvgMS != tc.wantMS {
				t.Fatalf("history: %+v", s.History)
			}
			s.History[0].AvgMS = 999
			if n.networkStats(start.Add(time.Minute))[0].History[0].AvgMS != tc.wantMS {
				t.Fatal("history aliases internal samples")
			}
			if got := n.networkStats(start.Add(NetworkWindow + time.Minute + time.Second))[0]; len(got.History) != 0 {
				t.Fatalf("expired buckets retained: %+v", got.History)
			}
		})
	}
	n := &node{}
	for i := 0; i < 400; i++ {
		n.addNetwork([]proto.Ping{{Name: "CT", MS: 1}}, start.Add(time.Duration(i)*time.Second))
	}
	s := n.networkStats(start.Add(399 * time.Second))[0]
	if len(s.History) > 11 {
		t.Fatalf("history must remain bounded: %d", len(s.History))
	}
}

func TestNetworkBoundsAndContinuity(t *testing.T) {
	start := time.Unix(1700000000, 0)
	n := &node{}
	for i := 0; i < maxProbeSamples+20; i++ {
		n.addNetwork([]proto.Ping{{Name: "edge", MS: 10}}, start.Add(time.Duration(i)*time.Second))
	}
	if len(n.network["edge"].samples) != maxProbeSamples {
		t.Fatal("unbounded sample window")
	}
	now := start.Add(10 * time.Minute)
	n.addNetwork([]proto.Ping{{Name: "edge", MS: -1}, {Name: "edge", MS: 0}}, now)
	s := n.networkStats(now)[0]
	if s.Count != 1 || s.FailureRate != 100 || s.AvgMS != -1 || !s.ContinuousSince.Equal(now) {
		t.Fatalf("gap/duplicate: %+v", s)
	}
	pings := make([]proto.Ping, proto.MaxPingTargets+10)
	for i := range pings {
		pings[i] = proto.Ping{Name: fmt.Sprintf("target-%d", i), MS: 1}
	}
	n.addNetwork(pings, now.Add(time.Second))
	if len(n.network) != proto.MaxPingTargets {
		t.Fatal("unbounded target set")
	}
	if _, ok := n.network["edge"]; ok {
		t.Fatal("removed local target retained")
	}
}

func TestNetworkSuccessFreshness(t *testing.T) {
	start := time.Unix(1700000000, 0)
	n := &node{}
	n.addNetwork([]proto.Ping{{Name: "edge", MS: 10}}, start)
	n.addNetwork([]proto.Ping{{Name: "edge", MS: -1}}, start.Add(time.Minute))
	n.addNetwork([]proto.Ping{{Name: "edge", MS: -1}}, start.Add(2*time.Minute+time.Second))
	s := n.networkStats(start.Add(2*time.Minute + time.Second))[0]
	if !s.Fresh || s.LatencyFresh || s.Count != 3 || s.SuccessCount != 1 {
		t.Fatalf("recent failures made old successful latency fresh: %+v", s)
	}
}

func TestNetworkHubLifecycle(t *testing.T) {
	start := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name string
		gap  time.Duration
		boot int64
		want int
	}{
		{"empty report retains samples", time.Second, 1, 1},
		{"offline gap resets samples", time.Minute, 1, 0},
		{"reboot resets samples", time.Second, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(start)
			nodes := []store.Node{{ID: 1, TokenHash: "token", TrafficResetDay: 1}}
			h.Reload(nodes, nil, start)
			if _, err := h.Ingest("token", &proto.Report{BootTime: 1, Interval: 1000, Ping: []proto.Ping{{Name: "edge", MS: 10}}}, "", start); err != nil {
				t.Fatal(err)
			}
			h.Drain(start.Add(time.Second), true)
			h.Reload(nodes, nil, start)
			if _, err := h.Ingest("token", &proto.Report{BootTime: tc.boot, Interval: 1000}, "", start.Add(tc.gap)); err != nil {
				t.Fatal(err)
			}
			view := h.Snapshot(start.Add(tc.gap))[0]
			if len(view.Network) != tc.want {
				t.Fatalf("%+v", view.Network)
			}
			if len(view.Network) > 0 {
				view.Network[0].AvgMS = 999
				if h.Snapshot(start.Add(tc.gap))[0].Network[0].AvgMS != 10 {
					t.Fatal("snapshot aliases internal samples")
				}
			}
		})
	}
}
