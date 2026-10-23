package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	DeliveryQueueCap    = 1000
	DeliveryHistoryCap  = 5000
	DeliveryMaxAttempts = 5
	DeliveryRetention   = 7 * 24 * time.Hour
	DeliveryLease       = time.Minute
	DeliveryMaxPayload  = 32 << 10
)

var ErrDeliveryQueueFull = errors.New("notification queue full")

type Delivery struct {
	ID            int64             `json:"id"`
	ChannelID     int64             `json:"channel_id"`
	Kind          string            `json:"kind"`
	Firing        bool              `json:"firing"`
	Status        string            `json:"status"`
	Attempts      int               `json:"attempts"`
	MaxAttempts   int               `json:"max_attempts"`
	CreatedAt     int64             `json:"created_at"`
	UpdatedAt     int64             `json:"updated_at"`
	NextAttemptAt int64             `json:"next_attempt_at"`
	LastError     string            `json:"last_error"`
	History       []DeliveryAttempt `json:"history"`
	Payload       json.RawMessage   `json:"-"`
}

type DeliveryAttempt struct {
	Number     int    `json:"number"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

// EnqueueDeliveries atomically fans out to currently enabled channels. No channel
// credentials are copied; workers load current configuration for every attempt.
func (s *Store) EnqueueDeliveries(ctx context.Context, payload json.RawMessage, kind string, firing bool, now time.Time) error {
	if len(payload) > DeliveryMaxPayload || !json.Valid(payload) || len(kind) > 64 {
		return errors.New("invalid notification payload")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deliveries: %w", err)
	}
	defer tx.Rollback()
	if err := enqueueDeliveries(ctx, tx, payload, kind, firing, now); err != nil {
		return err
	}
	return tx.Commit()
}

func enqueueDeliveries(ctx context.Context, tx *sql.Tx, payload json.RawMessage, kind string, firing bool, now time.Time) error {
	var active, channels int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM notification_deliveries WHERE status IN ('pending','retrying','sending')").Scan(&active); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM notify_channels WHERE enabled = 1").Scan(&channels); err != nil {
		return err
	}
	if active+channels > DeliveryQueueCap {
		return ErrDeliveryQueueFull
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_deliveries
		(channel_id, payload, kind, firing, created_at, updated_at, next_attempt_at, max_attempts)
		SELECT id, ?, ?, ?, ?, ?, ?, ? FROM notify_channels WHERE enabled = 1 ORDER BY id`,
		string(payload), kind, firing, now.Unix(), now.Unix(), now.Unix(), DeliveryMaxAttempts); err != nil {
		return err
	}
	if err := pruneDeliveries(ctx, tx, now); err != nil {
		return err
	}
	return nil
}

