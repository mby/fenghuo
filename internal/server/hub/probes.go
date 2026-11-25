package hub

import (
	"github.com/mby/fenghuo/internal/proto"
	"time"
)

type probeMinute struct {
	kind, name string
	minute     int64
}

func (n *node) addProbes(r *proto.Report, now time.Time) {
	if len(r.Probes) == 0 {
		return
	}
	if n.probeSessions == nil {
		n.probeSessions = map[string]uint64{}
		n.network = nil
	}
	if _, ok := n.probeSessions[r.ProbeSession]; !ok && len(n.probeSessions) >= 4 {
		for key := range n.probeSessions {
			delete(n.probeSessions, key)
			break
		}
	}
	if n.network == nil {
		n.network = map[string]*probeWindow{}
	}
	if n.probeMinutes == nil {
		n.probeMinutes = map[probeMinute]*pingAcc{}
	}
	for key, w := range n.network {
		if len(w.samples) == 0 || now.Sub(w.samples[len(w.samples)-1].at) > NetworkCardWindow+time.Minute {
			delete(n.network, key)
		}
	}
	for _, p := range r.Probes {
		if p.Seq <= n.probeSessions[r.ProbeSession] {
			continue
		}
		n.probeSessions[r.ProbeSession] = p.Seq
		at := now.Add(time.Duration(p.Time-r.Time) * time.Millisecond)
		if now.Sub(at) > NetworkWindow || at.After(now.Add(time.Second)) {
			continue
		}
		key := p.Kind + "\x00" + p.Name
		w := n.network[key]
		if w == nil {
			if len(n.network) >= 32 {
				continue
			}
			w = &probeWindow{since: at}
			n.network[key] = w
		}
		first := 0
		for first < len(w.samples) && now.Sub(w.samples[first].at) > NetworkCardWindow+time.Minute {
			first++
		}
		w.samples = append(w.samples[:0], w.samples[first:]...)
		if len(w.samples) >= 1024 {
			w.samples = append(w.samples[:0], w.samples[1:]...)
		}
		w.samples = append(w.samples, probeSample{at: at, ms: p.MS, err: p.Error})
		if p.Error != "" {
			continue
		}
		minute := probeMinute{kind: p.Kind, name: p.Name, minute: at.Unix() / 60 * 60}
		a := n.probeMinutes[minute]
		if a == nil {
			a = &pingAcc{minute: minute.minute}
			n.probeMinutes[minute] = a
		}
		a.total++
		if p.MS >= 0 {
			a.ok++
			a.sum += p.MS
		}
	}
}
