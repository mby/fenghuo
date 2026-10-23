package api

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mby/fenghuo/internal/server/alert"
	"github.com/mby/fenghuo/internal/server/store"
)

func checkText(field, v string, minLen, maxLen int) error {
	n := utf8.RuneCountInString(v)
	if n < minLen || n > maxLen {
		return fmt.Errorf("%s must be %d-%d characters", field, minLen, maxLen)
	}
	if strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
		return fmt.Errorf("%s contains control characters", field)
	}
	return nil
}

type nodeInput struct {
	store.Asset
	Name            string  `json:"name"`
	Group           string  `json:"group"`
	Sort            int     `json:"sort"`
	Hidden          bool    `json:"hidden"`
	Note            string  `json:"note"`
	PublicNote      string  `json:"public_note"`
	PublicTags      *string `json:"public_tags"`
	ExpireAt        int64   `json:"expire_at"`
	TrafficLimit    int64   `json:"traffic_limit"`
	TrafficMode     string  `json:"traffic_mode"`
	TrafficResetDay int     `json:"traffic_reset_day"`
}

func (in *nodeInput) validate() error {
	if in.PublicTags != nil {
		tags, err := normalizePublicTags(*in.PublicTags)
		if err != nil {
			return err
		}
		in.PublicTags = &tags
	}
	if err := validateAsset(&in.Asset); err != nil {
		return err
	}
	in.Name, in.Group = strings.TrimSpace(in.Name), strings.TrimSpace(in.Group)
	if err := checkText("name", in.Name, 1, 64); err != nil {
		return err
	}
	if err := checkText("group", in.Group, 0, 32); err != nil {
		return err
	}
	if err := checkText("note", in.Note, 0, 1000); err != nil {
		return err
	}
	if err := checkText("public_note", in.PublicNote, 0, 200); err != nil {
		return err
	}
	if in.Sort < -100000 || in.Sort > 100000 {
		return errors.New("sort out of range")
	}
	if in.ExpireAt < 0 || in.TrafficLimit < 0 {
		return errors.New("expire_at and traffic_limit must not be negative")
	}
	if in.TrafficMode == "" {
		in.TrafficMode = "sum"
	}
	if !slices.Contains([]string{"sum", "up", "down", "max"}, in.TrafficMode) {
		return errors.New("traffic_mode must be sum, up, down or max")
	}
	if in.TrafficResetDay == 0 {
		in.TrafficResetDay = 1
	}
	if in.TrafficResetDay < 1 || in.TrafficResetDay > 31 {
		return errors.New("traffic_reset_day must be 1-31")
	}
	return nil
}

func (in nodeInput) apply(n *store.Node) {
	if in.PublicTags != nil {
		n.PublicTags = *in.PublicTags
	}
	n.Asset = in.Asset
	n.Name, n.Group, n.Sort, n.Hidden = in.Name, in.Group, in.Sort, in.Hidden
	n.Note, n.PublicNote, n.ExpireAt = in.Note, in.PublicNote, in.ExpireAt
	n.TrafficLimit, n.TrafficMode, n.TrafficResetDay = in.TrafficLimit, in.TrafficMode, in.TrafficResetDay
}

func normalizePublicTags(value string) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
		return "", errors.New("public_tags contains control characters")
	}
	var tags []string
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == '\n' }) {
		tag = strings.TrimSpace(tag)
		if tag == "" || slices.Contains(tags, tag) {
			continue
		}
		if utf8.RuneCountInString(tag) > 20 {
			return "", errors.New("public_tags entries must be at most 20 characters")
		}
		tags = append(tags, tag)
		if len(tags) > 6 {
			return "", errors.New("public_tags must contain at most 6 tags")
		}
	}
	return strings.Join(tags, ", "), nil
}

