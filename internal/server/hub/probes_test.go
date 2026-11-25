package hub

import (
	"github.com/mby/fenghuo/internal/proto"
	"testing"
	"time"
)

func TestProbeDedupProtocolsAndLocalErrors(t *testing.T) {
	now := time.Unix(1800000010, 0)
	n := &node{}
	r := &proto.Report{Time: now.UnixMilli(), ProbeSession: "01234567890123456789012345678901", Probes: []proto.Probe{
		{Name: "CT", Kind: "icmp", MS: 100, Time: now.Add(-4 * time.Second).UnixMilli(), Seq: 1},
		{Name: "CT", Kind: "icmp", MS: 200, Time: now.Add(-2 * time.Second).UnixMilli(), Seq: 2},
		{Name: "CT", Kind: "icmp", MS: -1, Time: now.UnixMilli(), Seq: 3},
		{Name: "CT", Kind: "tcp", MS: 1200, Time: now.UnixMilli(), Seq: 4},
		{Name: "local", Kind: "icmp", MS: -1, Error: "permission", Time: now.UnixMilli(), Seq: 5},
	}}
	n.addProbes(r, now)
	n.addProbes(r, now.Add(time.Second))
	stats := n.networkStats(now.Add(time.Second))
	if len(stats) != 3 {
		t.Fatalf("protocols merged: %+v", stats)
	}
	for _, s := range stats {
		if s.Target == "local" {
			if s.Count != 0 || s.Errors != 1 || s.Error != "permission" {
				t.Fatalf("local error counted as packet loss: %+v", s)
			}
			continue
		}
		if s.Kind == "icmp" && (s.Count != 3 || s.SuccessCount != 2 || s.MedianMS != 150 || s.AvgMS != 150 || len(s.History) != 1) {
			t.Fatalf("icmp: %+v", s)
		}
		if s.Kind == "tcp" && (s.Count != 1 || s.AvgMS != 1200) {
			t.Fatalf("tcp: %+v", s)
		}
	}
	for key, a := range n.probeMinutes {
		if key.kind == "icmp" && (a.total != 3 || a.ok != 2 || a.sum != 300) {
			t.Fatalf("duplicate persisted: %+v", a)
		}
	}
}

func TestCardHistoryKeepsTwelveCompleteMinutesWithoutChangingAlertWindow(t *testing.T) {
	end := time.Unix(1800000000, 0)
	n := &node{}
	for i := 0; i < 25; i++ {
		at := end.Add(time.Duration(i-24) * 30 * time.Second)
		n.addProbes(&proto.Report{Time: at.UnixMilli(), ProbeSession: "card", Probes: []proto.Probe{{Name: "CT", Kind: "icmp", Time: at.UnixMilli(), Seq: uint64(i + 1), MS: 100}}}, at)
	}
	now := end.Add(20 * time.Second)
	cards := n.cardNetworkStats(now)
	if len(cards) != 1 || len(cards[0].History) != 12 || cards[0].Count != 24 || cards[0].WindowEnd != end.Unix() {
		t.Fatalf("card window: %+v", cards)
	}
	if cards[0].History[0].T != end.Add(-12*time.Minute).Unix() || cards[0].History[11].T != end.Add(-time.Minute).Unix() {
		t.Fatal("minute boundaries shifted")
	}
	alerts := n.networkStats(now)
	if len(alerts) != 1 || alerts[0].Count != 10 {
		t.Fatalf("alert window changed: %+v", alerts)
	}
}
