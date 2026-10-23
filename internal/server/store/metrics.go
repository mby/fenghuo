package store

import (
	"context"
	"fmt"
)

// Metric is one aggregated sample. The same shape is used for the in-memory
// live buffer and for stored history, and is serialised to the frontend as-is.
type Metric struct {
	T        int64   `json:"t"`
	CPU      float64 `json:"cpu"`
	Load1    float64 `json:"load1"`
	MemUsed  uint64  `json:"mem_used"`
	SwapUsed uint64  `json:"swap_used"`
	DiskUsed uint64  `json:"disk_used"`
	NetIn    uint64  `json:"net_in"`
	NetOut   uint64  `json:"net_out"`
	TCP      uint32  `json:"tcp"`
	UDP      uint32  `json:"udp"`
	Procs    uint32  `json:"procs"`
}

type MetricRow struct {
	NodeID int64
	Metric
}

type PingRow struct {
	NodeID int64
	Target string
	TS     int64
	AvgMS  float64 // -1 when every attempt in the minute failed
	Loss   float64 // percent
}

type InfoRow struct {
	NodeID int64
	NodeInfo
}

type TrafficRow struct {
	NodeID int64
	Traffic
}

// Batch is everything the hub accumulated since the previous flush.
type Batch struct {
	Metrics []MetricRow
	Pings   []PingRow
	Infos   []InfoRow
	Traffic []TrafficRow
}

func (b Batch) Empty() bool {
	return len(b.Metrics) == 0 && len(b.Pings) == 0 && len(b.Infos) == 0 && len(b.Traffic) == 0
}

// SaveBatch writes a batch in one transaction. Rows for nodes deleted in the
// meantime are skipped instead of failing the whole batch.
func (s *Store) SaveBatch(ctx context.Context, b Batch) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const exists = "EXISTS (SELECT 1 FROM nodes WHERE id = ?)"
	for _, m := range b.Metrics {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO metrics_1m
			(node_id, ts, cpu, load1, mem_used, swap_used, disk_used, net_in, net_out, tcp, udp, procs)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE `+exists,
			m.NodeID, m.T, m.CPU, m.Load1, m.MemUsed, m.SwapUsed, m.DiskUsed, m.NetIn, m.NetOut,
			m.TCP, m.UDP, m.Procs, m.NodeID); err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
	}
	for _, p := range b.Pings {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO ping_1m (node_id, target, ts, avg_ms, loss)
			SELECT ?, ?, ?, ?, ? WHERE `+exists,
			p.NodeID, p.Target, p.TS, p.AvgMS, p.Loss, p.NodeID); err != nil {
			return fmt.Errorf("ping: %w", err)
		}
	}
	for _, i := range b.Infos {
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_info
			(node_id, os, kernel, arch, cpu_model, cpu_cores, virt, agent_version, last_ip, updated_at, mem_total, disk_total)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE `+exists+`
			ON CONFLICT(node_id) DO UPDATE SET os = excluded.os, kernel = excluded.kernel, arch = excluded.arch,
			cpu_model = excluded.cpu_model, cpu_cores = excluded.cpu_cores, virt = excluded.virt,
			agent_version = excluded.agent_version, last_ip = excluded.last_ip, updated_at = excluded.updated_at,
			mem_total = excluded.mem_total, disk_total = excluded.disk_total`,
			i.NodeID, i.OS, i.Kernel, i.Arch, i.CPUModel, i.CPUCores, i.Virt, i.AgentVersion, i.LastIP, i.UpdatedAt,
			i.MemTotal, i.DiskTotal, i.NodeID); err != nil {
			return fmt.Errorf("node info: %w", err)
		}
	}
	for _, t := range b.Traffic {
		if _, err := tx.ExecContext(ctx, `INSERT INTO traffic
			(node_id, period_start, up, down, boot_time, last_rx, last_tx)
			SELECT ?, ?, ?, ?, ?, ?, ? WHERE `+exists+`
			ON CONFLICT(node_id) DO UPDATE SET period_start = excluded.period_start, up = excluded.up,
			down = excluded.down, boot_time = excluded.boot_time, last_rx = excluded.last_rx, last_tx = excluded.last_tx`,
			t.NodeID, t.PeriodStart, t.Up, t.Down, t.BootTime, t.LastRx, t.LastTx, t.NodeID); err != nil {
			return fmt.Errorf("traffic: %w", err)
		}
	}
	return tx.Commit()
}

type Resolution string

const (
	Minute Resolution = "metrics_1m"
	Hour   Resolution = "metrics_1h"
)

// QueryMetrics returns averaged metrics in buckets of step seconds.
func (s *Store) QueryMetrics(ctx context.Context, nodeID int64, res Resolution, from, step int64) ([]Metric, error) {
	if res != Minute && res != Hour {
		return nil, fmt.Errorf("invalid resolution %q", res)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT (ts / ?1) * ?1 AS b, AVG(cpu), AVG(load1),
		CAST(AVG(mem_used) AS INTEGER), CAST(AVG(swap_used) AS INTEGER), CAST(AVG(disk_used) AS INTEGER),
		CAST(AVG(net_in) AS INTEGER), CAST(AVG(net_out) AS INTEGER),
		CAST(AVG(tcp) AS INTEGER), CAST(AVG(udp) AS INTEGER), CAST(AVG(procs) AS INTEGER)
		FROM `+string(res)+` WHERE node_id = ?2 AND ts >= ?3 GROUP BY b ORDER BY b`, step, nodeID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Metric{}
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.T, &m.CPU, &m.Load1, &m.MemUsed, &m.SwapUsed, &m.DiskUsed,
			&m.NetIn, &m.NetOut, &m.TCP, &m.UDP, &m.Procs); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type PingPoint struct {
	T    int64   `json:"t"`
	MS   float64 `json:"ms"` // -1 when no attempt succeeded
	Loss float64 `json:"loss"`
}

