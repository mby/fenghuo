// Package rollup flushes the hub to the store once a minute and performs
// hourly aggregation and retention cleanup.
package rollup

import (
	"context"
	"log/slog"
	"time"

	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

const eventRetention = 30 * 24 * time.Hour

type Runner struct {
	Hub        *hub.Hub
	Store      *store.Store
	KeepMinute time.Duration
	KeepHour   time.Duration
	Log        *slog.Logger
}

func (r *Runner) Run(ctx context.Context) {
	var lastHour time.Time
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute + 2*time.Second)
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		now = time.Now()
		r.Flush(ctx, now, false)
		if h := now.Truncate(time.Hour); !h.Equal(lastHour) {
			lastHour = h
			r.maintain(ctx, now)
		}
	}
}

// Flush persists completed minutes; flushAll also writes the minute in progress
// and is used on shutdown.
func (r *Runner) Flush(ctx context.Context, now time.Time, flushAll bool) {
	b := r.Hub.Drain(now, flushAll)
	if b.Empty() {
		return
	}
	if err := r.Store.SaveBatch(ctx, b); err != nil {
		r.Log.Error("flush metrics", "err", err)
	}
}

func (r *Runner) maintain(ctx context.Context, now time.Time) {
	hour := now.Truncate(time.Hour).Unix()
	// Re-aggregate the last few hours so that short downtimes are covered.
	if err := r.Store.RollupHours(ctx, hour-3*3600, hour); err != nil {
		r.Log.Error("hourly rollup", "err", err)
	}
	err := r.Store.Cleanup(ctx,
		now.Add(-r.KeepMinute).Unix(),
		now.Add(-r.KeepHour).Unix(),
		now.Add(-eventRetention).Unix(),
		now.Unix())
	if err != nil {
		r.Log.Error("cleanup", "err", err)
	}
}
