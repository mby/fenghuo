package store

import (
	"context"
	"testing"
	"time"
)

func availabilityNode(t *testing.T, s *Store) Node {
	t.Helper()
	n := Node{Name: "availability", TokenHash: "hash", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAvailabilitySummary(t *testing.T) {
	s := openTest(t)
	n := availabilityNode(t, s)
	ctx := context.Background()
	for _, span := range []AvailabilityInterval{{Start: 100, End: 160, State: "online"}, {Start: 160, End: 220, State: "online"}, {Start: 280, End: 340, State: "offline"}, {Start: 340, End: 400, State: "offline"}, {Start: 460, End: 520, State: "offline"}} {
		if err := s.SaveAvailability(ctx, []AvailabilityRow{{NodeID: n.ID, UUID: n.UUID, AvailabilityInterval: span}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name                                        string
		from, to, online, offline, unknown, longest int64
		count                                       int
	}{
		{"full", 100, 580, 120, 180, 180, 120, 2},
		{"clipped", 130, 370, 90, 90, 60, 90, 1},
		{"empty", 600, 660, 0, 0, 60, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.QueryAvailability(ctx, n.ID, tc.from, tc.to)
			if err != nil {
				t.Fatal(err)
			}
			if got.OnlineSeconds != tc.online || got.OfflineSeconds != tc.offline || got.UnknownSeconds != tc.unknown || got.LongestOutageSeconds != tc.longest || got.OutageCount != tc.count {
				t.Fatalf("summary: %+v", got)
			}
			known := tc.online + tc.offline
			if known == 0 {
				if got.UptimePercent != nil {
					t.Fatal("empty uptime must be null")
				}
			} else if got.UptimePercent == nil || *got.UptimePercent != 100*float64(tc.online)/float64(known) {
				t.Fatal("incorrect uptime")
			}
			if got.CoveragePercent != 100*float64(known)/float64(tc.to-tc.from) {
				t.Fatal("incorrect coverage")
			}
			for i, span := range got.Timeline {
				if i > 0 && span.Start != got.Timeline[i-1].End {
					t.Fatal("timeline gap missing unknown")
				}
			}
		})
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM availability_intervals").Scan(&count); err != nil || count != 3 {
		t.Fatalf("coalescing: %d %v", count, err)
	}
}

func TestAvailabilityRetentionAndCascade(t *testing.T) {
	s := openTest(t)
	n := availabilityNode(t, s)
	ctx := context.Background()
	now := time.Now()
	cut := now.Add(-AvailabilityRetention).Unix()
	for _, start := range []int64{cut - 120, cut + 60, now.Add(-90 * 24 * time.Hour).Unix()} {
		if err := s.SaveAvailability(ctx, []AvailabilityRow{{NodeID: n.ID, UUID: n.UUID, AvailabilityInterval: AvailabilityInterval{Start: start, End: start + 60, State: "offline"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CleanupAvailability(ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM availability_intervals").Scan(&count); err != nil || count != 2 {
		t.Fatalf("retention: %d %v", count, err)
	}
	if err := s.DeleteNode(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM availability_intervals").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade: %d %v", count, err)
	}
	if err := s.SaveAvailability(ctx, []AvailabilityRow{{NodeID: n.ID, UUID: n.UUID, AvailabilityInterval: AvailabilityInterval{Start: 1, End: 2, State: "online"}}}); err != nil {
		t.Fatal("deleted snapshot should be skipped", err)
	}
}

func TestAvailabilityTimelineBoundAndValidation(t *testing.T) {
	s := openTest(t)
	n := availabilityNode(t, s)
	ctx := context.Background()
	batch := make([]AvailabilityRow, 120)
	for i := range batch {
		batch[i] = AvailabilityRow{NodeID: n.ID, UUID: n.UUID, AvailabilityInterval: AvailabilityInterval{Start: int64(i * 120), End: int64(i*120 + 60), State: "offline"}}
	}
	if err := s.SaveAvailability(ctx, batch); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryAvailability(ctx, n.ID, 0, 14400)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Timeline) != AvailabilityTimelineLimit || !got.TimelineTruncated || got.OutageCount != 120 || got.OfflineSeconds != 7200 {
		t.Fatalf("bounded timeline lost totals: %+v", got)
	}
	if err := s.SaveAvailability(ctx, make([]AvailabilityRow, AvailabilityBatchSize+1)); err == nil {
		t.Fatal("unbounded batch accepted")
	}
	for _, window := range [][2]int64{{1, 1}, {2, 1}, {0, 91 * 86400}} {
		if _, err := s.QueryAvailability(ctx, n.ID, window[0], window[1]); err == nil {
			t.Fatal("invalid window accepted")
		}
	}
}

func TestAvailabilityBatchRollbackAndNodeIdentity(t *testing.T) {
	s := openTest(t)
	n := availabilityNode(t, s)
	ctx := context.Background()
	row := AvailabilityRow{NodeID: n.ID, UUID: n.UUID, AvailabilityInterval: AvailabilityInterval{Start: 100, End: 160, State: "online"}}
	if err := s.SaveAvailability(ctx, []AvailabilityRow{row}); err != nil {
		t.Fatal(err)
	}
	adjacent := row
	adjacent.Start, adjacent.End = 160, 220
	overlap := row
	overlap.Start, overlap.End = 180, 240
	if err := s.SaveAvailability(ctx, []AvailabilityRow{adjacent, overlap}); err == nil {
		t.Fatal("overlapping batch accepted")
	}
	adjacent.UUID = "stale-identity"
	if err := s.SaveAvailability(ctx, []AvailabilityRow{adjacent}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryAvailability(ctx, n.ID, 100, 240)
	if err != nil {
		t.Fatal(err)
	}
	if got.OnlineSeconds != 60 || got.UnknownSeconds != 80 {
		t.Fatalf("rollback or UUID guard failed: %+v", got)
	}
}
