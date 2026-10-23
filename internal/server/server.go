// Package server wires the store, hub, alert engine and HTTP listeners together.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/server/alert"
	"github.com/mby/fenghuo/internal/server/api"
	"github.com/mby/fenghuo/internal/server/auth"
	"github.com/mby/fenghuo/internal/server/availability"
	"github.com/mby/fenghuo/internal/server/config"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/notify"
	"github.com/mby/fenghuo/internal/server/rollup"
	"github.com/mby/fenghuo/internal/server/store"
	"github.com/mby/fenghuo/internal/server/webui"
	"github.com/mby/fenghuo/internal/version"
)

const dbFile = "fenghuo.db"

func OpenStore(cfg config.Config) (*store.Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	return store.Open(filepath.Join(cfg.DataDir, dbFile))
}

func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	st, err := OpenStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := ensureAdmin(ctx, st, cfg.DataDir, log); err != nil {
		return err
	}

	now := time.Now()
	nodes, err := st.ListNodes(ctx)
	if err != nil {
		return err
	}
	traffic, err := st.LoadTraffic(ctx)
	if err != nil {
		return err
	}
	h := hub.New(now)
	h.Reload(nodes, traffic, now)

	notifier := notify.New(st, cfg.NotifyAllowPrivate, log)
	alerts := alert.New(h, st, notifier, log)
	if err := alerts.Reload(ctx); err != nil {
		return err
	}
	roll := &rollup.Runner{Hub: h, Store: st, KeepMinute: cfg.KeepMinute, KeepHour: cfg.KeepHour, Log: log}

	srv := api.New(api.Deps{
		Store: st, Hub: h, Alerts: alerts, Notify: notifier, Web: webui.Handler(),
		Trusted: cfg.TrustedProxies, KeepMinute: cfg.KeepMinute, KeepHour: cfg.KeepHour, DataDir: cfg.DataDir, Log: log,
	})

	var servers []*http.Server
	var listeners []net.Listener
	shared := cfg.AdminListen == config.AdminShared
	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	var requests sync.WaitGroup

	pub, err := listen(cfg.Listen, srv.PublicHandler(shared), log, requestCtx, &requests)
	if err != nil {
		return err
	}
	servers, listeners = append(servers, pub.srv), append(listeners, pub.ln)
	if cfg.TLSCert != "" {
		pub.srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if cfg.AdminListen != config.AdminShared && cfg.AdminListen != config.AdminOff && cfg.AdminListen != "" {
		adm, err := listen(cfg.AdminListen, srv.AdminHandler(), log, requestCtx, &requests)
		if err != nil {
			pub.ln.Close()
			return err
		}
		servers, listeners = append(servers, adm.srv), append(listeners, adm.ln)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Audit remains alive until in-flight HTTP mutations have finished.
	auditCtx, stopAudit := context.WithCancel(context.WithoutCancel(ctx))
	defer stopAudit()
	var wg sync.WaitGroup
	wg.Go(func() { roll.Run(runCtx) })
	wg.Go(func() { alerts.Run(runCtx) })
	wg.Go(func() { notifier.Run(runCtx) })
	wg.Go(func() {
		if err := srv.SecurityAudit.Run(auditCtx); err != nil {
			log.Error("audit worker", "err", err)
		}
	})
	availabilityRunner := &availability.Runner{Hub: h, Store: st, Log: log}
	wg.Go(func() { availabilityRunner.Run(runCtx) })

	errc := make(chan error, len(servers))
	for i, s := range servers {
		ln := listeners[i]
		useTLS := i == 0 && cfg.TLSCert != ""
		go func() {
			var err error
			if useTLS {
				err = s.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
			} else {
				err = s.Serve(ln)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}

	log.Info("fenghuo server started", "version", version.Version, "listen", cfg.Listen,
		"admin", cfg.AdminListen, "tls", cfg.TLSCert != "", "data", cfg.DataDir)

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
		log.Error("listener failed", "err", runErr)
	}

	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	drainHTTP(shutdownCtx, servers, cancelRequests, &requests, log)
	stopAudit()
	cancel()
	wg.Wait()
	flushCtx, stopFlush := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopFlush()
	roll.Flush(flushCtx, time.Now(), true)
	log.Info("fenghuo server stopped")
	return runErr
}

type listener struct {
	srv *http.Server
	ln  net.Listener
}

func drainHTTP(ctx context.Context, servers []*http.Server, cancelRequests context.CancelFunc, requests *sync.WaitGroup, log *slog.Logger) {
	for _, s := range servers {
		if err := s.Shutdown(ctx); err != nil {
			log.Warn("HTTP shutdown deadline; canceling active requests", "err", err)
			cancelRequests()
			_ = s.Close()
		}
	}
	cancelRequests()
	// Close does not wait for handlers. Their audit defers must run before the
	// audit worker drains and the database closes.
	requests.Wait()
}

func listen(addr string, h http.Handler, log *slog.Logger, requestCtx context.Context, requests *sync.WaitGroup) (listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return listener{}, fmt.Errorf("listen %s: %w", addr, err)
	}
	return listener{ln: ln, srv: &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			defer requests.Done()
			h.ServeHTTP(w, r)
		}),
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}}, nil
}

// ensureAdmin creates the admin account on first start with a random
// password written to a 0600 file in the data directory.
func ensureAdmin(ctx context.Context, st *store.Store, dataDir string, log *slog.Logger) error {
	_, err := st.Admin(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	pw := auth.NewPassword()
	path := filepath.Join(dataDir, api.InitialPasswordFile)
	if err := os.WriteFile(path, []byte(pw+"\n"), 0o600); err != nil {
		return err
	}
	if err := st.CreateAdmin(ctx, "admin", auth.HashPassword(pw)); err != nil {
		return err
	}
	log.Warn("created admin account; the initial password is stored in a file readable only by the service user. Change it after the first login.",
		"username", "admin", "password_file", path)
	return nil
}

// ResetPassword sets a new random admin password and signs out all sessions.
func ResetPassword(ctx context.Context, cfg config.Config) (string, error) {
	st, err := OpenStore(cfg)
	if err != nil {
		return "", err
	}
	defer st.Close()
	pw := auth.NewPassword()
	if _, err := st.Admin(ctx); errors.Is(err, store.ErrNotFound) {
		err = st.CreateAdmin(ctx, "admin", auth.HashPassword(pw))
		return pw, err
	}
	if err := st.SetPassword(ctx, auth.HashPassword(pw)); err != nil {
		return "", err
	}
	_ = os.Remove(filepath.Join(cfg.DataDir, api.InitialPasswordFile))
	return pw, st.DeleteAllSessions(ctx)
}

func DisableTOTP(ctx context.Context, cfg config.Config) error {
	st, err := OpenStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.SetTOTP(ctx, "")
}
