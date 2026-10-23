package api

import (
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
	"strings"
)

type infoDTO struct {
	OS           string `json:"os"`
	Kernel       string `json:"kernel"`
	Arch         string `json:"arch"`
	CPUModel     string `json:"cpu_model"`
	CPUCores     int    `json:"cpu_cores"`
	MemTotal     uint64 `json:"mem_total"`
	DiskTotal    uint64 `json:"disk_total"`
	Virt         string `json:"virt"`
	AgentVersion string `json:"agent_version"`
}

type statusDTO struct {
	CPU       float64 `json:"cpu"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
	MemTotal  uint64  `json:"mem_total"`
	MemUsed   uint64  `json:"mem_used"`
	SwapTotal uint64  `json:"swap_total"`
	SwapUsed  uint64  `json:"swap_used"`
	DiskTotal uint64  `json:"disk_total"`
	DiskUsed  uint64  `json:"disk_used"`
	NetIn     uint64  `json:"net_in"`
	NetOut    uint64  `json:"net_out"`
	TCP       uint32  `json:"tcp"`
	UDP       uint32  `json:"udp"`
	Procs     uint32  `json:"procs"`
	Uptime    uint64  `json:"uptime"`
}

type trafficDTO struct {
	Up             uint64 `json:"up"`
	Down           uint64 `json:"down"`
	Used           uint64 `json:"used"`
	Limit          int64  `json:"limit"`
	Mode           string `json:"mode"`
	ResetDay       int    `json:"reset_day"`
	PeriodStart    int64  `json:"period_start"`
	NextResetAt    int64  `json:"next_reset_at"`
	ResetUTCOffset int    `json:"reset_utc_offset"`
}

// publicNode never contains IP addresses, tokens or private notes.
type publicNode struct {
	HasReported bool          `json:"has_reported"`
	Visibility  visibilityDTO `json:"visibility"`
	UUID        string        `json:"uuid"`
	Name        string        `json:"name"`
	Group       string        `json:"group"`
	PublicNote  string        `json:"public_note"`
	PublicTags  string        `json:"public_tags"`
	ExpireAt    int64         `json:"expire_at"`
	Online      bool          `json:"online"`
	LastSeen    int64         `json:"last_seen"`
	Info        infoDTO       `json:"info"`
	Status      *statusDTO    `json:"status"`
	Network     []networkDTO  `json:"network"`
	Traffic     trafficDTO    `json:"traffic"`
}

type networkDTO struct {
	Target       string             `json:"target"`
	AvgMS        float64            `json:"avg_ms"`
	FailureRate  float64            `json:"failure_rate"`
	Fresh        bool               `json:"fresh"`
	LatencyFresh bool               `json:"latency_fresh"`
	Samples      int                `json:"samples"`
	Successes    int                `json:"successes"`
	History      []networkBucketDTO `json:"history"`
}

type networkBucketDTO struct {
	T         int64   `json:"t"`
	AvgMS     float64 `json:"avg_ms"`
	Samples   int     `json:"samples"`
	Successes int     `json:"successes"`
}

type adminNode struct {
	publicNode
	store.Asset
	ID              int64  `json:"id"`
	Sort            int    `json:"sort"`
	Hidden          bool   `json:"hidden"`
	Note            string `json:"note"`
	LastIP          string `json:"last_ip"`
	TrafficLimit    int64  `json:"traffic_limit"`
	TrafficMode     string `json:"traffic_mode"`
	TrafficResetDay int    `json:"traffic_reset_day"`
	CreatedAt       int64  `json:"created_at"`
}

func toPublic(v hub.View) publicNode {
	n := publicNode{
		Network:     []networkDTO{},
		HasReported: !v.LastSeen.IsZero() || v.Info.UpdatedAt > 0,
		Visibility:  visibilityDTO{true, true, true, true},
		UUID:        v.Node.UUID, Name: v.Node.Name, Group: v.Node.Group, PublicNote: v.Node.PublicNote,
		PublicTags: v.Node.PublicTags, ExpireAt: v.Node.ExpireAt, Online: v.Online,
		Info: infoDTO{
			OS: v.Info.OS, Kernel: v.Info.Kernel, Arch: v.Info.Arch, CPUModel: v.Info.CPUModel,
			CPUCores: v.Info.CPUCores, Virt: v.Info.Virt, AgentVersion: v.Info.AgentVersion,
			MemTotal: v.Info.MemTotal, DiskTotal: v.Info.DiskTotal,
		},
		Traffic: trafficDTO{
			Up: v.Traffic.Up, Down: v.Traffic.Down, Used: hub.TrafficUsed(v.Traffic, v.Node.TrafficMode),
			Limit: v.Node.TrafficLimit, Mode: v.Node.TrafficMode, ResetDay: v.Node.TrafficResetDay,
			PeriodStart: v.Traffic.PeriodStart,
			NextResetAt: v.NextResetAt, ResetUTCOffset: v.ResetUTCOffset,
		},
	}
	for _, stat := range v.Network {
		if stat.Fresh {
			history := make([]networkBucketDTO, 0, len(stat.History))
			for _, b := range stat.History {
				history = append(history, networkBucketDTO{T: b.T, AvgMS: b.AvgMS, Samples: b.Samples, Successes: b.Successes})
			}
			n.Network = append(n.Network, networkDTO{
				Target: stat.Target, AvgMS: stat.AvgMS, FailureRate: stat.FailureRate,
				Fresh: stat.Fresh, LatencyFresh: stat.LatencyFresh,
				Samples: stat.Count, Successes: stat.SuccessCount, History: history,
			})
		}
	}
	if !v.LastSeen.IsZero() {
		n.LastSeen = v.LastSeen.Unix()
	}
	if r := v.Status; r != nil {
		n.Status = &statusDTO{
			CPU: r.CPU, Load1: r.Load1, Load5: r.Load5, Load15: r.Load15,
			MemTotal: r.MemTotal, MemUsed: r.MemUsed, SwapTotal: r.SwapTotal, SwapUsed: r.SwapUsed,
			DiskTotal: r.DiskTotal, DiskUsed: r.DiskUsed, NetIn: r.NetIn, NetOut: r.NetOut,
			TCP: r.TCP, UDP: r.UDP, Procs: r.Procs, Uptime: r.Uptime,
		}
	}
	return n
}

func toAdmin(v hub.View) adminNode {
	return adminNode{
		Asset:      v.Node.Asset,
		publicNode: toPublic(v),
		ID:         v.Node.ID, Sort: v.Node.Sort, Hidden: v.Node.Hidden, Note: v.Node.Note,
		LastIP: v.Info.LastIP, TrafficLimit: v.Node.TrafficLimit, TrafficMode: v.Node.TrafficMode,
		TrafficResetDay: v.Node.TrafficResetDay, CreatedAt: v.Node.CreatedAt,
	}
}

type visibilityDTO struct {
	System  bool `json:"system"`
	Expiry  bool `json:"expiry"`
	Traffic bool `json:"traffic"`
	Ping    bool `json:"ping"`
}

func (s *Server) nodeForViewer(v hub.View, admin bool) publicNode {
	n := toPublic(v)
	cfg := s.Store.Settings()
	selected := []networkDTO{}
	if cfg.ProbeCards != "" {
		for _, name := range strings.Split(cfg.ProbeCards, ",") {
			for _, probe := range n.Network {
				if probe.Target == name {
					selected = append(selected, probe)
					break
				}
			}
		}
	} else {
		// Prefer preset order; locally configured targets still work without a preset.
		entries := strings.Split(cfg.ProbeTargets, ",")
		if len(entries) > 3 {
			entries = entries[:3]
		}
		for _, entry := range entries {
			name, _, _ := strings.Cut(entry, "=")
			for _, probe := range n.Network {
				if probe.Target == name {
					selected = append(selected, probe)
					break
				}
			}
		}
		if cfg.ProbeTargets == "" {
			selected = n.Network
		}
	}
	if len(selected) > 3 {
		selected = selected[:3]
	}
	n.Network = selected
	if admin {
		return n
	}
	n.Visibility = visibilityDTO{cfg.PublicSystem, cfg.PublicExpiry, cfg.PublicTraffic, cfg.PublicPing}
	if !cfg.PublicPing {
		n.Network = []networkDTO{}
	}
	if !cfg.PublicSystem {
		n.Info = infoDTO{}
	}
	if !cfg.PublicExpiry {
		n.ExpireAt = 0
	}
	if !cfg.PublicTraffic {
		n.Traffic = trafficDTO{Mode: "sum", ResetDay: 1}
	}
	return n
}
