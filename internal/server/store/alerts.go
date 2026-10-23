package store

import (
	"context"
	"encoding/json"
	"time"
)

type Rule struct {
	Target    string  `json:"target"`
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Threshold float64 `json:"threshold"`
	Duration  int64   `json:"duration"` // seconds
	NodeIDs   []int64 `json:"node_ids"` // empty = all nodes
	Enabled   bool    `json:"enabled"`
	CreatedAt int64   `json:"created_at"`
}

type Channel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	Enabled   bool            `json:"enabled"`
	CreatedAt int64           `json:"created_at"`
}

type AlertState struct {
	RuleID  int64
	NodeID  int64
	Ref     int64 // context of a one-shot alert, e.g. traffic period start or expiry date
	FiredAt int64
}

type Event struct {
	ID        int64  `json:"id"`
	RuleID    int64  `json:"rule_id"`
	NodeID    int64  `json:"node_id"`
	Kind      string `json:"kind"`
	Firing    bool   `json:"firing"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, kind, threshold, duration, node_ids, enabled, created_at, target FROM alert_rules ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var ids string
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Threshold, &r.Duration, &ids, &enabled, &r.CreatedAt, &r.Target); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ids), &r.NodeIDs); err != nil {
			return nil, err
		}
		if r.NodeIDs == nil {
			r.NodeIDs = []int64{}
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRule(ctx context.Context, r *Rule) error {
	ids, _ := json.Marshal(nonNil(r.NodeIDs))
	r.CreatedAt = time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO alert_rules (name, kind, threshold, duration, node_ids, enabled, created_at, target) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		r.Name, r.Kind, r.Threshold, r.Duration, string(ids), boolInt(r.Enabled), r.CreatedAt, r.Target)
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	return err
}

// UpdateRule replaces a rule and clears its alert state, because the
// condition it was evaluated against may no longer apply.
func (s *Store) UpdateRule(ctx context.Context, r Rule) error {
	ids, _ := json.Marshal(nonNil(r.NodeIDs))
	if err := affected(s.db.ExecContext(ctx,
		"UPDATE alert_rules SET name = ?, kind = ?, threshold = ?, duration = ?, node_ids = ?, enabled = ?, target = ? WHERE id = ?",
		r.Name, r.Kind, r.Threshold, r.Duration, string(ids), boolInt(r.Enabled), r.Target, r.ID)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM alert_state WHERE rule_id = ?", r.ID)
	return err
}

func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM alert_rules WHERE id = ?", id))
}

func (s *Store) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, type, config, enabled, created_at FROM notify_channels ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Channel{}
	for rows.Next() {
		var c Channel
		var cfg string
		var enabled int
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &cfg, &enabled, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Config = json.RawMessage(cfg)
		c.Enabled = enabled != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateChannel(ctx context.Context, c *Channel) error {
	c.CreatedAt = time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO notify_channels (name, type, config, enabled, created_at) VALUES (?, ?, ?, ?, ?)",
		c.Name, c.Type, string(c.Config), boolInt(c.Enabled), c.CreatedAt)
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateChannel(ctx context.Context, c Channel) error {
	return affected(s.db.ExecContext(ctx,
		"UPDATE notify_channels SET name = ?, type = ?, config = ?, enabled = ? WHERE id = ?",
		c.Name, c.Type, string(c.Config), boolInt(c.Enabled), c.ID))
}

func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM notify_channels WHERE id = ?", id))
}

func (s *Store) ListAlertStates(ctx context.Context) ([]AlertState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT rule_id, node_id, ref, fired_at FROM alert_state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertState
	for rows.Next() {
		var a AlertState
		if err := rows.Scan(&a.RuleID, &a.NodeID, &a.Ref, &a.FiredAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) PutAlertState(ctx context.Context, a AlertState) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO alert_state (rule_id, node_id, ref, fired_at) VALUES (?, ?, ?, ?)",
		a.RuleID, a.NodeID, a.Ref, a.FiredAt)
	return err
}

func (s *Store) DeleteAlertState(ctx context.Context, ruleID, nodeID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM alert_state WHERE rule_id = ? AND node_id = ?", ruleID, nodeID)
	return err
}

func (s *Store) AddEvent(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO alert_events (rule_id, node_id, kind, firing, message, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		e.RuleID, e.NodeID, e.Kind, boolInt(e.Firing), e.Message, e.CreatedAt)
	return err
}

func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, rule_id, node_id, kind, firing, message, created_at FROM alert_events ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var firing int
		if err := rows.Scan(&e.ID, &e.RuleID, &e.NodeID, &e.Kind, &firing, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Firing = firing != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
