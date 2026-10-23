// Package agent implements the reporting loop. The agent only ever sends
// data; it never reads instructions or configuration from the server.
package agent

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"

	"github.com/mby/fenghuo/internal/proto"
)

type Target struct {
	Name string
	Addr string // host:port
}

type Config struct {
	Server       string
	Token        string
	Interval     time.Duration
	IdleInterval time.Duration // 0 disables adaptive reporting
	NICs         []string
	Ping         []Target
	PingInterval time.Duration
	AllowHTTP    bool
}

func (c Config) Validate() error {
	if c.Server == "" {
		return errors.New("server URL is required (-server or FENGHUO_SERVER)")
	}
	if c.Token == "" {
		return errors.New("token is required (FENGHUO_TOKEN)")
	}
	if c.Interval < time.Second || c.Interval > time.Minute {
		return errors.New("interval must be between 1s and 60s")
	}
	if c.IdleInterval != 0 && (c.IdleInterval < c.Interval || c.IdleInterval > 10*time.Minute) {
		return errors.New("idle interval must be 0 (off) or between the interval and 10m")
	}
	if len(c.Ping) > 0 && (c.PingInterval < 10*time.Second || c.PingInterval > 10*time.Minute) {
		return errors.New("ping interval must be between 10s and 10m")
	}
	return nil
}

// ParseTargets parses "name=host:port,name2=host:port".
func ParseTargets(s string) ([]Target, error) {
	var out []Target
	seen := map[string]bool{}
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, addr, ok := strings.Cut(item, "=")
		name, addr = strings.TrimSpace(name), strings.TrimSpace(addr)
		if !ok || name == "" || addr == "" {
			return nil, fmt.Errorf("invalid ping target %q, want name=host:port", item)
		}
		if len(name) > proto.MaxPingNameLen || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("invalid ping target name %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate ping target name %q", name)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return nil, fmt.Errorf("invalid ping target address %q, want host:port", addr)
		}
		seen[name] = true
		out = append(out, Target{Name: name, Addr: addr})
	}
	if len(out) > proto.MaxPingTargets {
		return nil, fmt.Errorf("at most %d ping targets are allowed", proto.MaxPingTargets)
	}
	return out, nil
}

func SplitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
