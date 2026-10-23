// Package hub keeps the live state of every node in memory: the latest
// report, a short ring buffer for live charts and per-minute accumulators
// that are periodically drained to the store.
package hub

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/store"
)

const (
	ringSize = 200 // ~10 minutes at the default 3s interval
	// minGap leaves room for network jitter at the shortest 1s agent interval.
	minGap = 500 * time.Millisecond
)

var (
	ErrUnknownToken = errors.New("unknown token")
	ErrTooFrequent  = errors.New("reporting too frequently")
)

// viewerWindow is how long one dashboard request keeps agents in the normal
// interval. Browsers poll every 3s while the tab is visible.
const viewerWindow = 15 * time.Second

type Hub struct {
	mu         sync.RWMutex
	nodes      map[int64]*node
	byToken    map[string]*node
	startedAt  time.Time
	lastViewed atomic.Int64 // unix nanoseconds
}

type node struct {
	mu sync.Mutex

	cfg       store.Node
	info      store.NodeInfo
	infoDirty bool

	last     *proto.Report
	lastSeen time.Time
	gap      time.Duration
	expect   time.Duration // when the next report is due, as told by the agent

	ring     [ringSize]store.Metric
	ringHead int
	ringLen  int

	acc     accum
	pending []store.Metric

	pings       map[string]*pingAcc
	pingPending []store.PingRow
	network     map[string]*probeWindow

	traffic      store.Traffic
	trafficDirty bool
}

func New(now time.Time) *Hub {
	return &Hub{nodes: map[int64]*node{}, byToken: map[string]*node{}, startedAt: now}
}

func (h *Hub) StartedAt() time.Time { return h.startedAt }

// MarkViewed records that someone is looking at live data.
func (h *Hub) MarkViewed(now time.Time) { h.lastViewed.Store(now.UnixNano()) }

// Watched reports whether someone looked at live data recently.
func (h *Hub) Watched(now time.Time) bool {
	return now.UnixNano()-h.lastViewed.Load() < int64(viewerWindow)
}

// Reload replaces node configuration while keeping live state. traffic is
// only used for nodes that are not yet known (i.e. on startup).
func (h *Hub) Reload(nodes []store.Node, traffic map[int64]store.Traffic, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := make(map[int64]*node, len(nodes))
	byToken := make(map[string]*node, len(nodes))
	for _, cfg := range nodes {
		n := h.nodes[cfg.ID]
		if n == nil {
			n = &node{info: cfg.Info, traffic: traffic[cfg.ID], pings: map[string]*pingAcc{}}
			n.cfg = cfg
		}
		n.mu.Lock()
		if n.cfg.TrafficResetDay != cfg.TrafficResetDay {
			// Changing the reset day moves the period boundary without discarding usage.
			n.traffic.PeriodStart = PeriodStart(now, cfg.TrafficResetDay)
			n.trafficDirty = true
		}
		n.cfg = cfg
		n.ensurePeriod(now)
		n.mu.Unlock()
		next[cfg.ID] = n
		byToken[cfg.TokenHash] = n
	}
	h.nodes, h.byToken = next, byToken
}

// Ingest stores a report. watched tells the caller which status to answer
// with, which in turn decides the agent's next interval.
func (h *Hub) Ingest(tokenHash string, r *proto.Report, ip string, now time.Time) (watched bool, err error) {
	h.mu.RLock()
	n := h.byToken[tokenHash]
	h.mu.RUnlock()
	if n == nil {
		return false, ErrUnknownToken
	}
	watched = h.Watched(now)

	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.lastSeen.IsZero() {
		gap := now.Sub(n.lastSeen)
		if gap < minGap {
			return watched, ErrTooFrequent
		}
		if gap > offlineAfter(n.expect, n.gap) || (n.last != nil && n.last.BootTime != r.BootTime) {
			n.network = nil
		}
		n.gap = gap
	}
	n.lastSeen = now
	n.last = r
	n.expect = time.Duration(r.Interval) * time.Millisecond
	if !watched && r.IdleInterval > 0 {
		n.expect = time.Duration(r.IdleInterval) * time.Millisecond
	}

	if n.info.LastIP != ip {
		n.info.LastIP = ip
		n.infoDirty = true
	}
	if n.info.MemTotal != r.MemTotal || n.info.DiskTotal != r.DiskTotal {
		n.info.MemTotal, n.info.DiskTotal = r.MemTotal, r.DiskTotal
		n.infoDirty = true
	}
	if r.Info != nil {
		i := r.Info
		n.info.OS, n.info.Kernel, n.info.Arch, n.info.CPUModel = i.OS, i.Kernel, i.Arch, i.CPUModel
		n.info.CPUCores, n.info.Virt, n.info.AgentVersion = i.CPUCores, i.Virt, i.AgentVersion
		n.info.UpdatedAt = now.Unix()
		n.infoDirty = true
	}

	n.ensurePeriod(now)
	applyCounters(&n.traffic, r.BootTime, r.NetRx, r.NetTx)
	n.trafficDirty = true

	m := store.Metric{
		T: now.Unix(), CPU: r.CPU, Load1: r.Load1,
		MemUsed: r.MemUsed, SwapUsed: r.SwapUsed, DiskUsed: r.DiskUsed,
		NetIn: r.NetIn, NetOut: r.NetOut, TCP: r.TCP, UDP: r.UDP, Procs: r.Procs,
	}
	n.pushRing(m)
	n.addMinute(m)
	n.addPings(r.Ping, now)
	n.addNetwork(r.Ping, now)
	return watched, nil
}

