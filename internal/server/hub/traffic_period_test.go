package hub

import (
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/mby/fenghuo/internal/server/store"
)

func TestBillingPeriodBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name             string
		day              int
		now, start, next string
	}{
		{"before January reset", 31, "2026-01-30T23:59:59Z", "2025-12-31T00:00:00Z", "2026-01-31T00:00:00Z"},
		{"January boundary", 31, "2026-01-31T00:00:00Z", "2026-01-31T00:00:00Z", "2026-02-28T00:00:00Z"},
		{"before February reset", 31, "2026-02-27T23:59:59.999999999Z", "2026-01-31T00:00:00Z", "2026-02-28T00:00:00Z"},
		{"February boundary", 31, "2026-02-28T00:00:00Z", "2026-02-28T00:00:00Z", "2026-03-31T00:00:00Z"},
		{"March before original day", 31, "2026-03-30T12:00:00Z", "2026-02-28T00:00:00Z", "2026-03-31T00:00:00Z"},
		{"March boundary", 31, "2026-03-31T00:00:00Z", "2026-03-31T00:00:00Z", "2026-04-30T00:00:00Z"},
		{"thirty day month", 31, "2026-04-30T23:00:00Z", "2026-04-30T00:00:00Z", "2026-05-31T00:00:00Z"},
		{"leap February before reset", 31, "2024-02-28T12:00:00Z", "2024-01-31T00:00:00Z", "2024-02-29T00:00:00Z"},
		{"leap February boundary", 31, "2024-02-29T00:00:00Z", "2024-02-29T00:00:00Z", "2024-03-31T00:00:00Z"},
		{"day 29 nonleap", 29, "2026-02-28T00:00:00Z", "2026-02-28T00:00:00Z", "2026-03-29T00:00:00Z"},
		{"day 30 nonleap", 30, "2026-02-28T00:00:00Z", "2026-02-28T00:00:00Z", "2026-03-30T00:00:00Z"},
		{"year end", 31, "2026-12-31T00:00:00Z", "2026-12-31T00:00:00Z", "2027-01-31T00:00:00Z"},
		{"year start", 1, "2027-01-01T00:00:00Z", "2027-01-01T00:00:00Z", "2027-02-01T00:00:00Z"},
		{"positive offset", 31, "2026-02-28T00:00:00+08:00", "2026-02-28T00:00:00+08:00", "2026-03-31T00:00:00+08:00"},
		{"fractional offset", 15, "2026-01-14T23:59:59+05:45", "2025-12-15T00:00:00+05:45", "2026-01-15T00:00:00+05:45"},
		{"invalid zero fallback", 0, "2026-02-15T00:00:00Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z"},
		{"invalid high fallback", 32, "2026-02-15T00:00:00Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parse := func(value string) time.Time {
				t.Helper()
				v, err := time.Parse(time.RFC3339Nano, value)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			now, start, next := parse(tt.now), parse(tt.start), parse(tt.next)
			if got := PeriodStart(now, tt.day); got != start.Unix() {
				t.Fatalf("start = %v, want %v", time.Unix(got, 0), start)
			}
			if got := NextPeriodStart(now, tt.day); !got.Equal(next) || got.Location() != now.Location() {
				t.Fatalf("next = %v, want %v in %v", got, next, now.Location())
			}
			if got := PeriodStart(next, tt.day); got != next.Unix() {
				t.Fatalf("next reset is not a period start: %d", got)
			}
		})
	}
}

func TestBillingPeriodDSTAndView(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name        string
		month       time.Month
		day, offset int
	}{
		{"spring offset changes before next reset", time.March, 15, -4 * 3600},
		{"fall offset changes before next reset", time.October, 31, -5 * 3600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, tt.month, 1, 12, 0, 0, 0, loc)
			if tt.month == time.October {
				now = time.Date(2026, tt.month, 31, 12, 0, 0, 0, loc)
			}
			nextMonth := tt.month
			nextDay := tt.day
			if tt.month == time.October {
				nextMonth, nextDay = time.November, 30
			}
			want := time.Date(2026, nextMonth, nextDay, 0, 0, 0, 0, loc)
			h := New(now)
			h.Reload([]store.Node{{ID: 1, UUID: "node", TrafficResetDay: tt.day}}, nil, now)
			views := h.Snapshot(now)
			v := views[0]
			if v.NextResetAt != want.Unix() || v.ResetUTCOffset != tt.offset {
				t.Fatalf("future time/offset: %+v, want %v offset %d", v, want, tt.offset)
			}
			_, currentOffset := now.Zone()
			if currentOffset == v.ResetUTCOffset {
				t.Fatal("fixture did not cross a DST change")
			}
			byID, ok := h.ByUUID("node", now)
			if !ok || byID.NextResetAt != v.NextResetAt || byID.ResetUTCOffset != v.ResetUTCOffset {
				t.Fatalf("snapshot/lookup mismatch: %+v", byID)
			}
			if start := PeriodStart(want, tt.day); start != want.Unix() {
				t.Fatalf("DST boundary: %d", start)
			}
		})
	}
}

func TestViewResetsTrafficAtClampedBoundary(t *testing.T) {
	loc := time.FixedZone("server", 8*3600)
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	boundary := time.Date(2026, 2, 28, 0, 0, 0, 0, loc)
	h := New(start)
	h.Reload([]store.Node{{ID: 1, UUID: "node", TrafficResetDay: 31}}, map[int64]store.Traffic{
		1: {PeriodStart: start.Unix(), Up: 10, Down: 20, BootTime: 100, LastRx: 200, LastTx: 300},
	}, start)
	before := h.Snapshot(boundary.Add(-time.Nanosecond))[0]
	if before.NextResetAt != boundary.Unix() || before.Traffic.Up != 10 {
		t.Fatalf("before: %+v", before)
	}
	at := h.Snapshot(boundary)[0]
	wantNext := time.Date(2026, 3, 31, 0, 0, 0, 0, loc)
	if at.NextResetAt != wantNext.Unix() || at.Traffic.PeriodStart != boundary.Unix() || at.Traffic.Up != 0 || at.Traffic.Down != 0 {
		t.Fatalf("at boundary: %+v", at)
	}
	if at.Traffic.BootTime != 100 || at.Traffic.LastRx != 200 || at.Traffic.LastTx != 300 || !at.LastSeen.IsZero() {
		t.Fatalf("reset changed counters/last seen: %+v", at)
	}
	b := h.Drain(boundary, false)
	if len(b.Traffic) != 1 || b.Traffic[0].PeriodStart != boundary.Unix() || b.Traffic[0].Up != 0 {
		t.Fatalf("dirty reset: %+v", b)
	}
}
