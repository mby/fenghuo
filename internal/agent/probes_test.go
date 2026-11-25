package agent

import (
	"github.com/mby/fenghuo/internal/proto"
	"testing"
	"time"
)

func TestProbeQueueRetriesAndAck(t *testing.T) {
	q := newProbeQueue()
	now := time.Now()
	q.add(proto.Probe{Name: "a", Kind: "icmp", Time: now.UnixMilli(), MS: 20})
	first, _ := q.batch(now)
	q.add(proto.Probe{Name: "a", Kind: "icmp", Time: now.UnixMilli(), MS: -1})
	retry, _ := q.batch(now)
	if len(retry) != 2 || retry[0].Seq != first[0].Seq {
		t.Fatal("failed report lost its samples")
	}
	first[0].MS = 999
	if retry[0].MS != 20 {
		t.Fatal("batch aliases queue")
	}
	q.ack(retry[0].Seq)
	last, _ := q.batch(now)
	if len(last) != 1 || last[0].MS != -1 {
		t.Fatal("ack discarded unacknowledged attempt")
	}
	for range probeQueueLimit + 20 {
		q.add(proto.Probe{Time: now.UnixMilli()})
	}
	batch, dropped := q.batch(now)
	if len(q.samples) != probeQueueLimit || len(batch) != proto.MaxProbeBatch || dropped != 21 {
		t.Fatalf("bounds: %d %d %d", len(q.samples), len(batch), dropped)
	}
	batch, dropped = q.batch(now.Add(6 * time.Minute))
	if len(batch) != 0 || dropped == 0 {
		t.Fatal("expired samples retained")
	}
}

func TestEchoReplyValidation(t *testing.T) {
	request := echoRequest([]byte("0123456789abcdef"))
	valid := append([]byte(nil), request...)
	valid[0] = 0
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
		want   bool
	}{
		{"reply", func(b []byte) []byte { return b }, true},
		{"kernel identifier", func(b []byte) []byte { b[4] = 99; return b }, true},
		{"request", func(b []byte) []byte { b[0] = 8; return b }, false},
		{"wrong sequence", func(b []byte) []byte { b[7]++; return b }, false},
		{"foreign payload", func(b []byte) []byte { b[9]++; return b }, false},
		{"truncated", func(b []byte) []byte { return b[:7] }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validEcho(tc.mutate(append([]byte(nil), valid...)), request) != tc.want {
				t.Fatal("unexpected echo validation")
			}
		})
	}
	var sum uint32
	for i := 0; i < len(request); i += 2 {
		sum += uint32(request[i])<<8 | uint32(request[i+1])
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	if sum != 65535 {
		t.Fatal("bad request checksum")
	}
}

func TestICMPTargetValidation(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"示例目标=192.0.2.10", true}, {"", true}, {"x=1.2.3.4:80", false}, {"x=999.1.1.1", false}, {"x=::1", false}, {"x=1.2.3.4,x=1.1.1.1", false},
	} {
		_, err := ParseICMPTargets(tc.raw)
		if (err == nil) != tc.valid {
			t.Fatalf("%q: %v", tc.raw, err)
		}
	}
}
