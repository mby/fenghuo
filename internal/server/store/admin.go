package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Admin struct {
	Username     string
	PasswordHash string
	TOTPSecret   string
}

func (s *Store) Admin(ctx context.Context) (Admin, error) {
	var a Admin
	err := s.db.QueryRowContext(ctx, "SELECT username, password_hash, totp_secret FROM admin WHERE id = 1").
		Scan(&a.Username, &a.PasswordHash, &a.TOTPSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO admin (id, username, password_hash, updated_at) VALUES (1, ?, ?, ?)",
		username, passwordHash, time.Now().Unix())
	return err
}

func (s *Store) SetPassword(ctx context.Context, passwordHash string) error {
	return affected(s.db.ExecContext(ctx,
		"UPDATE admin SET password_hash = ?, updated_at = ? WHERE id = 1", passwordHash, time.Now().Unix()))
}

func (s *Store) SetTOTP(ctx context.Context, secret string) error {
	return affected(s.db.ExecContext(ctx,
		"UPDATE admin SET totp_secret = ?, updated_at = ? WHERE id = 1", secret, time.Now().Unix()))
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, created, expires int64, ip, userAgent string) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Errorf("generate session id: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (id, token_hash, created_at, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?)",
		hex.EncodeToString(id[:]), tokenHash, created, expires, ip, userAgent)
	return err
}

// Session exposes a management identifier that cannot authenticate a request.
type Session struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Current   bool   `json:"current"`
}

func (s *Store) ListSessions(ctx context.Context, currentTokenHash string, now int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, expires_at, ip, user_agent, token_hash = ?
		FROM sessions WHERE expires_at > ? ORDER BY created_at DESC, id`, currentTokenHash, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		var v Session
		if err := rows.Scan(&v.ID, &v.CreatedAt, &v.ExpiresAt, &v.IP, &v.UserAgent, &v.Current); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RevokeSession(ctx context.Context, id string) error {
	if err := affected(s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Store) RevokeOtherSessions(ctx context.Context, currentTokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash <> ?", currentTokenHash)
	if err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	return nil
}

func (s *Store) SessionValid(ctx context.Context, tokenHash string, now int64) (bool, error) {
	var exp int64
	err := s.db.QueryRowContext(ctx, "SELECT expires_at FROM sessions WHERE token_hash = ?", tokenHash).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exp > now, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

func (s *Store) DeleteAllSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions")
	return err
}
