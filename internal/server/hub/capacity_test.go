package hub

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestCapacityBackgroundPersistenceAndReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "capacity.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := store.Node{Name: "capacity", TokenHash: "hash", TrafficResetDay: 1, TrafficMode: "sum"}
	if err := st.CreateNode(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	read := func() store.Node {
		t.Helper()
		nodes, err := st.ListNodes(ctx)
		if err != nil || len(nodes) != 1 {
			t.Fatalf("nodes: %+v %v", nodes, err)
		}
		return nodes[0]
	}
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	h := New(start)
	h.Reload([]store.Node{read()}, nil, start)
	if info := read().Info; info.MemTotal != 0 || info.DiskTotal != 0 {
		t.Fatalf("new capacity: %+v", info)
	}
	var previousMem, previousDisk uint64
	for i, tt := range []struct {
		name      string
		mem, disk uint64
		info      *proto.Info
		wantDirty bool
	}{
		{"initial", 8 << 30, 100 << 30, &proto.Info{OS: "Linux"}, true},
		{"unchanged without info", 8 << 30, 100 << 30, nil, false},
		{"memory changed without info", 16 << 30, 100 << 30, nil, true},
		{"disk changed without info", 16 << 30, 200 << 30, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := start.Add(time.Duration(i) * 3 * time.Second)
			r := &proto.Report{V: proto.Version, MemTotal: tt.mem, DiskTotal: tt.disk, MemUsed: 123, DiskUsed: 456, NetIn: 789, Info: tt.info}
			if _, err := h.Ingest(cfg.TokenHash, r, "", now); err != nil {
				t.Fatal(err)
			}
			if info := read().Info; info.MemTotal != previousMem || info.DiskTotal != previousDisk {
				t.Fatalf("ingest persisted before background flush: %+v", info)
			}
			// Configuration reload must not replace unflushed observed capacities.
			h.Reload([]store.Node{read()}, nil, now)
			v := h.Snapshot(now)[0]
			if v.Info.MemTotal != tt.mem || v.Info.DiskTotal != tt.disk || !v.LastSeen.Equal(now) {
				t.Fatalf("live capacity/last seen: %+v", v)
			}
			b := h.Drain(now, false)
			if (len(b.Infos) == 1) != tt.wantDirty || len(b.Metrics) != 0 {
				t.Fatalf("capacity dirty/current minute: %+v", b)
			}
			if err := st.SaveBatch(ctx, b); err != nil {
				t.Fatal(err)
			}
			info := read().Info
			if info.MemTotal != tt.mem || info.DiskTotal != tt.disk || info.OS != "Linux" || info.UpdatedAt != start.Unix() {
				t.Fatalf("stored capacity/system timestamp: %+v", info)
			}
			if b := h.Drain(now, false); len(b.Infos) != 0 {
				t.Fatalf("dirty not cleared: %+v", b)
			}
			previousMem, previousDisk = tt.mem, tt.disk
		})
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restart := start.Add(time.Hour)
	reloaded := New(restart)
	reloaded.Reload([]store.Node{read()}, nil, restart)
	v := reloaded.Snapshot(restart)[0]
	if v.Info.MemTotal != previousMem || v.Info.DiskTotal != previousDisk || v.Status != nil || !v.LastSeen.IsZero() {
		t.Fatalf("restart capacity/live state: %+v", v)
	}
}
