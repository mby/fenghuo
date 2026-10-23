package store

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const (
	SecurityAuditLimit     = 10000
	SecurityAuditRetention = 30 * 24 * time.Hour
	SecurityAuditPageLimit = 100
)

type SecurityAuditEvent struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Status    int    `json:"status"`
	IP        string `json:"ip"`
}

// Targets are either numeric database IDs or opaque session IDs, never arbitrary paths.
func ValidSessionID(v string) bool {
	return len(v) == 32 && strings.Trim(v, "0123456789abcdef") == ""
}

func ValidSecurityAction(action string) bool {
	switch action {
	case "login", "logout", "password.change", "totp.enable", "totp.disable", "settings.update",
		"node.create", "node.update", "node.delete", "node.token.rotate",
		"rule.create", "rule.update", "rule.delete", "channel.create", "channel.update", "channel.delete", "channel.test",
		"session.revoke", "session.revoke_others":
		return true
	}
	return false
}

// NormalizeSecurityAudit applies the same boundary to middleware and other callers.
func NormalizeSecurityAudit(e SecurityAuditEvent) (SecurityAuditEvent, bool) {
	if !ValidSecurityAction(e.Action) || e.Status < 200 || e.Status > 599 || e.CreatedAt <= 0 {
		return SecurityAuditEvent{}, false
	}
	e.ID = 0
	if e.Target != "" {
		valid := false
		switch {
		case e.Action == "session.revoke":
			valid = ValidSessionID(e.Target)
		case strings.HasPrefix(e.Action, "node."), strings.HasPrefix(e.Action, "rule."), strings.HasPrefix(e.Action, "channel."):
			id, err := strconv.ParseInt(e.Target, 10, 64)
			valid = err == nil && id > 0 && strconv.FormatInt(id, 10) == e.Target
		}
		if !valid {
			e.Target = ""
		}
	}
	if ip, err := netip.ParseAddr(e.IP); err == nil && ip.Zone() == "" {
		e.IP = ip.Unmap().String()
	} else {
		e.IP = ""
	}
	// A small path value may share backing storage with a very large request URI.
	// Detach retained strings so queue memory stays bounded by the schema limits.
	e.Action = strings.Clone(e.Action)
	e.Target = strings.Clone(e.Target)
	return e, true
}

// SaveSecurityAudit is called only by the background audit worker. Pruning and
// insertion commit together, so readers never observe more than 10,000 rows.
func (s *Store) SaveSecurityAudit(ctx context.Context, events []SecurityAuditEvent, now int64) error {
	if len(events) > 2048 {
		return fmt.Errorf("security audit batch exceeds limit")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin security audit: %w", err)
	}
	defer tx.Rollback()
	for _, event := range events {
		e, ok := NormalizeSecurityAudit(event)
		if !ok {
			return fmt.Errorf("invalid security audit event")
		}
		if e.CreatedAt > now {
			e.CreatedAt = now
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO security_audit (created_at, action, target, status, ip) VALUES (?, ?, ?, ?, ?)", e.CreatedAt, e.Action, e.Target, e.Status, e.IP); err != nil {
			return fmt.Errorf("insert security audit: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM security_audit WHERE created_at <= ?", now-int64(SecurityAuditRetention.Seconds())); err != nil {
		return fmt.Errorf("expire security audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM security_audit WHERE id IN (SELECT id FROM security_audit ORDER BY id DESC LIMIT -1 OFFSET ?)", SecurityAuditLimit); err != nil {
		return fmt.Errorf("bound security audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit security audit: %w", err)
	}
	return nil
}

type SecurityAuditFilter struct {
	Action  string
	Outcome string // empty, success, failure
	Before  int64
	Limit   int
}

type SecurityAuditPage struct {
	Items      []SecurityAuditEvent `json:"items"`
	NextBefore int64                `json:"next_before"`
}

func (s *Store) ListSecurityAudit(ctx context.Context, filter SecurityAuditFilter, now int64) (SecurityAuditPage, error) {
	page := SecurityAuditPage{Items: make([]SecurityAuditEvent, 0)}
	if filter.Action != "" && !ValidSecurityAction(filter.Action) || filter.Outcome != "" && filter.Outcome != "success" && filter.Outcome != "failure" || filter.Before < 0 {
		return page, fmt.Errorf("invalid security audit filter")
	}
	if filter.Limit <= 0 {
		filter.Limit = 25
	}
	if filter.Limit > SecurityAuditPageLimit {
		filter.Limit = SecurityAuditPageLimit
	}
	query := "SELECT id, created_at, action, target, status, ip FROM security_audit WHERE created_at > ?"
	args := []any{now - int64(SecurityAuditRetention.Seconds())}
	if filter.Action != "" {
		query += " AND action = ?"
		args = append(args, filter.Action)
	}
	if filter.Before > 0 {
		query += " AND id < ?"
		args = append(args, filter.Before)
	}
	if filter.Outcome == "success" {
		query += " AND status < 400"
	}
	if filter.Outcome == "failure" {
		query += " AND status >= 400"
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, filter.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return page, fmt.Errorf("list security audit: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e SecurityAuditEvent
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Action, &e.Target, &e.Status, &e.IP); err != nil {
			return page, fmt.Errorf("scan security audit: %w", err)
		}
		page.Items = append(page.Items, e)
	}
	if len(page.Items) > filter.Limit {
		page.Items = page.Items[:filter.Limit]
		page.NextBefore = page.Items[len(page.Items)-1].ID
	}
	return page, rows.Err()
}
