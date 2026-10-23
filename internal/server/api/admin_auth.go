package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/store"
)

const (
	sessionCookie = "fenghuo_session"
	sessionTTL    = 7 * 24 * time.Hour
	smallBody     = 8 << 10

	// InitialPasswordFile holds the generated first-run password until it is changed.
	InitialPasswordFile = "initial_admin_password"
)

func (s *Server) mountAdmin(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/api/login", s.auditAdmin(s.handleLogin))

	a := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.requireAdmin(s.auditAdmin(h))) }
	a("POST /admin/api/logout", s.handleLogout)
	a("GET /admin/api/sessions", s.handleListSessions)
	a("DELETE /admin/api/sessions/{sessionID}", s.handleRevokeSession)
	a("DELETE /admin/api/sessions", s.handleRevokeOtherSessions)
	a("GET /admin/api/audit", s.handleListSecurityAudit)
	a("GET /admin/api/deliveries", s.handleListDeliveries)
	a("GET /admin/api/me", s.handleMe)
	a("POST /admin/api/password", s.handlePassword)
	a("GET /admin/api/totp/setup", s.handleTOTPSetup)
	a("POST /admin/api/totp/enable", s.handleTOTPEnable)
	a("POST /admin/api/totp/disable", s.handleTOTPDisable)

	a("GET /admin/api/settings", s.handleGetSettings)
	a("PUT /admin/api/settings", s.handlePutSettings)

	a("GET /admin/api/nodes", s.handleListNodes)
	a("GET /admin/api/costs", s.handleCosts)
	a("POST /admin/api/nodes", s.handleCreateNode)
	a("PUT /admin/api/nodes/{id}", s.handleUpdateNode)
	a("DELETE /admin/api/nodes/{id}", s.handleDeleteNode)
	a("POST /admin/api/nodes/{id}/token", s.handleRotateToken)

	a("GET /admin/api/rules", s.handleListRules)
	a("POST /admin/api/rules", s.handleCreateRule)
	a("PUT /admin/api/rules/{id}", s.handleUpdateRule)
	a("DELETE /admin/api/rules/{id}", s.handleDeleteRule)

	a("GET /admin/api/channels", s.handleListChannels)
	a("POST /admin/api/channels", s.handleCreateChannel)
	a("PUT /admin/api/channels/{id}", s.handleUpdateChannel)
	a("DELETE /admin/api/channels/{id}", s.handleDeleteChannel)
	a("POST /admin/api/channels/{id}/test", s.handleTestChannel)

	a("GET /admin/api/events", s.handleListEvents)
}

func (s *Server) sessionOK(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	ok, err := s.Store.SessionValid(r.Context(), auth.HashToken(c.Value), time.Now().Unix())
	if err != nil {
		s.Log.Error("session lookup", "err", err)
	}
	return ok
}

func (s *Server) requireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sessionOK(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) error {
	token := auth.NewToken("")
	now := time.Now()
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	if err := s.Store.CreateSession(r.Context(), auth.HashToken(token), now.Unix(), now.Add(sessionTTL).Unix(), s.clientIP(r), ua); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	ip := s.clientIP(r)
	if s.loginIP.Blocked(ip, now) || s.loginAll.Blocked("*", now) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decodeJSON(w, r, &req, smallBody) {
		return
	}
	adm, err := s.Store.Admin(r.Context())
	if err != nil {
		s.Log.Error("load admin", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(adm.Username)) == 1
	passOK := auth.VerifyPassword(req.Password, adm.PasswordHash)
	fail := func(code string) {
		s.loginIP.Fail(ip, now)
		s.loginAll.Fail("*", now)
		s.Log.Warn("admin login failed", "ip", ip, "reason", code)
		writeError(w, http.StatusUnauthorized, code)
	}
	if !userOK || !passOK {
		fail("invalid_credentials")
		return
	}
	if adm.TOTPSecret != "" {
		if req.Code == "" {
			writeError(w, http.StatusUnauthorized, "totp_required")
			return
		}
		if !s.checkTOTP(adm.TOTPSecret, req.Code, now) {
			fail("invalid_code")
			return
		}
	}
	s.loginIP.Reset(ip)
	if err := s.startSession(w, r); err != nil {
		s.Log.Error("create session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	s.Log.Info("admin logged in", "ip", ip)
	w.WriteHeader(http.StatusNoContent)
}

// checkTOTP verifies a code and rejects reuse of an already accepted step.
func (s *Server) checkTOTP(secret, code string, now time.Time) bool {
	counter, ok := auth.VerifyTOTP(secret, code, now)
	if !ok {
		return false
	}
	for {
		last := s.lastTOTP.Load()
		if counter <= last {
			return false
		}
		if s.lastTOTP.CompareAndSwap(last, counter) {
			return true
		}
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	adm, err := s.Store.Admin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	_, statErr := os.Stat(filepath.Join(s.DataDir, InitialPasswordFile))
	writeJSON(w, http.StatusOK, map[string]any{
		"username":         adm.Username,
		"totp_enabled":     adm.TOTPSecret != "",
		"default_password": statErr == nil,
	})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decodeJSON(w, r, &req, smallBody) {
		return
	}
	if n := utf8.RuneCountInString(req.New); n < 10 || n > 128 {
		writeErrorMsg(w, http.StatusBadRequest, "weak_password", "password must be 10-128 characters")
		return
	}
	adm, err := s.Store.Admin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if !auth.VerifyPassword(req.Current, adm.PasswordHash) {
		writeError(w, http.StatusForbidden, "invalid_credentials")
		return
	}
	if err := s.Store.SetPassword(r.Context(), auth.HashPassword(req.New)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	// Changing the password signs out every other session.
	if err := s.Store.DeleteAllSessions(r.Context()); err != nil {
		s.Log.Error("delete sessions", "err", err)
	}
	if err := os.Remove(filepath.Join(s.DataDir, InitialPasswordFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Log.Warn("remove initial password file", "err", err)
	}
	if err := s.startSession(w, r); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	s.Log.Info("admin password changed", "ip", s.clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	adm, err := s.Store.Admin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	secret := auth.NewTOTPSecret()
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": secret,
		"uri":    auth.TOTPURI(s.Store.Settings().SiteName, adm.Username, secret),
	})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if !decodeJSON(w, r, &req, smallBody) {
		return
	}
	if !auth.ValidTOTPSecret(req.Secret) {
		writeError(w, http.StatusBadRequest, "invalid_secret")
		return
	}
	if !s.checkTOTP(req.Secret, req.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}
	if err := s.Store.SetTOTP(r.Context(), req.Secret); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	s.Log.Info("two-factor authentication enabled")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decodeJSON(w, r, &req, smallBody) {
		return
	}
	adm, err := s.Store.Admin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if !auth.VerifyPassword(req.Password, adm.PasswordHash) {
		writeError(w, http.StatusForbidden, "invalid_credentials")
		return
	}
	if adm.TOTPSecret != "" && !s.checkTOTP(adm.TOTPSecret, req.Code, time.Now()) {
		writeError(w, http.StatusForbidden, "invalid_code")
		return
	}
	if err := s.Store.SetTOTP(r.Context(), ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	s.Log.Warn("two-factor authentication disabled")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Settings())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var v store.Settings
	if !decodeJSON(w, r, &v, smallBody) {
		return
	}
	if err := validateSettings(&v); err != nil {
		writeErrorMsg(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}
	if err := s.Store.SaveSettings(r.Context(), v); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, v)
}
