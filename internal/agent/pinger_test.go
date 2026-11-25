package agent

import (
	"context"
	"net"
	"testing"
)

func TestTCPLatency(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		addr string
		ok   bool
	}{
		{"reachable", context.Background(), listener.Addr().String(), true},
		{"invalid address", context.Background(), "missing-port", false},
		{"cancelled", cancelled, listener.Addr().String(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms := tcpLatency(tc.ctx, tc.addr)
			if (ms >= 0) != tc.ok || (!tc.ok && ms != -1) {
				t.Fatalf("latency %v, want success %v", ms, tc.ok)
			}
		})
	}
}
