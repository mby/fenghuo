package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func deliveryChannel(t *testing.T, s *Store) Channel {
	t.Helper()
	c := Channel{Name: "hook", Type: "webhook", Config: json.RawMessage(`{"url":"https://example.com/secret?token=private"}`), Enabled: true}
	if err := s.CreateChannel(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func enqueueTestDelivery(t *testing.T, s *Store, now time.Time) {
	t.Helper()
	if err := s.EnqueueDeliveries(context.Background(), json.RawMessage(`{"title":"message"}`), "latency", true, now); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryRetryAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		succeedAt int
		status    string
	}{
		{"immediate", 1, "succeeded"}, {"retry", 3, "succeeded"}, {"exhausted", 0, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			deliveryChannel(t, s)
			ctx := context.Background()
			now := time.Now().Truncate(time.Second)
			enqueueTestDelivery(t, s, now)
			for attempt := 1; attempt <= DeliveryMaxAttempts; attempt++ {
				d, err := s.ClaimDelivery(ctx, now)
				if err != nil || d == nil || d.Attempts != attempt {
					t.Fatalf("claim %d: %+v %v", attempt, d, err)
				}
				if duplicate, err := s.ClaimDelivery(ctx, now); err != nil || duplicate != nil {
					t.Fatalf("duplicate: %+v %v", duplicate, err)
				}
				code := "https://example.com/secret?token=private"
				if attempt == tc.succeedAt {
					code = ""
				}
				if err := s.FinishDelivery(ctx, *d, code, now); err != nil {
					t.Fatal(err)
				}
				if code == "" || attempt == DeliveryMaxAttempts {
					break
				}
				now = now.Add(DeliveryBackoff(attempt))
				if early, err := s.ClaimDelivery(ctx, now.Add(-time.Second)); err != nil || early != nil {
					t.Fatalf("early retry: %+v %v", early, err)
				}
			}
			rows, err := s.ListDeliveries(ctx, 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("%+v %v", rows, err)
			}
			d := rows[0]
			if d.Status != tc.status || len(d.History) != d.Attempts || d.NextAttemptAt != 0 {
				t.Fatalf("%+v", d)
			}
			for i, a := range d.History {
				if a.Number != i+1 || a.FinishedAt == 0 {
					t.Fatalf("%+v", a)
				}
			}
			raw, _ := json.Marshal(d)
			if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "token") || strings.Contains(string(raw), "payload") {
				t.Fatalf("leaked: %s", raw)
			}
			var payload string
			if err := s.db.QueryRow("SELECT payload FROM notification_deliveries WHERE id = ?", d.ID).Scan(&payload); err != nil || payload != "" {
				t.Fatalf("terminal payload retained: %q %v", payload, err)
			}
		})
	}
}

func TestDeliveryCancellation(t *testing.T) {
	for _, action := range []string{"disable", "delete"} {
		for _, claim := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "/pending", true: "/sending"}[claim], func(t *testing.T) {
				s := openTest(t)
				ch := deliveryChannel(t, s)
				ctx, now := context.Background(), time.Now()
				enqueueTestDelivery(t, s, now)
				var d *Delivery
				if claim {
					var err error
					d, err = s.ClaimDelivery(ctx, now)
					if err != nil || d == nil {
						t.Fatalf("%+v %v", d, err)
					}
				}
				if action == "disable" {
					ch.Enabled = false
					if err := s.UpdateChannel(ctx, ch); err != nil {
						t.Fatal(err)
					}
					ch.Enabled = true
					if err := s.UpdateChannel(ctx, ch); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := s.DeleteChannel(ctx, ch.ID); err != nil {
						t.Fatal(err)
					}
					deliveryChannel(t, s) // SQLite may reuse the deleted ID.
				}
				if d != nil {
					if err := s.FinishDelivery(ctx, *d, "", now); err != nil {
						t.Fatal(err)
					}
				}
				rows, err := s.ListDeliveries(ctx, 10)
				if err != nil || len(rows) != 1 || rows[0].Status != "canceled" {
					t.Fatalf("%+v %v", rows, err)
				}
				if claim && rows[0].History[0].Status != "canceled" {
					t.Fatalf("%+v", rows[0])
				}
				if d, err := s.ClaimDelivery(ctx, now.Add(time.Hour)); err != nil || d != nil {
					t.Fatalf("resurrected: %+v %v", d, err)
				}
			})
		}
	}
}

