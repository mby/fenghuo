package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Settings struct {
	SiteName      string `json:"site_name"`
	PublicEnabled bool   `json:"public_enabled"`
	// PublicURL is the address agents use to reach the server; it is only used
	// to render install commands.
	PublicURL string `json:"public_url"`
	// Lang is the language of notifications: "en" or "zh".
	Lang string `json:"lang"`
	// This is only an installation preset; it is never sent to a running agent.
	AgentIdleInterval int `json:"agent_idle_interval"`
	// ProbeTargets is a copyable local installation preset, never agent control.
	ProbeTargets  string `json:"probe_targets"`
	ProbeCards    string `json:"probe_cards"`
	ProbeInterval int    `json:"probe_interval"`
	PublicSystem  bool   `json:"public_system"`
	PublicExpiry  bool   `json:"public_expiry"`
	PublicTraffic bool   `json:"public_traffic"`
	PublicPing    bool   `json:"public_ping"`
}

func DefaultSettings() Settings {
	return Settings{SiteName: "Fenghuo", PublicEnabled: true, Lang: "en", AgentIdleInterval: 15, ProbeInterval: 30, PublicTraffic: true, PublicPing: true}
}

func (s *Store) loadSettings(ctx context.Context) error {
	v := DefaultSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'site'").Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return err
		}
	}
	s.settings.Store(&v)
	return nil
}

// Settings returns the cached site settings.
func (s *Store) Settings() Settings { return *s.settings.Load() }

func (s *Store) SaveSettings(ctx context.Context, v Settings) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO settings (key, value) VALUES ('site', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", string(b))
	if err != nil {
		return err
	}
	s.settings.Store(&v)
	return nil
}
