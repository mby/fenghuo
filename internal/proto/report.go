// Package proto defines the only message that ever travels between an agent
// and the server: the agent's periodic report. There is intentionally no
// message type flowing from the server to the agent.
package proto

import (
	"errors"
	"fmt"
	"math"
	"unicode"
	"unicode/utf8"
)

// Version is the report protocol version. Bump only on breaking changes.
const Version = 1

// The server answers a report with one of two success codes. This single bit
// is the only thing the agent ever learns from the server: it switches between
// its two locally configured intervals and nothing else.
const (
	StatusWatched = 204 // someone is viewing the dashboard: use the normal interval
	StatusIdle    = 202 // nobody is viewing: the idle interval may be used
)

const (
	MaxBodyBytes   = 32 << 10
	MaxPingTargets = 16
	MaxTextLen     = 128
	MaxPingNameLen = 32

	// Interval limits in milliseconds.
	MinInterval     = 1_000
	MaxInterval     = 60_000
	MaxIdleInterval = 600_000
)

type Report struct {
	V        int    `json:"v"`
	Time     int64  `json:"t"`    // agent clock, unix milliseconds
	BootTime int64  `json:"boot"` // unix seconds
	Uptime   uint64 `json:"uptime"`

	// The agent's own intervals in milliseconds, so the server knows when to
	// expect the next report. IdleInterval is 0 when adaptive reporting is off.
	Interval     int64 `json:"interval"`
	IdleInterval int64 `json:"idle_interval"`

	CPU    float64 `json:"cpu"` // percent, 0-100
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	MemTotal  uint64 `json:"mem_total"`
	MemUsed   uint64 `json:"mem_used"`
	SwapTotal uint64 `json:"swap_total"`
	SwapUsed  uint64 `json:"swap_used"`
	DiskTotal uint64 `json:"disk_total"`
	DiskUsed  uint64 `json:"disk_used"`

	NetRx  uint64 `json:"net_rx"`  // cumulative bytes since boot
	NetTx  uint64 `json:"net_tx"`  // cumulative bytes since boot
	NetIn  uint64 `json:"net_in"`  // bytes per second
	NetOut uint64 `json:"net_out"` // bytes per second

	TCP   uint32 `json:"tcp"`
	UDP   uint32 `json:"udp"`
	Procs uint32 `json:"procs"`

	// Info is static host information, sent on start and then periodically.
	Info *Info `json:"info,omitempty"`
	// Ping holds the latest TCP latency round, only when a new round completed.
	Ping []Ping `json:"ping,omitempty"`
}

type Info struct {
	OS           string `json:"os"`
	Kernel       string `json:"kernel"`
	Arch         string `json:"arch"`
	CPUModel     string `json:"cpu_model"`
	CPUCores     int    `json:"cpu_cores"`
	Virt         string `json:"virt"`
	AgentVersion string `json:"agent_version"`
}

type Ping struct {
	Name string `json:"name"`
	// MS is the TCP connect latency in milliseconds, or -1 when the attempt failed.
	MS float64 `json:"ms"`
}

func (r *Report) Validate() error {
	if r.V != Version {
		return fmt.Errorf("unsupported protocol version %d", r.V)
	}
	if err := inRange("cpu", r.CPU, 0, 100); err != nil {
		return err
	}
	for _, l := range []struct {
		name string
		v    float64
	}{{"load1", r.Load1}, {"load5", r.Load5}, {"load15", r.Load15}} {
		if err := inRange(l.name, l.v, 0, 1e6); err != nil {
			return err
		}
	}
	if r.MemUsed > r.MemTotal {
		return errors.New("mem_used exceeds mem_total")
	}
	if r.SwapUsed > r.SwapTotal {
		return errors.New("swap_used exceeds swap_total")
	}
	if r.DiskUsed > r.DiskTotal {
		return errors.New("disk_used exceeds disk_total")
	}
	if r.BootTime < 0 {
		return errors.New("negative boot time")
	}
	if r.Interval != 0 && (r.Interval < MinInterval || r.Interval > MaxInterval) {
		return errors.New("interval out of range")
	}
	if r.IdleInterval != 0 && (r.IdleInterval < max(r.Interval, MinInterval) || r.IdleInterval > MaxIdleInterval) {
		return errors.New("idle_interval out of range")
	}
	if r.Info != nil {
		if err := r.Info.validate(); err != nil {
			return err
		}
	}
	if len(r.Ping) > MaxPingTargets {
		return fmt.Errorf("too many ping targets (max %d)", MaxPingTargets)
	}
	for _, p := range r.Ping {
		if err := validText("ping name", p.Name, MaxPingNameLen); err != nil {
			return err
		}
		if p.Name == "" {
			return errors.New("empty ping name")
		}
		if p.MS != -1 {
			if err := inRange("ping ms", p.MS, 0, 60000); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *Info) validate() error {
	for _, f := range []struct{ name, v string }{
		{"os", i.OS}, {"kernel", i.Kernel}, {"arch", i.Arch}, {"cpu_model", i.CPUModel},
		{"virt", i.Virt}, {"agent_version", i.AgentVersion},
	} {
		if err := validText(f.name, f.v, MaxTextLen); err != nil {
			return err
		}
	}
	if i.CPUCores < 0 || i.CPUCores > 65536 {
		return errors.New("cpu_cores out of range")
	}
	return nil
}

func inRange(name string, v, lo, hi float64) error {
	if math.IsNaN(v) || v < lo || v > hi {
		return fmt.Errorf("%s out of range", name)
	}
	return nil
}

func validText(name, s string, max int) error {
	if len(s) > max {
		return fmt.Errorf("%s too long", name)
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}
