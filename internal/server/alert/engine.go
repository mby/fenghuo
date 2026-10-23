// Package alert evaluates alert rules against the live hub state and sends
// notifications when an alert fires or resolves.
package alert

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/store"
)

const (
	KindOffline    = "offline"
	KindCPU        = "cpu"
	KindMem        = "mem"
	KindDisk       = "disk"
	KindTraffic    = "traffic"
	KindExpire     = "expire"
	KindLatency    = "latency"
	KindPacketLoss = "packet_loss"
)

var Kinds = []string{KindOffline, KindCPU, KindMem, KindDisk, KindTraffic, KindExpire, KindLatency, KindPacketLoss}

const evalEvery = 10 * time.Second

type Sender interface {
	Broadcast(notify.Message)
}

type Engine struct {
	hub    *hub.Hub
	st     *store.Store
	sender Sender
	log    *slog.Logger

	mu      sync.Mutex
	rules   []store.Rule
	firing  map[key]store.AlertState
	pending map[key]time.Time
}

type key struct{ rule, node int64 }

func New(h *hub.Hub, st *store.Store, sender Sender, log *slog.Logger) *Engine {
	return &Engine{hub: h, st: st, sender: sender, log: log, firing: map[key]store.AlertState{}, pending: map[key]time.Time{}}
}

// Reload re-reads rules and persisted alert state.
func (e *Engine) Reload(ctx context.Context) error {
	rules, err := e.st.ListRules(ctx)
	if err != nil {
		return err
	}
	states, err := e.st.ListAlertStates(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
	e.firing = map[key]store.AlertState{}
	for _, s := range states {
		e.firing[key{s.RuleID, s.NodeID}] = s
	}
	e.pending = map[key]time.Time{}
	return nil
}

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(evalEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			e.Evaluate(ctx, now)
		}
	}
}

func (e *Engine) Evaluate(ctx context.Context, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	views := e.hub.Snapshot(now)
	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}
		for _, v := range views {
			if len(r.NodeIDs) > 0 && !slices.Contains(r.NodeIDs, v.Node.ID) {
				continue
			}
			e.eval(ctx, r, v, now)
		}
	}
}

func (e *Engine) eval(ctx context.Context, r store.Rule, v hub.View, now time.Time) {
	k := key{r.ID, v.Node.ID}
	state, firing := e.firing[k]
	t := textsFor(e.st.Settings().Lang)
	name := v.Node.Name

	switch r.Kind {
	case KindOffline:
		everSeen := !v.LastSeen.IsZero() || v.Info.UpdatedAt > 0
		if !everSeen {
			return
		}
		if v.Online {
			if firing {
				e.resolve(ctx, r, v, now, fmt.Sprintf(t.online, name), true)
			}
			return
		}
		since := e.hub.StartedAt()
		if v.LastSeen.After(since) {
			since = v.LastSeen
		}
		offlineFor := now.Sub(since)
		if !firing && offlineFor >= time.Duration(r.Duration)*time.Second {
			e.fire(ctx, r, v, now, 0, fmt.Sprintf(t.offline, name, t.duration(offlineFor)))
		}

	case KindCPU, KindMem, KindDisk:
		if !v.Online || v.Status == nil {
			delete(e.pending, k)
			return
		}
		val := usage(r.Kind, v)
		if val <= r.Threshold {
			delete(e.pending, k)
			if firing {
				e.resolve(ctx, r, v, now, fmt.Sprintf(t.normal, name, t.metric[r.Kind], val), true)
			}
			return
		}
		since, ok := e.pending[k]
		if !ok {
			since = now
			e.pending[k] = now
		}
		held := now.Sub(since)
		if !firing && held >= time.Duration(r.Duration)*time.Second {
			e.fire(ctx, r, v, now, 0, fmt.Sprintf(t.over, name, t.metric[r.Kind], val, r.Threshold, t.duration(held)))
		}

	case KindLatency, KindPacketLoss:
		e.evalNetwork(ctx, r, v, now, firing)

	case KindTraffic:
		limit := v.Node.TrafficLimit
		period := v.Traffic.PeriodStart
		if firing && state.Ref != period {
			e.resolve(ctx, r, v, now, "", false) // new billing period
			firing = false
		}
		if limit <= 0 || firing {
			return
		}
		used := hub.TrafficUsed(v.Traffic, v.Node.TrafficMode)
		pct := float64(used) / float64(limit) * 100
		if pct >= r.Threshold {
			e.fire(ctx, r, v, now, period, fmt.Sprintf(t.traffic, name, formatBytes(used), formatBytes(uint64(limit)), pct))
		}

	case KindExpire:
		exp := v.Node.ExpireAt
		if firing && state.Ref != exp {
			e.resolve(ctx, r, v, now, "", false) // renewed or changed
			firing = false
		}
		if exp <= 0 || firing {
			return
		}
		left := time.Unix(exp, 0).Sub(now)
		days := int(math.Ceil(left.Hours() / 24))
		date := time.Unix(exp, 0).Format("2006-01-02")
		switch {
		case left <= 0:
			e.fire(ctx, r, v, now, exp, fmt.Sprintf(t.expired, name, date))
		case days <= int(r.Threshold):
			e.fire(ctx, r, v, now, exp, fmt.Sprintf(t.expiring, name, date, days))
		}
	}
}