// CommitAlertDelivery makes the alert transition, event and outbox inseparable.
// Capacity rejection rolls everything back so the next evaluation can retry.
func (s *Store) CommitAlertDelivery(ctx context.Context, state AlertState, event Event, payload json.RawMessage) error {
	if len(payload) > DeliveryMaxPayload || !json.Valid(payload) || len(event.Kind) > 64 {
		return errors.New("invalid notification payload")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if event.Firing {
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO alert_state (rule_id, node_id, ref, fired_at) VALUES (?, ?, ?, ?)`,
			state.RuleID, state.NodeID, state.Ref, state.FiredAt)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM alert_state WHERE rule_id = ? AND node_id = ?", state.RuleID, state.NodeID)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_events (rule_id, node_id, kind, firing, message, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		event.RuleID, event.NodeID, event.Kind, event.Firing, event.Message, event.CreatedAt); err != nil {
		return err
	}
	if err := enqueueDeliveries(ctx, tx, payload, event.Kind, event.Firing, time.Unix(event.CreatedAt, 0)); err != nil {
		return err
	}
	return tx.Commit()
}

// DeliveryErrorCode is an allowlist, never a redaction heuristic. Remote error
// strings, URLs, headers and response bodies must not reach history or logs.
func DeliveryErrorCode(code string) string {
	switch code {
	case "", "timeout", "canceled", "transport_error", "invalid_config", "invalid_payload",
		"http_error", "telegram_error", "channel_unavailable", "interrupted":
		return code
	default:
		return "delivery_failed"
	}
}

func DeliveryBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 30 * time.Second
	case 3:
		return 2 * time.Minute
	default:
		return 10 * time.Minute
	}
}

// ClaimDelivery leases one attempt. FIFO within a channel keeps a recovery behind
// its firing message; independent channels can be delivered concurrently.
func (s *Store) ClaimDelivery(ctx context.Context, now time.Time) (*Delivery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var d Delivery
	var payload string
	err = tx.QueryRowContext(ctx, `SELECT d.id, d.channel_id, d.payload, d.kind, d.firing, d.attempts, d.max_attempts
		FROM notification_deliveries d
		JOIN notify_channels c ON c.id = d.channel_id AND c.enabled = 1
		WHERE d.status IN ('pending','retrying') AND d.next_attempt_at <= ? AND d.attempts < d.max_attempts
		AND NOT EXISTS (SELECT 1 FROM notification_deliveries older
		 WHERE older.channel_id = d.channel_id AND older.id < d.id AND older.status IN ('pending','retrying','sending'))
		ORDER BY d.next_attempt_at, d.id LIMIT 1`, now.Unix()).
		Scan(&d.ID, &d.ChannelID, &payload, &d.Kind, &d.Firing, &d.Attempts, &d.MaxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim delivery: %w", err)
	}
	d.Attempts++
	d.Payload = json.RawMessage(payload)
	d.Status = "sending"
	if _, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET status = 'sending', attempts = ?, updated_at = ?, lease_until = ?, next_attempt_at = 0 WHERE id = ?`,
		d.Attempts, now.Unix(), now.Add(DeliveryLease).Unix(), d.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_attempts (delivery_id, number, started_at, status) VALUES (?, ?, ?, 'sending')`,
		d.ID, d.Attempts, now.Unix()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) DeliveryChannel(ctx context.Context, d Delivery) (Channel, error) {
	var ch Channel
	var config string
	err := s.db.QueryRowContext(ctx, `SELECT c.id, c.type, c.config, c.enabled
		FROM notify_channels c JOIN notification_deliveries d ON d.channel_id = c.id
		WHERE d.id = ? AND d.attempts = ? AND d.status = 'sending' AND c.enabled = 1`, d.ID, d.Attempts).
		Scan(&ch.ID, &ch.Type, &config, &ch.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ch, ErrNotFound
	}
	ch.Config = json.RawMessage(config)
	return ch, err
}

func (s *Store) FinishDelivery(ctx context.Context, d Delivery, code string, now time.Time) error {
	code = DeliveryErrorCode(code)
	status, attemptStatus := "succeeded", "succeeded"
	var next int64
	if code != "" {
		status, attemptStatus = "failed", "failed"
		if code == "channel_unavailable" {
			status, attemptStatus = "canceled", "canceled"
		} else if d.Attempts < d.MaxAttempts {
			status = "retrying"
			next = now.Add(DeliveryBackoff(d.Attempts)).Unix()
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
		SET status = ?, updated_at = ?, next_attempt_at = ?, lease_until = 0, last_error = ?,
		payload = CASE WHEN ? = 'retrying' THEN payload ELSE '' END
		WHERE id = ? AND status = 'sending' AND attempts = ?`,
		status, now.Unix(), next, code, status, d.ID, d.Attempts)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE notification_attempts SET status = ?, finished_at = ?, error = ?
			WHERE delivery_id = ? AND number = ? AND status = 'sending'`,
			attemptStatus, now.Unix(), code, d.ID, d.Attempts); err != nil {
			return err
		}
	}
	if err := pruneDeliveries(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

// MaintainDeliveries reclaims expired leases after crashes. An uncertain send
// consumes an attempt, so process restarts cannot create infinite retries.
func (s *Store) MaintainDeliveries(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE notification_attempts SET status = 'interrupted', error = 'interrupted', finished_at = ?
		WHERE status = 'sending' AND delivery_id IN
		(SELECT id FROM notification_deliveries WHERE status = 'sending' AND lease_until <= ?)`, now.Unix(), now.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
		SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'retrying' END,
		last_error = 'interrupted', updated_at = ?, lease_until = 0,
		next_attempt_at = CASE WHEN attempts >= max_attempts THEN 0 ELSE ? END,
		payload = CASE WHEN attempts >= max_attempts THEN '' ELSE payload END
		WHERE status = 'sending' AND lease_until <= ?`,
		now.Unix(), now.Add(DeliveryBackoff(DeliveryMaxAttempts)).Unix(), now.Unix()); err != nil {
		return err
	}
	if err := pruneDeliveries(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

func pruneDeliveries(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM notification_deliveries WHERE status IN ('succeeded','failed','canceled')
		AND (updated_at < ? OR id NOT IN (SELECT id FROM notification_deliveries
		WHERE status IN ('succeeded','failed','canceled') ORDER BY id DESC LIMIT ?))`,
		now.Add(-DeliveryRetention).Unix(), DeliveryHistoryCap)
	return err
}

// ListDeliveries returns sanitized newest-first metadata; payloads and channel
// configuration are intentionally absent, even for administrators.
func (s *Store) ListDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, channel_id, kind, firing, status, attempts, max_attempts,
		created_at, updated_at, next_attempt_at, last_error FROM notification_deliveries ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	out := []Delivery{}
	index := map[int64]int{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.Kind, &d.Firing, &d.Status, &d.Attempts, &d.MaxAttempts,
			&d.CreatedAt, &d.UpdatedAt, &d.NextAttemptAt, &d.LastError); err != nil {
			rows.Close()
			return nil, err
		}
		d.History = []DeliveryAttempt{}
		index[d.ID] = len(out)
		out = append(out, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT delivery_id, number, started_at, finished_at, status, error FROM notification_attempts
		WHERE delivery_id IN (SELECT id FROM notification_deliveries ORDER BY id DESC LIMIT ?) ORDER BY number`, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var a DeliveryAttempt
		if err := rows.Scan(&id, &a.Number, &a.StartedAt, &a.FinishedAt, &a.Status, &a.Error); err != nil {
			rows.Close()
			return nil, err
		}
		i := index[id]
		out[i].History = append(out[i].History, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
