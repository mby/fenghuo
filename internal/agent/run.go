package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mby/fenghuo/internal/agent/collect"
	"github.com/mby/fenghuo/internal/version"
)

const infoEvery = 10 * time.Minute

func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	client, err := NewClient(cfg.Server, cfg.Token, cfg.AllowHTTP)
	if err != nil {
		return err
	}
	col, err := collect.New(cfg.NICs)
	if err != nil {
		return err
	}

	var p *pinger
	if len(cfg.Ping) > 0 {
		p = &pinger{targets: cfg.Ping}
		go p.run(ctx, cfg.PingInterval)
	}

	log.Info("agent started", "version", version.Version, "server", cfg.Server,
		"interval", cfg.Interval, "idle_interval", cfg.IdleInterval, "ping_targets", len(cfg.Ping))

	// CPU usage and network speed are computed from counter deltas, so a
	// longer idle interval still yields the average over that interval.
	wait := cfg.Interval
	timer := time.NewTimer(wait)
	defer timer.Stop()

	var lastInfo, lastErrLog time.Time
	failing := false
	for {
		select {
		case <-ctx.Done():
			log.Info("agent stopped")
			return nil
		case <-timer.C:
		}

		now := time.Now()
		rep, err := col.Collect(now)
		if err != nil {
			log.Warn("collect failed", "err", err)
			timer.Reset(wait)
			continue
		}
		rep.Interval = cfg.Interval.Milliseconds()
		rep.IdleInterval = cfg.IdleInterval.Milliseconds()
		if now.Sub(lastInfo) >= infoEvery {
			info := col.Info()
			rep.Info = &info
		}
		if p != nil {
			rep.Ping = p.take()
		}

		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		watched, err := client.Send(sendCtx, &rep)
		cancel()
		if err != nil {
			if ctx.Err() == nil && (!failing || now.Sub(lastErrLog) >= time.Minute) {
				msg := "report failed"
				if errors.Is(err, ErrUnauthorized) {
					msg = "report rejected: check FENGHUO_TOKEN"
				}
				log.Warn(msg, "err", err)
				lastErrLog = now
			}
			failing = true
			timer.Reset(wait)
			continue
		}
		if rep.Info != nil {
			lastInfo = now
		}
		if failing {
			log.Info("report recovered")
			failing = false
		}

		next := cfg.Interval
		if !watched && cfg.IdleInterval > 0 {
			next = cfg.IdleInterval
		}
		if next != wait {
			log.Debug("report interval changed", "interval", next, "watched", watched)
			wait = next
		}
		timer.Reset(wait)
	}
}
