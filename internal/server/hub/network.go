package hub

import (
	"sort"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

const (
	NetworkWindow   = 5 * time.Minute
	NetworkFreshFor = 2 * time.Minute
	maxProbeSamples = 128
)

// NetworkStat describes only targets reported by the agent's local configuration.
// AvgMS excludes failed connections; -1 means no successful connection.
type NetworkStat struct {
	Target          string
	AvgMS           float64
	FailureRate     float64
	Count           int
	SuccessCount    int
	LastSeen        time.Time
	ContinuousSince time.Time
	Fresh           bool
	LatencyFresh    bool
	History         []NetworkBucket
}

// NetworkBucket is a 30-second view of existing in-memory samples, never a persisted record.
type NetworkBucket struct {
	T         int64
	AvgMS     float64
	Samples   int
	Successes int
}

type probeSample struct {
	at time.Time
	ms float64
}

type probeWindow struct {
	samples []probeSample
	since   time.Time
}

func (n *node) addNetwork(pings []proto.Ping, now time.Time) {
	if len(pings) == 0 {
		return // Most reports carry no new probe round.
	}
	next := make(map[string]*probeWindow, min(len(pings), proto.MaxPingTargets))
	for _, p := range pings {
		if _, duplicate := next[p.Name]; duplicate {
			continue
		}
		if len(next) == proto.MaxPingTargets {
			break
		}
		w := n.network[p.Name]
		if w == nil || now.Sub(w.samples[len(w.samples)-1].at) > NetworkFreshFor {
			w = &probeWindow{since: now}
		}
		first := 0
		for first < len(w.samples) && now.Sub(w.samples[first].at) > NetworkWindow {
			first++
		}
		copy(w.samples, w.samples[first:])
		w.samples = w.samples[:len(w.samples)-first]
		if len(w.samples) == maxProbeSamples {
			copy(w.samples, w.samples[1:])
			w.samples = w.samples[:maxProbeSamples-1]
		}
		w.samples = append(w.samples, probeSample{at: now, ms: p.MS})
		next[p.Name] = w
	}
	// A nonempty round contains the agent's current targets, not server configuration.
	n.network = next
}

func (n *node) networkStats(now time.Time) []NetworkStat {
	out := make([]NetworkStat, 0, len(n.network))
	for target, w := range n.network {
		s := NetworkStat{Target: target, AvgMS: -1, ContinuousSince: w.since, LastSeen: w.samples[len(w.samples)-1].at}
		var total float64
		buckets := make(map[int64]*NetworkBucket)
		for _, p := range w.samples {
			age := now.Sub(p.at)
			if age < 0 || age > NetworkWindow {
				continue
			}
			s.Count++
			ts := p.at.Unix() / 30 * 30
			b := buckets[ts]
			if b == nil {
				b = &NetworkBucket{T: ts}
				buckets[ts] = b
			}
			b.Samples++
			if p.ms >= 0 {
				b.Successes++
				b.AvgMS += p.ms
				s.SuccessCount++
				total += p.ms
				if age <= NetworkFreshFor {
					s.LatencyFresh = true
				}
			}
		}
		for _, b := range buckets {
			if b.Successes > 0 {
				b.AvgMS /= float64(b.Successes)
			} else {
				b.AvgMS = -1
			}
			s.History = append(s.History, *b)
		}
		sort.Slice(s.History, func(i, j int) bool { return s.History[i].T < s.History[j].T })
		if s.SuccessCount > 0 {
			s.AvgMS = total / float64(s.SuccessCount)
		}
		if s.Count > 0 {
			s.FailureRate = 100 * float64(s.Count-s.SuccessCount) / float64(s.Count)
		}
		s.Fresh = s.Count > 0 && !now.Before(s.LastSeen) && now.Sub(s.LastSeen) <= NetworkFreshFor
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}
