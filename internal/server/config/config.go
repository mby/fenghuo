// Package config parses server settings from flags and FENGHUO_* environment variables.
package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"runtime"
	"strings"
	"time"

	"github.com/mby/fenghuo/internal/flagenv"
)

const (
	AdminShared = "shared" // serve the admin UI on the public listener
	AdminOff    = "off"    // no admin UI at all
)

type Config struct {
	DataDir            string
	Listen             string
	AdminListen        string
	TLSCert            string
	TLSKey             string
	TrustedProxies     []netip.Prefix
	KeepMinute         time.Duration
	KeepHour           time.Duration
	NotifyAllowPrivate bool
	LogLevel           slog.Level
}

func defaultDataDir() string {
	if runtime.GOOS == "linux" {
		return "/var/lib/fenghuo"
	}
	return "./data"
}

func Parse(name string, args []string) (Config, error) {
	var c Config
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&c.DataDir, "data", defaultDataDir(), "data directory (database lives here)")
	fs.StringVar(&c.Listen, "listen", "127.0.0.1:8080", "public listener: agent reports and the read-only status page")
	fs.StringVar(&c.AdminListen, "admin-listen", "127.0.0.1:8081", `admin listener; "shared" serves admin on -listen, "off" disables admin`)
	fs.StringVar(&c.TLSCert, "tls-cert", "", "TLS certificate file for the public listener (optional, prefer a reverse proxy)")
	fs.StringVar(&c.TLSKey, "tls-key", "", "TLS private key file for the public listener")
	trusted := fs.String("trusted-proxies", "127.0.0.1/32,::1/128", "comma-separated CIDRs whose X-Forwarded-For/-Proto headers are trusted")
	keepMinuteDays := fs.Int("keep-minute-days", 7, "days to keep 1-minute metrics and latency data (1-90)")
	keepHourDays := fs.Int("keep-hour-days", 90, "days to keep 1-hour metrics and probe summaries (1-730)")
	fs.BoolVar(&c.NotifyAllowPrivate, "notify-allow-private", false, "allow notifications to private/loopback addresses (self-hosted webhooks, local proxies)")
	logLevel := fs.String("log-level", "info", "debug, info, warn or error")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s [flags]\n\nEvery flag can also be set with FENGHUO_<NAME>, e.g. FENGHUO_ADMIN_LISTEN.\n\n", name)
		fs.PrintDefaults()
	}
	if err := flagenv.Parse(fs, args); err != nil {
		return c, err
	}

	for _, s := range strings.Split(*trusted, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := parsePrefix(s)
		if err != nil {
			return c, fmt.Errorf("invalid trusted proxy %q: %w", s, err)
		}
		c.TrustedProxies = append(c.TrustedProxies, p)
	}
	if *keepMinuteDays < 1 || *keepMinuteDays > 90 {
		return c, errors.New("keep-minute-days must be between 1 and 90")
	}
	if *keepHourDays < 1 || *keepHourDays > 730 {
		return c, errors.New("keep-hour-days must be between 1 and 730")
	}
	c.KeepMinute = time.Duration(*keepMinuteDays) * 24 * time.Hour
	c.KeepHour = time.Duration(*keepHourDays) * 24 * time.Hour
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return c, errors.New("tls-cert and tls-key must be set together")
	}
	if c.Listen == "" {
		return c, errors.New("listen must not be empty")
	}
	if c.AdminListen == c.Listen {
		return c, fmt.Errorf(`admin-listen equals listen; use admin-listen=%q to share it explicitly`, AdminShared)
	}
	if err := c.LogLevel.UnmarshalText([]byte(*logLevel)); err != nil {
		return c, fmt.Errorf("invalid log-level %q", *logLevel)
	}
	return c, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}
