package availability

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestObservationBoundaries(t *testing.T) {
	base := time.Unix(1000, 0)
	view := hub.View{Node: store.Node{ID: 1, UUID: "a"}, Online: true, LastSeen: base, Status: &proto.Report{}}
	for _, tc := range []struct {
		name   string
		times  []int
		states []string
		want   []store.AvailabilityInterval
	}{
		{"single sample", []int{0}, []string{"online"}, nil},
		{"online offline recovery", []int{0, 60, 120, 180, 240, 300}, []string{"online", "online", "offline", "offline", "online", "online"}, []store.AvailabilityInterval{{Start: 1000, End: 1060, State: "online"}, {Start: 1120, End: 1180, State: "offline"}, {Start: 1240, End: 1300, State: "online"}}},
		{"stalled observer", []int{0, 60, 300, 360}, []string{"online", "online", "online", "online"}, []store.AvailabilityInterval{{Start: 1000, End: 1060, State: "online"}, {Start: 1300, End: 1360, State: "online"}}},
		{"restart grace", []int{0, 60, 120}, []string{"unknown", "unknown", "online"}, nil},
		{"never reported", []int{0, 60, 120, 180}, []string{"unknown", "unknown", "unknown", "unknown"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{}
			var got []store.AvailabilityInterval
			for i, seconds := range tc.times {
				v := view
				v.Online = tc.states[i] != "offline"
				if tc.states[i] == "unknown" {
					v.LastSeen = time.Time{}
					v.Status = nil
					v.Info.UpdatedAt = 500
				}
				for _, row := range r.intervals(base.Add(time.Duration(seconds)*time.Second), []hub.View{v}) {
					got = append(got, row.AvailabilityInterval)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v; want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %+v; want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestRestartGapPersistsAsUnknown(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "availability.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	n := store.Node{Name: "node", TokenHash: "hash", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Truncate(time.Second)
	for run := range 2 {
		if run > 0 {
			s, err = store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
		}
		r := &Runner{}
		v := hub.View{Node: n, Online: true, LastSeen: base, Status: &proto.Report{}}
		for _, offset := range []int{run * 300, run*300 + 60} {
			if err := s.SaveAvailability(ctx, r.intervals(base.Add(time.Duration(offset)*time.Second), []hub.View{v})); err != nil {
				t.Fatal(err)
			}
		}
		if run == 0 {
			s.Close()
		}
	}
	defer s.Close()
	got, err := s.QueryAvailability(ctx, n.ID, base.Unix(), base.Unix()+420)
	if err != nil {
		t.Fatal(err)
	}
	if got.OnlineSeconds != 120 || got.UnknownSeconds != 300 || got.OfflineSeconds != 0 {
		t.Fatalf("summary: %+v", got)
	}
}

func TestDeletedNodeIdentityNotBridged(t *testing.T) {
	r := &Runner{}
	now := time.Unix(1000, 0)
	v := hub.View{Node: store.Node{ID: 1, UUID: "old"}, LastSeen: now, Status: &proto.Report{}, Online: true}
	r.intervals(now, []hub.View{v})
	v.Node.UUID = "new"
	if got := r.intervals(now.Add(time.Minute), []hub.View{v}); len(got) != 0 {
		t.Fatal(got)
	}
}
