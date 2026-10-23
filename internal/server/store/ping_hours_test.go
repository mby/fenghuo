package store

import (
	"context"
	"testing"
)

func TestPingHourlyRetention(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	n := Node{Name: "a", TokenHash: "h", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBatch(ctx, Batch{Pings: []PingRow{
		{NodeID: n.ID, Target: "上海电信", TS: 3600, AvgMS: 12, Loss: 0},
		{NodeID: n.ID, Target: "上海电信", TS: 3660, AvgMS: -1, Loss: 100},
		{NodeID: n.ID, Target: "北京移动", TS: 7200, AvgMS: -1, Loss: 100},
	}}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.RollupHours(ctx, 0, 10800); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Cleanup(ctx, 10800, 3600, 0, 0); err != nil {
		t.Fatal(err)
	}
	minute, err := s.QueryPing(ctx, n.ID, 0, 60)
	if err != nil || len(minute) != 0 {
		t.Fatalf("minute cleanup: %v %v", minute, err)
	}
	hours, err := s.QueryPingHours(ctx, n.ID, 0, 3600)
	if err != nil || len(hours) != 2 {
		t.Fatalf("hours: %v %v", hours, err)
	}
	for _, series := range hours {
		if len(series.Points) != 1 {
			t.Fatalf("duplicate rollup: %v", series)
		}
		p := series.Points[0]
		if series.Name == "上海电信" && (p.MS != 12 || p.Loss != 50) {
			t.Fatalf("mixed attempts: %v", p)
		}
		if series.Name == "北京移动" && (p.MS != -1 || p.Loss != 100) {
			t.Fatalf("all failed: %v", p)
		}
	}
	if err := s.Cleanup(ctx, 10800, 10800, 0, 0); err != nil {
		t.Fatal(err)
	}
	hours, err = s.QueryPingHours(ctx, n.ID, 0, 3600)
	if err != nil || len(hours) != 0 {
		t.Fatalf("hour cleanup: %v %v", hours, err)
	}
}