func usage(kind string, v hub.View) float64 {
	s := v.Status
	switch kind {
	case KindCPU:
		return s.CPU
	case KindMem:
		if s.MemTotal > 0 {
			return float64(s.MemUsed) / float64(s.MemTotal) * 100
		}
	case KindDisk:
		if s.DiskTotal > 0 {
			return float64(s.DiskUsed) / float64(s.DiskTotal) * 100
		}
	}
	return 0
}

func (e *Engine) fire(ctx context.Context, r store.Rule, v hub.View, now time.Time, ref int64, text string) {
	k := key{r.ID, v.Node.ID}
	state := store.AlertState{RuleID: r.ID, NodeID: v.Node.ID, Ref: ref, FiredAt: now.Unix()}
	if err := e.transition(ctx, r, v, now, state, true, text); err != nil {
		e.log.Error("save alert and notification failed", "rule_id", r.ID, "node_id", v.Node.ID)
		return
	}
	e.firing[k] = state
}

func (e *Engine) resolve(ctx context.Context, r store.Rule, v hub.View, now time.Time, text string, notify bool) {
	k := key{r.ID, v.Node.ID}
	var err error
	if notify {
		err = e.transition(ctx, r, v, now, store.AlertState{RuleID: r.ID, NodeID: v.Node.ID}, false, text)
	} else {
		err = e.st.DeleteAlertState(ctx, r.ID, v.Node.ID)
	}
	if err != nil {
		e.log.Error("resolve alert and notification failed", "rule_id", r.ID, "node_id", v.Node.ID)
		return
	}
	delete(e.firing, k)
	delete(e.pending, k)
}

func (e *Engine) transition(ctx context.Context, r store.Rule, v hub.View, now time.Time, state store.AlertState, firing bool, text string) error {
	event := store.Event{
		RuleID: r.ID, NodeID: v.Node.ID, Kind: r.Kind, Firing: firing, Message: text, CreatedAt: now.Unix(),
	}
	settings := e.st.Settings()
	t := textsFor(settings.Lang)
	label := t.resolved
	if firing {
		label = t.firing
	}
	msg := notify.Message{
		Title:  fmt.Sprintf("[%s] %s: %s", settings.SiteName, label, r.Name),
		Text:   text,
		Node:   v.Node.Name,
		Kind:   r.Kind,
		Firing: firing,
		Time:   now.Unix(),
	}
	if sender, ok := e.sender.(interface {
		CommitAlert(context.Context, store.AlertState, store.Event, notify.Message) error
	}); ok {
		if err := sender.CommitAlert(ctx, state, event, msg); err != nil {
			return err
		}
	} else {
		// Compatibility for alternate senders; the built-in notifier uses the
		// transactional outbox above.
		var err error
		if firing {
			err = e.st.PutAlertState(ctx, state)
		} else {
			err = e.st.DeleteAlertState(ctx, r.ID, v.Node.ID)
		}
		if err != nil {
			return err
		}
		if err := e.st.AddEvent(ctx, event); err != nil {
			return err
		}
		e.sender.Broadcast(msg)
	}
	e.log.Info("alert", "rule", r.Name, "node", v.Node.Name, "firing", firing, "message", text)
	return nil
}
