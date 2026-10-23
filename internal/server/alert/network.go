package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func (e *Engine) evalNetwork(ctx context.Context, r store.Rule, v hub.View, now time.Time, firing bool) {
	k := key{r.ID, v.Node.ID}
	if !v.Online || v.Status == nil {
		delete(e.pending, k)
		return
	}
	s, value, known := networkValue(r, v.Network)
	if !known {
		// Missing data breaks the duration but cannot resolve a firing alert.
		delete(e.pending, k)
		return
	}
	if value < r.Threshold {
		delete(e.pending, k)
		if firing {
			e.resolve(ctx, r, v, now, networkMessage(e.st.Settings().Lang, r, v.Node.Name, s.Target, value, 0, false), true)
		}
		return
	}
	since, ok := e.pending[k]
	if !ok || s.ContinuousSince.After(since) {
		since = now
		e.pending[k] = now
	}
	held := now.Sub(since)
	if !firing && held >= time.Duration(r.Duration)*time.Second {
		e.fire(ctx, r, v, now, 0, networkMessage(e.st.Settings().Lang, r, v.Node.Name, s.Target, value, held, true))
	}
}

// A healthy target must not mask a stale or all-failed target. A known breach
// may fire even when another target is unknown; recovery requires complete data.
func networkValue(r store.Rule, stats []hub.NetworkStat) (hub.NetworkStat, float64, bool) {
	var worst hub.NetworkStat
	value := -1.0
	unknown := false
	for _, s := range stats {
		if r.Target != "" && s.Target != r.Target {
			continue
		}
		if !s.Fresh || s.Count == 0 || (r.Kind == KindLatency && (!s.LatencyFresh || s.SuccessCount == 0)) {
			unknown = true
			continue
		}
		v := s.AvgMS
		if r.Kind == KindPacketLoss {
			v = s.FailureRate
		}
		if v > value {
			worst, value = s, v
		}
	}
	return worst, value, value >= 0 && (!unknown || value >= r.Threshold)
}

func networkMessage(lang string, r store.Rule, node, target string, value float64, held time.Duration, firing bool) string {
	metric, unit := "TCP latency", "ms"
	if r.Kind == KindPacketLoss {
		metric, unit = "TCP connection failure rate", "%"
	}
	if lang == "zh" {
		metric = "TCP 连接延迟"
		if r.Kind == KindPacketLoss {
			metric = "TCP 连接失败率"
		}
		if firing {
			return fmt.Sprintf("节点 %s，目标 %s：%s %.1f%s 达到阈值 %.1f%s（持续 %s）", node, target, metric, value, unit, r.Threshold, unit, zhDuration(held))
		}
		return fmt.Sprintf("节点 %s，目标 %s：%s 已恢复正常（%.1f%s）", node, target, metric, value, unit)
	}
	if firing {
		return fmt.Sprintf("Node %s, target %s: %s %.1f%s meets or exceeds %.1f%s (for %s)", node, target, metric, value, unit, r.Threshold, unit, held.Round(time.Second))
	}
	return fmt.Sprintf("Node %s, target %s: %s is back to normal (%.1f%s)", node, target, metric, value, unit)
}