func TestDeliveryCapAndAtomicAlert(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	ch := deliveryChannel(t, s)
	// Seed a full queue in one statement; exercise the actual atomic admission
	// check instead of spending the test on 1000 individual disk transactions.
	_, err := s.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO notification_deliveries(channel_id,payload,kind,firing,created_at,updated_at,next_attempt_at)
		SELECT ?, '{}', 'latency', 1, ?, ?, ? FROM n`, DeliveryQueueCap, ch.ID, now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueDeliveries(ctx, json.RawMessage("{}"), "latency", true, now); !errors.Is(err, ErrDeliveryQueueFull) {
		t.Fatalf("cap: %v", err)
	}
	node := Node{Name: "node", TokenHash: "node", TrafficMode: "sum", TrafficResetDay: 1}
	if err := s.CreateNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	rule := Rule{Name: "latency", Kind: "latency", Enabled: true}
	if err := s.CreateRule(ctx, &rule); err != nil {
		t.Fatal(err)
	}
	state := AlertState{RuleID: rule.ID, NodeID: node.ID, FiredAt: now.Unix()}
	event := Event{RuleID: rule.ID, NodeID: node.ID, Kind: "latency", Firing: true, CreatedAt: now.Unix()}
	if err := s.CommitAlertDelivery(ctx, state, event, json.RawMessage("{}")); !errors.Is(err, ErrDeliveryQueueFull) {
		t.Fatalf("atomic cap: %v", err)
	}
	states, err := s.ListAlertStates(ctx)
	if err != nil || len(states) != 0 {
		t.Fatalf("state committed without delivery: %+v %v", states, err)
	}
	events, err := s.ListEvents(ctx, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("event committed without delivery: %+v %v", events, err)
	}
	ch.Enabled = false
	if err := s.UpdateChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	ch.Enabled = true
	if err := s.UpdateChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitAlertDelivery(ctx, state, event, json.RawMessage("{}")); err != nil {
		t.Fatal(err)
	}
	states, err = s.ListAlertStates(ctx)
	if err != nil || len(states) != 1 {
		t.Fatalf("%+v %v", states, err)
	}
	_, err = s.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO notification_deliveries(channel_id,payload,kind,firing,created_at,updated_at,next_attempt_at)
		SELECT ?, '{}', 'latency', 1, ?, ?, ? FROM n`, DeliveryQueueCap-1, ch.ID, now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	event.Firing = false
	if err := s.CommitAlertDelivery(ctx, state, event, json.RawMessage("{}")); !errors.Is(err, ErrDeliveryQueueFull) {
		t.Fatalf("recovery cap: %v", err)
	}
	states, err = s.ListAlertStates(ctx)
	if err != nil || len(states) != 1 {
		t.Fatalf("lost firing state without queued recovery: %+v %v", states, err)
	}
	events, err = s.ListEvents(ctx, 10)
	if err != nil || len(events) != 1 || !events[0].Firing {
		t.Fatalf("recovery event without queued recovery: %+v %v", events, err)
	}
}

func TestDeliveryCrashAttemptsAreFinite(t *testing.T) {
	s := openTest(t)
	deliveryChannel(t, s)
	ctx, now := context.Background(), time.Now().Truncate(time.Second)
	enqueueTestDelivery(t, s, now)
	for attempt := 1; attempt <= DeliveryMaxAttempts; attempt++ {
		d, err := s.ClaimDelivery(ctx, now)
		if err != nil || d == nil || d.Attempts != attempt {
			t.Fatalf("attempt %d: %+v %v", attempt, d, err)
		}
		now = now.Add(DeliveryLease)
		if err := s.MaintainDeliveries(ctx, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(DeliveryBackoff(DeliveryMaxAttempts))
	}
	rows, err := s.ListDeliveries(ctx, 10)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || len(rows[0].History) != DeliveryMaxAttempts {
		t.Fatalf("%+v %v", rows, err)
	}
	if d, err := s.ClaimDelivery(ctx, now.Add(24*time.Hour)); err != nil || d != nil {
		t.Fatalf("infinite crash retries: %+v %v", d, err)
	}
}

func TestDeliveryRestartAndRetention(t *testing.T) {
	ctx, now := context.Background(), time.Now().Truncate(time.Second)
	path := filepath.Join(t.TempDir(), "durable.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	deliveryChannel(t, s)
	enqueueTestDelivery(t, s, now)
	d, err := s.ClaimDelivery(ctx, now)
	if err != nil || d == nil {
		t.Fatalf("%+v %v", d, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now = now.Add(DeliveryLease)
	if err := s.MaintainDeliveries(ctx, now); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListDeliveries(ctx, 10)
	if err != nil || len(rows) != 1 || rows[0].Status != "retrying" || rows[0].History[0].Status != "interrupted" {
		t.Fatalf("%+v %v", rows, err)
	}
	now = time.Unix(rows[0].NextAttemptAt, 0)
	d, err = s.ClaimDelivery(ctx, now)
	if err != nil || d == nil || d.Attempts != 2 {
		t.Fatalf("%+v %v", d, err)
	}
	if err := s.FinishDelivery(ctx, *d, "", now); err != nil {
		t.Fatal(err)
	}
	enqueueTestDelivery(t, s, now) // Active deliveries survive history retention.
	if err := s.MaintainDeliveries(ctx, now.Add(DeliveryRetention+time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListDeliveries(ctx, 10)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" {
		t.Fatalf("%+v %v", rows, err)
	}
	var attempts int
	if err := s.db.QueryRow("SELECT count(*) FROM notification_attempts").Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("orphaned attempts: %d %v", attempts, err)
	}
}

func TestDeliveryHistoryCapAndFIFO(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	ch := deliveryChannel(t, s)
	_, err := s.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO notification_deliveries(channel_id,payload,kind,firing,status,created_at,updated_at,next_attempt_at)
		SELECT ?, '', 'latency', 1, 'succeeded', ?, ?, 0 FROM n`, DeliveryHistoryCap+10, ch.ID, now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MaintainDeliveries(ctx, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM notification_deliveries").Scan(&count); err != nil || count != DeliveryHistoryCap {
		t.Fatalf("history cap: %d %v", count, err)
	}
	enqueueTestDelivery(t, s, now)
	enqueueTestDelivery(t, s, now)
	d, err := s.ClaimDelivery(ctx, now)
	if err != nil || d == nil {
		t.Fatalf("%+v %v", d, err)
	}
	if err := s.FinishDelivery(ctx, *d, "timeout", now); err != nil {
		t.Fatal(err)
	}
	if next, err := s.ClaimDelivery(ctx, now); err != nil || next != nil {
		t.Fatalf("overtook retry: %+v %v", next, err)
	}
}
