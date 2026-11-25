package agent

import (
	"context"
	"math"
	"net"
	"time"
)

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
