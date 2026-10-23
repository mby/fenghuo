package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestMetricsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	n := Node{Name: "a", TokenHash: "h", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}

	b := Batch{
		Metrics: []MetricRow{
			{NodeID: n.ID, Metric: Metric{T: 3600, CPU: 10, MemUsed: 100, NetIn: 10}},
			{NodeID: n.ID, Metric: Metric{T: 3660, CPU: 30, MemUsed: 300, NetIn: 30}},
			{NodeID: 999, Metric: Metric{T: 3600, CPU: 50}}, // deleted node: skipped
		},
		Pings:   []PingRow{{NodeID: n.ID, Target: "CT", TS: 3600, AvgMS: 12, Loss: 0}, {NodeID: n.ID, Target: "CT", TS: 3660, AvgMS: -1, Loss: 100}},
		Infos:   []InfoRow{{NodeID: n.ID, NodeInfo: NodeInfo{OS: "Debian", LastIP: "1.2.3.4", UpdatedAt: 1}}},
		Traffic: []TrafficRow{{NodeID: n.ID, Traffic: Traffic{PeriodStart: 1, Up: 5, Down: 6}}},
	}
	if err := s.SaveBatch(ctx, b); err != nil {
		t.Fatal(err)
	}

	got, err := s.QueryMetrics(ctx, n.ID, Minute, 0, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CPU != 20 || got[0].MemUsed != 200 {
		t.Fatalf("query: %+v", got)
	}

	if err := s.RollupHours(ctx, 0, 7200); err != nil {
		t.Fatal(err)
	}
	hours, err := s.QueryMetrics(ctx, n.ID, Hour, 0, 3600)
	if err != nil || len(hours) != 1 || hours[0].NetIn != 20 {
		t.Fatalf("hourly: %+v %v", hours, err)
	}

	ping, err := s.QueryPing(ctx, n.ID, 0, 3600)
	if err != nil || len(ping) != 1 || ping[0].Points[0].MS != 12 || ping[0].Points[0].Loss != 50 {
		t.Fatalf("ping: %+v %v", ping, err)
	}

	nodes, err := s.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].Info.OS != "Debian" {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	tr, err := s.LoadTraffic(ctx)
	if err != nil || tr[n.ID].Down != 6 {
		t.Fatalf("traffic: %+v %v", tr, err)
	}

	if err := s.Cleanup(ctx, 4000, 4000, 0, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.QueryMetrics(ctx, n.ID, Minute, 0, 60); len(got) != 0 {
		t.Fatalf("cleanup left %d rows", len(got))
	}

	if err := s.DeleteNode(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNode(ctx, n.ID); err != ErrNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if s.Settings().SiteName != "Fenghuo" {
		t.Fatal("default settings not loaded")
	}
	v := s.Settings()
	v.SiteName = "Mine"
	if err := s.SaveSettings(ctx, v); err != nil {
		t.Fatal(err)
	}
	if s.Settings().SiteName != "Mine" {
		t.Fatal("settings cache not updated")
	}
}