func (n *node) ensurePeriod(now time.Time) {
	start := PeriodStart(now, n.cfg.TrafficResetDay)
	if n.traffic.PeriodStart >= start {
		return
	}
	if n.traffic.PeriodStart != 0 {
		n.traffic.Up, n.traffic.Down = 0, 0
	}
	n.traffic.PeriodStart = start
	n.trafficDirty = true
}

func (n *node) pushRing(m store.Metric) {
	n.ring[(n.ringHead+n.ringLen)%ringSize] = m
	if n.ringLen < ringSize {
		n.ringLen++
	} else {
		n.ringHead = (n.ringHead + 1) % ringSize
	}
}

func (n *node) addMinute(m store.Metric) {
	minute := m.T / 60 * 60
	if n.acc.n > 0 && n.acc.minute != minute {
		n.pending = append(n.pending, n.acc.avg())
		n.acc = accum{}
	}
	n.acc.minute = minute
	n.acc.add(m)
}

func (n *node) addPings(pings []proto.Ping, now time.Time) {
	minute := now.Unix() / 60 * 60
	for _, p := range pings {
		a := n.pings[p.Name]
		if a != nil && a.minute != minute {
			n.pingPending = append(n.pingPending, a.row(n.cfg.ID, p.Name))
			a = nil
		}
		if a == nil {
			a = &pingAcc{minute: minute}
			n.pings[p.Name] = a
		}
		a.total++
		if p.MS >= 0 {
			a.ok++
			a.sum += p.MS
		}
	}
}

// View is a consistent copy of one node's state.
type View struct {
	Node           store.Node
	Info           store.NodeInfo
	Online         bool
	LastSeen       time.Time
	Status         *proto.Report // nil if the node never reported since server start
	Traffic        store.Traffic
	NextResetAt    int64
	ResetUTCOffset int
	Network        []NetworkStat // internal only; public responses use explicit DTOs
}

// offlineAfter is how long a node may stay silent before it is offline: three
// missed reports at the interval the agent announced, or, for agents that do
// not announce one, at the interval observed between reports.
func offlineAfter(expect, gap time.Duration) time.Duration {
	if expect > 0 {
		return max(3*expect, 10*time.Second)
	}
	if gap <= 0 {
		return 30 * time.Second
	}
	return min(max(3*gap, 10*time.Second), 3*time.Minute)
}

// restartGrace is how long after a server start a previously known node is
// given to report before it counts as offline: three default idle intervals.
const restartGrace = 90 * time.Second

func (n *node) view(now, startedAt time.Time) View {
	n.ensurePeriod(now)
	next := NextPeriodStart(now, n.cfg.TrafficResetDay)
	_, offset := next.Zone()
	v := View{Node: n.cfg, Info: n.info, LastSeen: n.lastSeen, Status: n.last, Traffic: n.traffic, Network: n.networkStats(now)}
	v.NextResetAt, v.ResetUTCOffset = next.Unix(), offset
	if n.lastSeen.IsZero() {
		v.Online = n.info.UpdatedAt > 0 && now.Sub(startedAt) < restartGrace
	} else {
		v.Online = now.Sub(n.lastSeen) <= offlineAfter(n.expect, n.gap)
	}
	return v
}

func (h *Hub) Snapshot(now time.Time) []View {
	h.mu.RLock()
	out := make([]View, 0, len(h.nodes))
	for _, n := range h.nodes {
		n.mu.Lock()
		out = append(out, n.view(now, h.startedAt))
		n.mu.Unlock()
	}
	h.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node.Sort != out[j].Node.Sort {
			return out[i].Node.Sort < out[j].Node.Sort
		}
		return out[i].Node.ID < out[j].Node.ID
	})
	return out
}

func (h *Hub) ByUUID(uuid string, now time.Time) (View, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, n := range h.nodes {
		n.mu.Lock()
		if n.cfg.UUID == uuid {
			v := n.view(now, h.startedAt)
			n.mu.Unlock()
			return v, true
		}
		n.mu.Unlock()
	}
	return View{}, false
}

// Live returns the ring buffer contents, oldest first.
func (h *Hub) Live(id int64) []store.Metric {
	h.mu.RLock()
	n := h.nodes[id]
	h.mu.RUnlock()
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]store.Metric, n.ringLen)
	for i := range n.ringLen {
		out[i] = n.ring[(n.ringHead+i)%ringSize]
	}
	return out
}

// Drain collects everything that must be persisted. Minutes that are still
// in progress at now are kept unless flushAll is set.
func (h *Hub) Drain(now time.Time, flushAll bool) store.Batch {
	var b store.Batch
	current := now.Unix() / 60 * 60
	h.mu.RLock()
	defer h.mu.RUnlock()
	for id, n := range h.nodes {
		n.mu.Lock()
		if n.acc.n > 0 && (flushAll || n.acc.minute < current) {
			n.pending = append(n.pending, n.acc.avg())
			n.acc = accum{}
		}
		for _, m := range n.pending {
			b.Metrics = append(b.Metrics, store.MetricRow{NodeID: id, Metric: m})
		}
		n.pending = nil

		for name, a := range n.pings {
			if flushAll || a.minute < current {
				n.pingPending = append(n.pingPending, a.row(id, name))
				delete(n.pings, name)
			}
		}
		b.Pings = append(b.Pings, n.pingPending...)
		n.pingPending = nil

		if n.infoDirty {
			b.Infos = append(b.Infos, store.InfoRow{NodeID: id, NodeInfo: n.info})
			n.infoDirty = false
		}
		n.ensurePeriod(now)
		if n.trafficDirty {
			b.Traffic = append(b.Traffic, store.TrafficRow{NodeID: id, Traffic: n.traffic})
			n.trafficDirty = false
		}
		n.mu.Unlock()
	}
	return b
}
