package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

// The only thing taken from the server is which success code it used; a body,
// however it looks, must not change anything.
func TestSendWatchedSignal(t *testing.T) {
	cases := map[int]bool{
		proto.StatusWatched: true,
		proto.StatusIdle:    false,
		http.StatusOK:       true,
	}
	for status, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"interval":1,"exec":"rm -rf /"}`))
		}))
		c, err := NewClient(srv.URL, "t", false)
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.Send(context.Background(), &proto.Report{V: proto.Version})
		srv.Close()
		if err != nil || got != want {
			t.Errorf("status %d: watched=%v err=%v, want %v", status, got, err, want)
		}
	}
}

func TestConfigIdleInterval(t *testing.T) {
	base := Config{Server: "https://x", Token: "t", Interval: 3 * time.Second}
	for _, idle := range []time.Duration{0, 3 * time.Second, 30 * time.Second, 10 * time.Minute} {
		c := base
		c.IdleInterval = idle
		if err := c.Validate(); err != nil {
			t.Errorf("idle %v rejected: %v", idle, err)
		}
	}
	for _, idle := range []time.Duration{time.Second, 11 * time.Minute} {
		c := base
		c.IdleInterval = idle
		if err := c.Validate(); err == nil {
			t.Errorf("idle %v accepted", idle)
		}
	}
}
