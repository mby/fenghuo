package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/server/store"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestSendSanitizedErrors(t *testing.T) {
	const secret = "url-and-body-secret"
	for _, tc := range []struct {
		name, typ, config, code string
		status                  int
		body                    string
		err                     error
	}{
		{"transport", "webhook", `{"url":"https://example.com/url-and-body-secret?token=url-and-body-secret"}`, "transport_error", 0, "", errors.New(secret)},
		{"http echo", "webhook", `{"url":"https://example.com/url-and-body-secret"}`, "http_error", 500, secret, nil},
		{"invalid URL", "webhook", `{"url":"https://%url-and-body-secret"}`, "invalid_config", 0, "", nil},
		{"invalid type", "url-and-body-secret", `{}`, "invalid_config", 0, "", nil},
		{"canceled", "webhook", `{"url":"https://example.com"}`, "canceled", 0, "", context.Canceled},
		{"timeout", "webhook", `{"url":"https://example.com"}`, "timeout", 0, "", context.DeadlineExceeded},
		{"telegram false", "telegram", `{"bot_token":"123456:ABCdefGhIJKlmNoPQRstuVWXyz0123456789","chat_id":"1"}`, "telegram_error", 200, `{"ok":false,"description":"url-and-body-secret"}`, nil},
		{"telegram malformed", "telegram", `{"bot_token":"123456:ABCdefGhIJKlmNoPQRstuVWXyz0123456789","chat_id":"1"}`, "telegram_error", 200, secret, nil},
		{"telegram success", "telegram", `{"bot_token":"123456:ABCdefGhIJKlmNoPQRstuVWXyz0123456789","chat_id":"1"}`, "", 200, `{"ok":true}`, nil},
		{"webhook success", "webhook", `{"url":"https://example.com"}`, "", 204, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := New(nil, true, nil)
			n.hc = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, &url.Error{Op: "POST", URL: r.URL.String(), Err: tc.err}
				}
				return testResponse(tc.status, tc.body), nil
			})}
			err := n.Send(context.Background(), store.Channel{Type: tc.typ, Config: json.RawMessage(tc.config)}, Message{})
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != tc.code {
				t.Fatalf("got %v want %s", err, tc.code)
			}
		})
	}
}

func TestNotificationTransportBoundaries(t *testing.T) {
	c := newHTTPClient(false)
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("proxy bypasses destination SSRF checks")
	}
	if err := c.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("credentials may follow redirect: %v", err)
	}
	for _, raw := range []string{
		`{"url":"https://example.com"} {"url":"https://elsewhere.example"}`,
		`{"url":"https://example.com"} trailing`,
	} {
		if _, err := ValidateConfig("webhook", json.RawMessage(raw)); err == nil {
			t.Fatal("trailing config accepted")
		}
	}
}

func queueFixture(t *testing.T) (*store.Store, *Notifier, store.Channel) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ch := store.Channel{Name: "webhook", Type: "webhook", Config: json.RawMessage(`{"url":"https://example.com/old-secret"}`), Enabled: true}
	if err := s.CreateChannel(context.Background(), &ch); err != nil {
		t.Fatal(err)
	}
	n := New(s, true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s, n, ch
}

func waitDeliveryStatus(t *testing.T, s *store.Store, status string) []store.Delivery {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := s.ListDeliveries(context.Background(), 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) > 0 && rows[0].Status == status {
			return rows
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delivery never reached %s", status)
	return nil
}

func TestQueueRetryUsesCurrentConfig(t *testing.T) {
	s, n, ch := queueFixture(t)
	var logs bytes.Buffer
	n.log = slog.New(slog.NewTextHandler(&logs, nil))
	var calls []string
	n.hc = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.Path)
		if len(calls) == 1 {
			return testResponse(503, "old-secret"), nil
		}
		return testResponse(204, ""), nil
	})}
	ctx := context.Background()
	n.Broadcast(Message{Kind: "latency", Firing: true})
	if len(calls) != 0 {
		t.Fatal("Broadcast sent outside Run")
	}
	d, err := s.ClaimDelivery(ctx, time.Now())
	if err != nil || d == nil {
		t.Fatalf("%+v %v", d, err)
	}
	n.deliver(ctx, *d)
	rows := waitDeliveryStatus(t, s, "retrying")
	ch.Config = json.RawMessage(`{"url":"https://example.com/new-secret"}`)
	if err := s.UpdateChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	d, err = s.ClaimDelivery(ctx, time.Unix(rows[0].NextAttemptAt, 0))
	if err != nil || d == nil {
		t.Fatalf("%+v %v", d, err)
	}
	n.deliver(ctx, *d)
	rows = waitDeliveryStatus(t, s, "succeeded")
	if rows[0].Attempts != 2 || len(calls) != 2 || calls[0] != "/old-secret" || calls[1] != "/new-secret" {
		t.Fatalf("calls=%v delivery=%+v", calls, rows[0])
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatalf("leaked log: %s", &logs)
	}
}

func TestQueueRunLifecycleAndConcurrency(t *testing.T) {
	s, n, ch := queueFixture(t)
	for i := 0; i < 2; i++ {
		ch.ID = 0
		if err := s.CreateChannel(context.Background(), &ch); err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan struct{}, 3)
	var active, maxActive atomic.Int32
	n.hc = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); count > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, count) {
				break
			}
		}
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	n.Broadcast(Message{Kind: "latency"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); n.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	for i := 0; i < deliveryWorkers; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not start")
		}
	}
	duplicate := make(chan struct{})
	go func() { n.Run(ctx); close(duplicate) }()
	select {
	case <-duplicate:
	case <-time.After(time.Second):
		t.Fatal("duplicate Run started workers")
	}
	rows, err := s.ListDeliveries(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	var sending, pending int
	for _, d := range rows {
		if d.Status == "sending" {
			sending++
		}
		if d.Status == "pending" {
			pending++
		}
	}
	if sending != deliveryWorkers || pending != 1 || maxActive.Load() != deliveryWorkers {
		t.Fatalf("sending=%d pending=%d max=%d", sending, pending, maxActive.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not join workers")
	}
	if active.Load() != 0 {
		t.Fatal("send outlived Run")
	}
	rows, err = s.ListDeliveries(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range rows {
		if d.Status == "sending" {
			t.Fatalf("shutdown left active lease: %+v", d)
		}
	}
}

func TestQueueDisableCancelsInFlight(t *testing.T) {
	for _, action := range []string{"disable", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, n, ch := queueFixture(t)
			started := make(chan struct{}, 1)
			canceled := make(chan struct{}, 1)
			n.hc = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				started <- struct{}{}
				<-r.Context().Done()
				canceled <- struct{}{}
				return nil, r.Context().Err()
			})}
			n.Broadcast(Message{Kind: "packet_loss"})
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); n.Run(ctx) }()
			t.Cleanup(func() { cancel(); <-done })
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("send did not start")
			}
			if action == "disable" {
				ch.Enabled = false
				if err := s.UpdateChannel(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
			} else if err := s.DeleteChannel(context.Background(), ch.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("channel cancellation did not stop request")
			}
			rows := waitDeliveryStatus(t, s, "canceled")
			if rows[0].Attempts != 1 || rows[0].History[0].Status != "canceled" {
				t.Fatalf("%+v", rows)
			}
		})
	}
}
