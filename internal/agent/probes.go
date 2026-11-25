package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

const probeQueueLimit = 1024

type probeQueue struct {
	mu      sync.Mutex
	session string
	seq     uint64
	samples []proto.Probe
	dropped uint64
}

func newProbeQueue() *probeQueue {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return &probeQueue{session: hex.EncodeToString(id[:])}
}

func (q *probeQueue) add(p proto.Probe) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	p.Seq = q.seq
	if len(q.samples) == probeQueueLimit {
		copy(q.samples, q.samples[1:])
		q.samples = q.samples[:len(q.samples)-1]
		q.dropped++
	}
	q.samples = append(q.samples, p)
}

func (q *probeQueue) batch(now time.Time) ([]proto.Probe, uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.samples) > 0 && now.UnixMilli()-q.samples[0].Time > 300000 {
		q.samples = q.samples[1:]
		q.dropped++
	}
	return append([]proto.Probe(nil), q.samples[:min(len(q.samples), proto.MaxProbeBatch)]...), q.dropped
}

func (q *probeQueue) ack(seq uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := 0
	for i < len(q.samples) && q.samples[i].Seq <= seq {
		i++
	}
	q.samples = append(q.samples[:0], q.samples[i:]...)
}

func (q *probeQueue) start(ctx context.Context, cfg Config) {
	for _, group := range []struct {
		kind    string
		targets []Target
		every   time.Duration
	}{
		{"tcp", cfg.Ping, cfg.PingInterval}, {"icmp", cfg.ICMP, cfg.ICMPInterval},
	} {
		for _, target := range group.targets {
			go func(kind string, t Target, every time.Duration) {
				ticker := time.NewTicker(every)
				defer ticker.Stop()
				for {
					at := time.Now()
					p := proto.Probe{Name: t.Name, Kind: kind, Time: at.UnixMilli()}
					if kind == "icmp" {
						p.MS, p.Error = icmpLatency(ctx, t.Addr)
					} else {
						p.MS = tcpLatency(ctx, t.Addr)
					}
					if ctx.Err() != nil {
						return
					}
					q.add(p)
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}(group.kind, target, group.every)
		}
	}
}
