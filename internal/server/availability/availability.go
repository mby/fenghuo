// Package availability observes the hub outside the report request path.
package availability

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

const (
	ObservationInterval = time.Minute
	MaxObservationGap   = 90 * time.Second
)

type observation struct {
	at    time.Time
	uuid  string
	state string
}

// Runner must have a single owner. A fresh Run never resumes a persisted open
// interval: time since the previous process's final observation stays unknown.
type Runner struct {
	Hub      *hub.Hub
	Store    *store.Store
	Log      *slog.Logger
	mu       sync.Mutex
	previous map[int64]observation
}

func (o observation) continues(uuid, state string, now time.Time) bool {
	gap := now.Sub(o.at)
	return o.uuid == uuid && state != "unknown" && o.state == state && gap > 0 && gap <= MaxObservationGap && now.Unix() > o.at.Unix()
}

func stateOf(v hub.View) string {
	// No report in this process means no observation, including the hub's
	// synthetic online restart grace. Never infer history from node_info.
	if v.LastSeen.IsZero() || v.Status == nil {
		return "unknown"
	}
	if v.Online {
		return "online"
	}
	return "offline"
}

// Tail is the not yet persisted span that an observation at now would save.
func (r *Runner) Tail(v hub.View, now time.Time) (store.AvailabilityInterval, bool) {
	r.mu.Lock()
	prev, ok := r.previous[v.Node.ID]
	r.mu.Unlock()
	state := stateOf(v)
	if !ok || !prev.continues(v.Node.UUID, state, now) {
		return store.AvailabilityInterval{}, false
	}
	return store.AvailabilityInterval{Start: prev.at.Unix(), End: now.Unix(), State: state}, true
}

func (r *Runner) Run(ctx context.Context) {
	r.mu.Lock()
	r.previous = nil
	r.mu.Unlock()
	r.observe(ctx, time.Now())
	ticker := time.NewTicker(ObservationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.observe(ctx, time.Now())
		}
	}
}

func (r *Runner) observe(ctx context.Context, now time.Time) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	batch := r.intervals(now, r.Hub.Snapshot(now))
	for len(batch) > 0 {
		n := min(len(batch), store.AvailabilityBatchSize)
		if err := r.Store.SaveAvailability(ctx, batch[:n]); err != nil {
			// Failed writes are not backfilled from later samples.
			log.Error("observe availability", "err", err)
			break
		}
		batch = batch[n:]
	}
	if err := r.Store.CleanupAvailability(ctx, now); err != nil {
		log.Error("cleanup availability", "err", err)
	}
}

func (r *Runner) intervals(now time.Time, views []hub.View) []store.AvailabilityRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[int64]observation, len(views))
	batch := make([]store.AvailabilityRow, 0, len(views))
	for _, v := range views {
		state := stateOf(v)
		prev, ok := r.previous[v.Node.ID]
		if ok && prev.continues(v.Node.UUID, state, now) {
			batch = append(batch, store.AvailabilityRow{NodeID: v.Node.ID, UUID: v.Node.UUID, AvailabilityInterval: store.AvailabilityInterval{Start: prev.at.Unix(), End: now.Unix(), State: state}})
		}
		next[v.Node.ID] = observation{at: now, uuid: v.Node.UUID, state: state}
	}
	r.previous = next
	return batch
}
