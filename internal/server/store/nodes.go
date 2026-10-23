package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"
)

type Node struct {
	Asset
	ID              int64
	UUID            string
	Name            string
	TokenHash       string
	Group           string
	Sort            int
	Hidden          bool
	Note            string
	PublicNote      string
	PublicTags      string
	ExpireAt        int64
	TrafficLimit    int64
	TrafficMode     string // sum | up | down | max
	TrafficResetDay int    // 1-31; short months reset on their last day
	CreatedAt       int64
	Info            NodeInfo
}

type NodeInfo struct {
	OS           string
	Kernel       string
	Arch         string
	CPUModel     string
	CPUCores     int
	MemTotal     uint64
	DiskTotal    uint64
	Virt         string
	AgentVersion string
	LastIP       string
	UpdatedAt    int64
}

type Traffic struct {
	PeriodStart int64
	Up          uint64
	Down        uint64
	BootTime    int64
	LastRx      uint64
	LastTx      uint64
}

const nodeColumns = `n.id, n.uuid, n.name, n.token_hash, n.group_name, n.sort, n.hidden, n.note, n.public_note,
	n.expire_at, n.traffic_limit, n.traffic_mode, n.traffic_reset_day, n.created_at,
	coalesce(i.os, ''), coalesce(i.kernel, ''), coalesce(i.arch, ''), coalesce(i.cpu_model, ''),
	coalesce(i.cpu_cores, 0), coalesce(i.virt, ''), coalesce(i.agent_version, ''), coalesce(i.last_ip, ''),
	coalesce(i.updated_at, 0), n.provider, n.region, n.tags, n.price, n.currency, n.billing_months, n.auto_renew,
	n.public_tags, coalesce(i.mem_total, 0), coalesce(i.disk_total, 0)`

func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+nodeColumns+" FROM nodes n LEFT JOIN node_info i ON i.node_id = n.id ORDER BY n.sort, n.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		var hidden int
		if err := rows.Scan(&n.ID, &n.UUID, &n.Name, &n.TokenHash, &n.Group, &n.Sort, &hidden, &n.Note, &n.PublicNote,
			&n.ExpireAt, &n.TrafficLimit, &n.TrafficMode, &n.TrafficResetDay, &n.CreatedAt,
			&n.Info.OS, &n.Info.Kernel, &n.Info.Arch, &n.Info.CPUModel, &n.Info.CPUCores, &n.Info.Virt,
			&n.Info.AgentVersion, &n.Info.LastIP, &n.Info.UpdatedAt,
			&n.Provider, &n.Region, &n.Tags, &n.Price, &n.Currency, &n.BillingMonths, &n.AutoRenew,
			&n.PublicTags, &n.Info.MemTotal, &n.Info.DiskTotal); err != nil {
			return nil, err
		}
		n.Hidden = hidden != 0
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateNode inserts n and fills in ID, UUID and CreatedAt.
func (s *Store) CreateNode(ctx context.Context, n *Node) error {
	n.UUID = newUUID()
	n.CreatedAt = time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO nodes
		(uuid, name, token_hash, group_name, sort, hidden, note, public_note, expire_at, traffic_limit, traffic_mode, traffic_reset_day, created_at,
		provider, region, tags, price, currency, billing_months, auto_renew, public_tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UUID, n.Name, n.TokenHash, n.Group, n.Sort, boolInt(n.Hidden), n.Note, n.PublicNote,
		n.ExpireAt, n.TrafficLimit, n.TrafficMode, n.TrafficResetDay, n.CreatedAt,
		n.Provider, n.Region, n.Tags, n.Price, n.Currency, max(n.BillingMonths, 1), n.AutoRenew, n.PublicTags)
	if err != nil {
		return err
	}
	n.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateNode(ctx context.Context, n Node) error {
	return s.UpdateNodeWithPublicTags(ctx, n, &n.PublicTags)
}

// UpdateNodeWithPublicTags preserves public tags atomically when older clients omit them.
func (s *Store) UpdateNodeWithPublicTags(ctx context.Context, n Node, publicTags *string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE nodes SET
		name = ?, group_name = ?, sort = ?, hidden = ?, note = ?, public_note = ?, expire_at = ?,
		traffic_limit = ?, traffic_mode = ?, traffic_reset_day = ?,
		provider = ?, region = ?, tags = ?, price = ?, currency = ?, billing_months = ?, auto_renew = ?,
		public_tags = coalesce(?, public_tags) WHERE id = ?`,
		n.Name, n.Group, n.Sort, boolInt(n.Hidden), n.Note, n.PublicNote, n.ExpireAt,
		n.TrafficLimit, n.TrafficMode, n.TrafficResetDay,
		n.Provider, n.Region, n.Tags, n.Price, n.Currency, max(n.BillingMonths, 1), n.AutoRenew, publicTags, n.ID))
}

func (s *Store) SetNodeToken(ctx context.Context, id int64, tokenHash string) error {
	return affected(s.db.ExecContext(ctx, "UPDATE nodes SET token_hash = ? WHERE id = ?", tokenHash, id))
}

func (s *Store) DeleteNode(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ?", id))
}

func (s *Store) LoadTraffic(ctx context.Context) (map[int64]Traffic, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT node_id, period_start, up, down, boot_time, last_rx, last_tx FROM traffic")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Traffic{}
	for rows.Next() {
		var id int64
		var t Traffic
		if err := rows.Scan(&id, &t.PeriodStart, &t.Up, &t.Down, &t.BootTime, &t.LastRx, &t.LastTx); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
