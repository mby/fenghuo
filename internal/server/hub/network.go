package hub

import (
	"sort"
	"strings"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

const (
	NetworkWindow     = 5 * time.Minute
	NetworkCardWindow = 12 * time.Minute
	NetworkFreshFor   = 2 * time.Minute
	maxProbeSamples   = 128
)

// NetworkStat describes only targets reported by the agent's local configuration.
// AvgMS excludes failed connections; -1 means no successful connection.
type NetworkStat struct {
	WindowEnd       int64
	Kind            string
	MedianMS        float64
	Error           string
	Errors          int
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

// NetworkBucket groups in-memory samples; alert and card windows use different resolutions.
type NetworkBucket struct {
	T         int64
	AvgMS     float64
	Samples   int
	Successes int
}

type probeSample struct {
	at  time.Time
	ms  float64
	err string
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
		for first < len(w.samples) && now.Sub(w.samples[first].at) > NetworkCardWindow+time.Minute {
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
	return n.networkStatsFor(now, NetworkWindow, 30)
}

func (n *node) cardNetworkStats(now time.Time) []NetworkStat {
	start := now.Truncate(time.Minute).Add(-NetworkCardWindow)
	return n.networkStatsFor(now, now.Sub(start), 60)
}

func (n *node) networkStatsFor(now time.Time, window time.Duration, bucketSeconds int64) []NetworkStat {
	out := make([]NetworkStat, 0, len(n.network))
	for target, w := range n.network {
		kind, name, ok := strings.Cut(target, "\x00")
		if !ok {
			kind, name = "tcp", target
		}
		s := NetworkStat{Kind: kind, Target: name, AvgMS: -1, MedianMS: -1, ContinuousSince: w.since, LastSeen: w.samples[len(w.samples)-1].at, Error: w.samples[len(w.samples)-1].err}
		s.WindowEnd = now.Truncate(time.Minute).Unix()
		var total float64
		var latencies []float64
		buckets := make(map[int64]*NetworkBucket)
		for _, p := range w.samples {
			if bucketSeconds == 60 && p.at.Unix() >= s.WindowEnd {
				continue
			}
			age := now.Sub(p.at)
			if age < 0 || age > window {
				continue
			}
			if p.err != "" {
				s.Errors++
				continue
			}
			s.Count++
			ts := p.at.Unix() / bucketSeconds * bucketSeconds
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
				latencies = append(latencies, p.ms)
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
			sort.Float64s(latencies)
			s.MedianMS = (latencies[(len(latencies)-1)/2] + latencies[len(latencies)/2]) / 2
		}
		if s.Count > 0 {
			s.FailureRate = 100 * float64(s.Count-s.SuccessCount) / float64(s.Count)
		}
		s.Fresh = s.Count+s.Errors > 0 && !now.Before(s.LastSeen) && now.Sub(s.LastSeen) <= NetworkFreshFor
		if bucketSeconds == 60 {
			s.Fresh = !now.Before(s.LastSeen) && now.Sub(s.LastSeen) <= NetworkFreshFor
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}
