package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	AvailabilityRetention     = 91 * 24 * time.Hour
	AvailabilityBatchSize     = 256
	AvailabilityTimelineLimit = 100
)

// AvailabilityInterval is a half-open interval in Unix seconds. Unknown time
// is the complement of persisted observations, never an assumed online span.
type AvailabilityInterval struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	State string `json:"state"`
}

type AvailabilityRow struct {
	NodeID int64
	UUID   string
	AvailabilityInterval
}

type AvailabilitySummary struct {
	From                 int64                  `json:"from"`
	To                   int64                  `json:"to"`
	OnlineSeconds        int64                  `json:"online_seconds"`
	OfflineSeconds       int64                  `json:"offline_seconds"`
	UnknownSeconds       int64                  `json:"unknown_seconds"`
	UptimePercent        *float64               `json:"uptime_percent"`
	CoveragePercent      float64                `json:"coverage_percent"`
	OutageCount          int                    `json:"outage_count"`
	LongestOutageSeconds int64                  `json:"longest_outage_seconds"`
	Timeline             []AvailabilityInterval `json:"timeline"`
	TimelineTruncated    bool                   `json:"timeline_truncated"`
}

// SaveAvailability merges adjacent observations. UUID guards against SQLite
// reusing a deleted node ID while a snapshot is being persisted.
func (s *Store) SaveAvailability(ctx context.Context, batch []AvailabilityRow) error {
	if len(batch) > AvailabilityBatchSize {
		return fmt.Errorf("availability batch exceeds %d", AvailabilityBatchSize)
	}
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin availability: %w", err)
	}
	defer tx.Rollback()
	for _, row := range batch {
		if row.End <= row.Start || (row.State != "online" && row.State != "offline") || row.UUID == "" {
			return fmt.Errorf("invalid availability interval")
		}
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE id = ? AND uuid = ?", row.NodeID, row.UUID).Scan(&exists); err != nil {
			return fmt.Errorf("availability node: %w", err)
		}
		if exists == 0 {
			continue
		}
		var last AvailabilityInterval
		err := tx.QueryRowContext(ctx, "SELECT start, end, state FROM availability_intervals WHERE node_id = ? ORDER BY start DESC LIMIT 1", row.NodeID).Scan(&last.Start, &last.End, &last.State)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("availability tail: %w", err)
		}
		if err == nil && last.End > row.Start {
			return fmt.Errorf("overlapping availability interval")
		}
		if err == nil && last.End == row.Start && last.State == row.State {
			_, err = tx.ExecContext(ctx, "UPDATE availability_intervals SET end = ? WHERE node_id = ? AND start = ?", row.End, row.NodeID, last.Start)
		} else {
			_, err = tx.ExecContext(ctx, "INSERT INTO availability_intervals(node_id, start, end, state) VALUES (?, ?, ?, ?)", row.NodeID, row.Start, row.End, row.State)
		}
		if err != nil {
			return fmt.Errorf("save availability: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit availability: %w", err)
	}
	return nil
}

// CleanupAvailability bounds each cleanup transaction, including after a long
// shutdown. Repeated ticks drain any expired backlog without blocking reports.
func (s *Store) CleanupAvailability(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM availability_intervals WHERE rowid IN
		(SELECT rowid FROM availability_intervals WHERE end <= ? ORDER BY end LIMIT 4096)`, now.Add(-AvailabilityRetention).Unix())
	if err != nil {
		return fmt.Errorf("cleanup availability: %w", err)
	}
	return nil
}

// QueryAvailability streams the full range for totals, retaining only the most
// recent 100 timeline spans. Outages are observed offline runs intersecting the
// window, clipped to it; unknown gaps split runs and do not imply recovery.
func (s *Store) QueryAvailability(ctx context.Context, nodeID, from, to int64) (AvailabilitySummary, error) {
	out := AvailabilitySummary{From: from, To: to, Timeline: []AvailabilityInterval{}}
	if to <= from || to-from > int64((90*24*time.Hour)/time.Second) {
		return out, fmt.Errorf("invalid availability window")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT start, end, state FROM availability_intervals
		WHERE node_id = ? AND start < ? AND end > ? ORDER BY start`, nodeID, to, from)
	if err != nil {
		return out, fmt.Errorf("query availability: %w", err)
	}
	defer rows.Close()
	appendSpan := func(span AvailabilityInterval) {
		if span.End <= span.Start {
			return
		}
		n := len(out.Timeline)
		if n > 0 && out.Timeline[n-1].State == span.State && out.Timeline[n-1].End == span.Start {
			out.Timeline[n-1].End = span.End
			return
		}
		if n == AvailabilityTimelineLimit {
			copy(out.Timeline, out.Timeline[1:])
			out.Timeline = out.Timeline[:n-1]
			out.TimelineTruncated = true
		}
		out.Timeline = append(out.Timeline, span)
	}
	pos, offlineEnd, offlineRun := from, int64(-1), int64(0)
	for rows.Next() {
		var span AvailabilityInterval
		if err := rows.Scan(&span.Start, &span.End, &span.State); err != nil {
			return out, fmt.Errorf("scan availability: %w", err)
		}
		span.Start, span.End = max(span.Start, from), min(span.End, to)
		appendSpan(AvailabilityInterval{Start: pos, End: span.Start, State: "unknown"})
		d := span.End - span.Start
		if span.State == "online" {
			out.OnlineSeconds += d
		} else {
			out.OfflineSeconds += d
			if offlineEnd != span.Start {
				out.OutageCount++
				offlineRun = 0
			}
			offlineRun += d
			offlineEnd = span.End
			out.LongestOutageSeconds = max(out.LongestOutageSeconds, offlineRun)
		}
		appendSpan(span)
		pos = span.End
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read availability: %w", err)
	}
	appendSpan(AvailabilityInterval{Start: pos, End: to, State: "unknown"})
	known := out.OnlineSeconds + out.OfflineSeconds
	out.UnknownSeconds = to - from - known
	out.CoveragePercent = 100 * float64(known) / float64(to-from)
	if known > 0 {
		p := 100 * float64(out.OnlineSeconds) / float64(known)
		out.UptimePercent = &p
	}
	return out, nil
}