type PingSeries struct {
	Name   string      `json:"name"`
	Points []PingPoint `json:"points"`
}

func (s *Store) QueryPing(ctx context.Context, nodeID, from, step int64) ([]PingSeries, error) {
	return s.queryPing(ctx, nodeID, from, step, "ping_1m")
}

func (s *Store) QueryPingHours(ctx context.Context, nodeID, from, step int64) ([]PingSeries, error) {
	return s.queryPing(ctx, nodeID, from, step, "ping_1h")
}

// table is chosen only by the two fixed callers above.
func (s *Store) queryPing(ctx context.Context, nodeID, from, step int64, table string) ([]PingSeries, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT target, (ts / ?1) * ?1 AS b,
		COALESCE(AVG(CASE WHEN avg_ms >= 0 THEN avg_ms END), -1), AVG(loss)
		FROM `+table+` WHERE node_id = ?2 AND ts >= ?3 GROUP BY target, b ORDER BY target, b`, step, nodeID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PingSeries{}
	for rows.Next() {
		var name string
		var p PingPoint
		if err := rows.Scan(&name, &p.T, &p.MS, &p.Loss); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].Name != name {
			out = append(out, PingSeries{Name: name})
		}
		last := &out[len(out)-1]
		last.Points = append(last.Points, p)
	}
	return out, rows.Err()
}

// RollupHours (re)computes hourly averages for minute data in [from, to).
// It is idempotent, so overlapping windows are safe.
func (s *Store) RollupHours(ctx context.Context, from, to int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO metrics_1h
		(node_id, ts, cpu, load1, mem_used, swap_used, disk_used, net_in, net_out, tcp, udp, procs)
		SELECT node_id, (ts / 3600) * 3600 AS h, AVG(cpu), AVG(load1),
		CAST(AVG(mem_used) AS INTEGER), CAST(AVG(swap_used) AS INTEGER), CAST(AVG(disk_used) AS INTEGER),
		CAST(AVG(net_in) AS INTEGER), CAST(AVG(net_out) AS INTEGER),
		CAST(AVG(tcp) AS INTEGER), CAST(AVG(udp) AS INTEGER), CAST(AVG(procs) AS INTEGER)
		FROM metrics_1m WHERE ts >= ? AND ts < ? GROUP BY node_id, h`, from, to)
	if err != nil {
		return fmt.Errorf("roll up metrics: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO ping_1h (node_id, target, ts, avg_ms, loss)
		SELECT node_id, target, (ts / 3600) * 3600 AS h,
		COALESCE(AVG(CASE WHEN avg_ms >= 0 THEN avg_ms END), -1), AVG(loss)
		FROM ping_1m WHERE ts >= ? AND ts < ? GROUP BY node_id, target, h`, from, to)
	if err != nil {
		return fmt.Errorf("roll up probes: %w", err)
	}
	return err
}

const maxEvents = 1000

// Cleanup deletes expired data and returns freed pages to the filesystem.
func (s *Store) Cleanup(ctx context.Context, minuteBefore, hourBefore, eventsBefore, now int64) error {
	stmts := []struct {
		q    string
		args []any
	}{
		{"DELETE FROM metrics_1m WHERE ts < ?", []any{minuteBefore}},
		{"DELETE FROM ping_1m WHERE ts < ?", []any{minuteBefore}},
		{"DELETE FROM metrics_1h WHERE ts < ?", []any{hourBefore}},
		{"DELETE FROM ping_1h WHERE ts < ?", []any{hourBefore}},
		{"DELETE FROM alert_events WHERE created_at < ?", []any{eventsBefore}},
		{"DELETE FROM alert_events WHERE id <= (SELECT id FROM alert_events ORDER BY id DESC LIMIT 1 OFFSET ?)", []any{maxEvents}},
		{"DELETE FROM sessions WHERE expires_at < ?", []any{now}},
		{"PRAGMA incremental_vacuum", nil},
	}
	for _, st := range stmts {
		if _, err := s.db.ExecContext(ctx, st.q, st.args...); err != nil {
			return fmt.Errorf("%s: %w", st.q, err)
		}
	}
	return nil
}
