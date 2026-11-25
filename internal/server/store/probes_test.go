package store

import (
	"context"
	"testing"
)

func TestProtocolHistoryWeightsAndLateSamples(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	n := Node{Name: "probe", TokenHash: "probe", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	for _, row := range []PingRow{
		{NodeID: n.ID, Target: "CT", Kind: "icmp", TS: 3600, AvgMS: 100, Samples: 1, Successes: 1, SumMS: 100},
		{NodeID: n.ID, Target: "CT", Kind: "icmp", TS: 3660, AvgMS: 300, Loss: 66.6666666667, Samples: 9, Successes: 3, SumMS: 900},
		{NodeID: n.ID, Target: "CT", Kind: "tcp", TS: 3600, AvgMS: 1200, Samples: 1, Successes: 1, SumMS: 1200},
		{NodeID: n.ID, Target: "CT", Kind: "icmp", TS: 3600, AvgMS: 500, Samples: 1, Successes: 1, SumMS: 500},
	} {
		if err := s.SaveBatch(ctx, Batch{Pings: []PingRow{row}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, hourly := range []bool{false, true} {
		if err := s.RollupHours(ctx, 3600, 7200); err != nil {
			t.Fatal(err)
		}
		series, err := s.QueryPing(ctx, n.ID, 0, 3600)
		if hourly {
			series, err = s.QueryPingHours(ctx, n.ID, 0, 3600)
		}
		if err != nil || len(series) != 2 {
			t.Fatalf("series: %+v %v", series, err)
		}
		for _, v := range series {
			p := v.Points[0]
			if v.Kind == "icmp" && (p.MS != 300 || p.Samples == nil || *p.Samples != 11 || *p.Successes != 5 || p.Loss < 54.54 || p.Loss > 54.55) {
				t.Fatalf("weighted counts: %+v", p)
			}
			if v.Kind == "tcp" && p.MS != 1200 {
				t.Fatal("protocol history mixed")
			}
		}
	}
}
