//go:build linux

package agent

import (
	"context"
	"testing"
)

func TestICMPLoopback(t *testing.T) {
	ms, err := icmpLatency(context.Background(), "127.0.0.1")
	if err == "permission" || err == "unsupported" {
		t.Skip("unprivileged ping sockets unavailable")
	}
	if err != "" || ms < 0 {
		t.Fatalf("loopback ICMP failed: ms=%v local_error=%q", ms, err)
	}
}
