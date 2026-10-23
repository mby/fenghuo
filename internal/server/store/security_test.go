package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecuritySessionManagement(t *testing.T) {
	for _, mode := range []string{"other", "current", "all_others"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t)
			for _, hash := range []string{"current-secret-hash", "other-secret-hash", "expired-secret-hash"} {
				expires := int64(200)
				if strings.HasPrefix(hash, "expired") {
					expires = 100
				}
				if err := s.CreateSession(ctx, hash, 1, expires, "192.0.2.1", "browser"); err != nil {
					t.Fatal(err)
				}
			}
			list, err := s.ListSessions(ctx, "current-secret-hash", 100)
			if err != nil || len(list) != 2 {
				t.Fatalf("list: %+v %v", list, err)
			}
			var current, other string
			for _, v := range list {
				if !ValidSessionID(v.ID) {
					t.Fatalf("invalid opaque id: %q", v.ID)
				}
				if valid, err := s.SessionValid(ctx, v.ID, 100); err != nil || valid {
					t.Fatalf("management ID authenticated: %v %v", valid, err)
				}
				if v.Current {
					current = v.ID
				} else {
					other = v.ID
				}
			}
			if current == "" || other == "" || current == other {
				t.Fatalf("current detection: %+v", list)
			}
			encoded, _ := json.Marshal(list)
			if strings.Contains(string(encoded), "secret-hash") || strings.Contains(string(encoded), "token_hash") {
				t.Fatal("session response leaked credential")
			}
			switch mode {
			case "current":
				err = s.RevokeSession(ctx, current)
			case "other":
				err = s.RevokeSession(ctx, other)
			case "all_others":
				err = s.RevokeOtherSessions(ctx, "current-secret-hash")
			}
			if err != nil {
				t.Fatal(err)
			}
			currentValid, _ := s.SessionValid(ctx, "current-secret-hash", 100)
			otherValid, _ := s.SessionValid(ctx, "other-secret-hash", 100)
			if currentValid != (mode != "current") || otherValid != (mode == "current") {
				t.Fatalf("revocation leaked across sessions: current=%v other=%v", currentValid, otherValid)
			}
			if err := s.RevokeSession(ctx, "current-secret-hash"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("token hash accepted as id: %v", err)
			}
		})
	}
}

func TestSecurityMigratesOldSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := fs.Glob(schemaFS, "schema/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name >= "schema/005_security.sql" {
			break
		}
		body, err := schemaFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO sessions (token_hash, created_at, expires_at) VALUES ('old-one', 1, 999), ('old-two', 2, 999)"); err != nil {
		t.Fatal(err)
	}
	body, err := schemaFS.ReadFile("schema/005_security.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db}
	list, err := s.ListSessions(context.Background(), "old-one", 100)
	if err != nil || len(list) != 2 {
		t.Fatalf("migrated list: %+v %v", list, err)
	}
	if list[0].ID == list[1].ID || !ValidSessionID(list[0].ID) || !ValidSessionID(list[1].ID) {
		t.Fatalf("migration IDs: %+v", list)
	}
	if valid, _ := s.SessionValid(context.Background(), "old-one", 100); !valid {
		t.Fatal("migration invalidated old login")
	}
}

func TestSecurityAuditRetentionAndPagination(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for batch := 0; batch < 11; batch++ {
		events := make([]SecurityAuditEvent, 1000)
		for i := range events {
			events[i] = SecurityAuditEvent{CreatedAt: now, Action: "login", Status: 401, IP: "192.0.2.1"}
		}
		if err := s.SaveSecurityAudit(ctx, events, now); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM security_audit").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > SecurityAuditLimit {
			t.Fatalf("unbounded rows: %d", count)
		}
	}
	page, err := s.ListSecurityAudit(ctx, SecurityAuditFilter{Action: "login", Outcome: "failure", Limit: 7}, now)
	if err != nil || len(page.Items) != 7 || page.NextBefore == 0 {
		t.Fatalf("page: %+v %v", page, err)
	}
	next, err := s.ListSecurityAudit(ctx, SecurityAuditFilter{Before: page.NextBefore, Limit: 7}, now)
	if err != nil || len(next.Items) != 7 || next.Items[0].ID >= page.NextBefore {
		t.Fatalf("cursor overlap: %+v %v", next, err)
	}
	empty, err := s.ListSecurityAudit(ctx, SecurityAuditFilter{Outcome: "success"}, now)
	if err != nil || len(empty.Items) != 0 {
		t.Fatalf("filter: %+v %v", empty, err)
	}
	bounded, err := s.ListSecurityAudit(ctx, SecurityAuditFilter{Limit: 100000}, now)
	if err != nil || len(bounded.Items) != SecurityAuditPageLimit {
		t.Fatalf("unbounded page: %d %v", len(bounded.Items), err)
	}
	later := now + int64(SecurityAuditRetention.Seconds())
	expired, err := s.ListSecurityAudit(ctx, SecurityAuditFilter{}, later)
	if err != nil || len(expired.Items) != 0 {
		t.Fatal("expired records visible before maintenance")
	}
	if err := s.SaveSecurityAudit(ctx, nil, later); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM security_audit").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retention: %d %v", count, err)
	}
}

func TestSecurityAuditValidation(t *testing.T) {
	for _, tc := range []struct {
		name, action, target, ip, wantTarget, wantIP string
		accepted                                     bool
	}{
		{"numeric", "node.update", "12", "::ffff:192.0.2.1", "12", "192.0.2.1", true},
		{"credentials", "node.update", "password=secret", "secret", "", "", true},
		{"long", "node.update", strings.Repeat("1", 300), "", "", "", true},
		{"noncanonical", "node.update", "+1", "", "", "", true},
		{"login_target", "login", "123", "", "", "", true},
		{"setup", "totp.setup", "", "", "", "", false},
		{"unknown", "password=secret", "", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := NormalizeSecurityAudit(SecurityAuditEvent{CreatedAt: 1, Action: tc.action, Target: tc.target, IP: tc.ip, Status: 401})
			if ok != tc.accepted || e.Target != tc.wantTarget || e.IP != tc.wantIP {
				t.Fatalf("normalized: %+v %v", e, ok)
			}
		})
	}
}
