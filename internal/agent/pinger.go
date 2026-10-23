package agent

import (
	"context"
	"math"
	"net"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

// pinger measures TCP connect latency to locally configured targets.
// Targets come only from the agent's own configuration.
type pinger struct {
	targets []Target

	mu     sync.Mutex
	latest []proto.Ping
	fresh  bool
}

func (p *pinger) run(ctx context.Context, every time.Duration) {
	p.round(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.round(ctx)
		}
	}
}

func (p *pinger) round(ctx context.Context) {
	res := make([]proto.Ping, len(p.targets))
	var wg sync.WaitGroup
	for i, t := range p.targets {
		wg.Go(func() {
			res[i] = proto.Ping{Name: t.Name, MS: tcpLatency(ctx, t.Addr)}
		})
	}
	wg.Wait()
	p.mu.Lock()
	p.latest, p.fresh = res, true
	p.mu.Unlock()
}

// take returns the latest round once; subsequent calls return nil until a new round completes.
func (p *pinger) take() []proto.Ping {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.fresh {
		return nil
	}
	p.fresh = false
	return p.latest
}

func tcpLatency(ctx context.Context, addr string) float64 {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return -1
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return -1
	}
	ip := ips[0].IP
	for _, a := range ips {
		if a.IP.To4() != nil {
			ip = a.IP
			break
		}
	}
	// DNS resolution is excluded from the measurement.
	var d net.Dialer
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
	if err != nil {
		return -1
	}
	elapsed := time.Since(start)
	conn.Close()
	return math.Round(float64(elapsed.Microseconds())/10) / 100
}
