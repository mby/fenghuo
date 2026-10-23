package hub

import (
	"math"

	"github.com/mby/fenghuo/internal/server/store"
)

type accum struct {
	minute int64
	n      uint64
	cpu    float64
	load   float64
	mem    uint64
	swap   uint64
	disk   uint64
	in     uint64
	out    uint64
	tcp    uint64
	udp    uint64
	procs  uint64
}

func (a *accum) add(m store.Metric) {
	a.n++
	a.cpu += m.CPU
	a.load += m.Load1
	a.mem += m.MemUsed
	a.swap += m.SwapUsed
	a.disk += m.DiskUsed
	a.in += m.NetIn
	a.out += m.NetOut
	a.tcp += uint64(m.TCP)
	a.udp += uint64(m.UDP)
	a.procs += uint64(m.Procs)
}

func (a *accum) avg() store.Metric {
	n := a.n
	return store.Metric{
		T:        a.minute,
		CPU:      round2(a.cpu / float64(n)),
		Load1:    round2(a.load / float64(n)),
		MemUsed:  a.mem / n,
		SwapUsed: a.swap / n,
		DiskUsed: a.disk / n,
		NetIn:    a.in / n,
		NetOut:   a.out / n,
		TCP:      uint32(a.tcp / n),
		UDP:      uint32(a.udp / n),
		Procs:    uint32(a.procs / n),
	}
}

type pingAcc struct {
	minute int64
	total  int
	ok     int
	sum    float64
}

func (a *pingAcc) row(nodeID int64, name string) store.PingRow {
	r := store.PingRow{NodeID: nodeID, Target: name, TS: a.minute, AvgMS: -1}
	if a.ok > 0 {
		r.AvgMS = round2(a.sum / float64(a.ok))
	}
	if a.total > 0 {
		r.Loss = round2(float64(a.total-a.ok) / float64(a.total) * 100)
	}
	return r
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
