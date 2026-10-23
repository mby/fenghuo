package alert

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/store"
)

type recorder struct {
	mu   sync.Mutex
	msgs []notify.Message
}

func (r *recorder) Broadcast(m notify.Message) {
	r.mu.Lock()
	r.msgs = append(r.msgs, m)
	r.mu.Unlock()
}

func (r *recorder) take() []notify.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.msgs
	r.msgs = nil
	return out
}

func TestEngine(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	start := time.Now()
	n := store.Node{Name: "tokyo", TokenHash: "tok", TrafficMode: "sum", TrafficResetDay: 1,
		TrafficLimit: 1000, ExpireAt: start.Add(36 * time.Hour).Unix()}
	if err := st.CreateNode(ctx, &n); err != nil {
		t.Fatal(err)
	}
	for _, r := range []store.Rule{
		{Name: "offline", Kind: KindOffline, Duration: 60, Enabled: true},
		{Name: "cpu", Kind: KindCPU, Threshold: 90, Duration: 20, Enabled: true},
		{Name: "traffic", Kind: KindTraffic, Threshold: 50, Enabled: true},
		{Name: "expire", Kind: KindExpire, Threshold: 3, Enabled: true},
	} {
		if err := st.CreateRule(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}

	h := hub.New(start)
	nodes, _ := st.ListNodes(ctx)
	h.Reload(nodes, nil, start)
	rec := &recorder{}
	e := New(h, st, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := e.Reload(ctx); err != nil {
		t.Fatal(err)
	}

	report := func(at time.Time, cpu float64, rx uint64) {
		t.Helper()
		if _, err := h.Ingest("tok", &proto.Report{V: 1, BootTime: 1, CPU: cpu, NetRx: rx}, "1.1.1.1", at); err != nil {
			t.Fatal(err)
		}
	}
	kinds := func(msgs []notify.Message) map[string]bool {
		out := map[string]bool{}
		for _, m := range msgs {
			out[m.Kind+map[bool]string{true: "+", false: "-"}[m.Firing]] = true
		}
		return out
	}

	// Never-seen node: no offline alert. Expiry fires immediately (1.5 days left).
	e.Evaluate(ctx, start.Add(5*time.Minute))
	if got := kinds(rec.take()); len(got) != 1 || !got["expire+"] {
		t.Fatalf("initial: %v", got)
	}

	t0 := start.Add(10 * time.Minute)
	report(t0, 95, 0)
	report(t0.Add(3*time.Second), 95, 600) // 600 of 1000 bytes
	e.Evaluate(ctx, t0.Add(4*time.Second))
	if got := kinds(rec.take()); len(got) != 1 || !got["traffic+"] {
		t.Fatalf("traffic: %v", got)
	}

	// CPU must stay above the threshold for 20s.
	report(t0.Add(25*time.Second), 95, 600)
	e.Evaluate(ctx, t0.Add(26*time.Second))
	if got := kinds(rec.take()); !got["cpu+"] {
		t.Fatalf("cpu: %v", got)
	}
	report(t0.Add(30*time.Second), 10, 600)
	e.Evaluate(ctx, t0.Add(31*time.Second))
	if got := kinds(rec.take()); !got["cpu-"] {
		t.Fatalf("cpu resolve: %v", got)
	}

	// Silence -> offline after the rule duration, then back online.
	e.Evaluate(ctx, t0.Add(2*time.Minute))
	if got := kinds(rec.take()); !got["offline+"] {
		t.Fatalf("offline: %v", got)
	}
	report(t0.Add(3*time.Minute), 10, 600)
	e.Evaluate(ctx, t0.Add(3*time.Minute+time.Second))
	if got := kinds(rec.take()); !got["offline-"] {
		t.Fatalf("online: %v", got)
	}

	// One-shot alerts survive a restart and do not fire twice.
	e2 := New(h, st, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := e2.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	e2.Evaluate(ctx, t0.Add(3*time.Minute+2*time.Second))
	if got := rec.take(); len(got) != 0 {
		t.Fatalf("re-fired after reload: %+v", got)
	}
}