func validateRule(r *store.Rule) error {
	r.Target = strings.TrimSpace(r.Target)
	if err := checkText("target", r.Target, 0, 32); err != nil {
		return err
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) {
		return errors.New("threshold must be finite")
	}
	r.Name = strings.TrimSpace(r.Name)
	if err := checkText("name", r.Name, 1, 64); err != nil {
		return err
	}
	if !slices.Contains(alert.Kinds, r.Kind) {
		return fmt.Errorf("kind must be one of %s", strings.Join(alert.Kinds, ", "))
	}
	if r.Duration < 0 || r.Duration > 7*86400 {
		return errors.New("duration must be 0-604800 seconds")
	}
	switch r.Kind {
	case "latency":
		if r.Threshold <= 0 || r.Threshold > 60000 {
			return errors.New("latency threshold must be 0-60000 ms")
		}
	case "packet_loss":
		if r.Threshold <= 0 || r.Threshold > 100 {
			return errors.New("failure threshold must be in (0, 100]")
		}
	case alert.KindOffline:
		if r.Duration < 30 {
			return errors.New("offline duration must be at least 30 seconds")
		}
	case alert.KindCPU, alert.KindMem, alert.KindDisk, alert.KindTraffic:
		if r.Threshold <= 0 || r.Threshold > 100 {
			return errors.New("threshold must be a percentage in (0, 100]")
		}
	case alert.KindExpire:
		if r.Threshold < 0 || r.Threshold > 365 {
			return errors.New("threshold must be 0-365 days")
		}
	}
	if len(r.NodeIDs) > 1000 {
		return errors.New("too many node ids")
	}
	return nil
}

func validateSettings(v *store.Settings) error {
	if v.AgentIdleInterval == 0 {
		v.AgentIdleInterval = 15
	}
	if !slices.Contains([]int{3, 15, 30, 60}, v.AgentIdleInterval) {
		return errors.New("agent_idle_interval must be 3, 15, 30 or 60 seconds")
	}
	if v.ProbeInterval == 0 {
		v.ProbeInterval = 30
	}
	if !slices.Contains([]int{15, 30, 60}, v.ProbeInterval) {
		return errors.New("probe_interval must be 15, 30 or 60 seconds")
	}
	if err := validateProbeTargets(v.ProbeTargets); err != nil {
		return err
	}
	if v.ProbeCards != "" {
		selected := strings.Split(v.ProbeCards, ",")
		if len(selected) > 3 {
			return errors.New("select at most three homepage probe targets")
		}
		seen := map[string]bool{}
		for _, name := range selected {
			if seen[name] || !strings.Contains(","+v.ProbeTargets, ","+name+"=") {
				return errors.New("homepage probe names must be unique configured targets")
			}
			seen[name] = true
		}
	}
	v.SiteName = strings.TrimSpace(v.SiteName)
	v.PublicURL = strings.TrimRight(strings.TrimSpace(v.PublicURL), "/")
	if err := checkText("site_name", v.SiteName, 1, 64); err != nil {
		return err
	}
	if v.PublicURL != "" {
		u, err := url.Parse(v.PublicURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("public_url must be an http(s) URL without query")
		}
	}
	if v.Lang != "en" && v.Lang != "zh" {
		return errors.New("lang must be en or zh")
	}
	return nil
}

func validateProbeTargets(raw string) error {
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	if len(strings.Split(raw, ",")) > 16 {
		return errors.New("at most 16 probe targets are allowed")
	}
	for _, entry := range strings.Split(raw, ",") {
		name, address, ok := strings.Cut(entry, "=")
		if !ok || len(name) == 0 || len(name) > 32 || seen[name] || strings.ContainsFunc(name, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !strings.ContainsRune("_-.", r)
		}) {
			return errors.New("probe names must be unique, 1-32 bytes, using letters, numbers, underscores, hyphens or dots")
		}
		host, portText, err := net.SplitHostPort(address)
		port, portErr := strconv.Atoi(portText)
		if err != nil || portErr != nil || strings.Contains(host, ":") || strings.Trim(portText, "0123456789") != "" || net.ParseIP(host).To4() == nil || port < 1 || port > 65535 {
			return errors.New("probe_targets must use IPv4:port with port 1-65535")
		}
		seen[name] = true
	}
	return nil
}
