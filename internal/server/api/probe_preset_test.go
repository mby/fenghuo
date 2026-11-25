package api

import (
	"context"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
	"strings"
	"testing"
)

func TestSelectableProbeProtocols(t *testing.T) {
	for _, tc := range []struct {
		protocols string
		valid     bool
	}{{"CT=icmp,CU=both", true}, {"CT=tcp", true}, {"", true}, {"missing=icmp", false}, {"CT=udp", false}, {"CT=tcp,CT=icmp", false}} {
		cfg := store.DefaultSettings()
		cfg.ProbeTargets = "CT=192.0.2.10:80,CU=198.51.100.10:80"
		cfg.ProbeProtocols = tc.protocols
		if (validateSettings(&cfg) == nil) != tc.valid {
			t.Fatalf("protocol preset %q", tc.protocols)
		}
	}
}

func TestICMPPresetWithoutPort(t *testing.T) {
	for _, tc := range []struct {
		targets, protocols string
		valid              bool
	}{
		{"CT=192.0.2.10", "CT=icmp", true},
		{"CT=192.0.2.10", "CT=tcp", false},
		{"CT=192.0.2.10", "CT=both", false},
		{"CT=192.0.2.10:80,", "CT=icmp", false},
		{",", "", false},
	} {
		cfg := store.DefaultSettings()
		cfg.ProbeTargets = tc.targets
		cfg.ProbeProtocols = tc.protocols
		if (validateSettings(&cfg) == nil) != tc.valid {
			t.Fatalf("preset %q %q", tc.targets, tc.protocols)
		}
	}
}

func TestValidateProbeTargets(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"disabled", "", true},
		{"one", "CT=203.0.113.10:443", true},
		{"three", "CT=203.0.113.10:443,CU=198.51.100.2:80,CM=192.0.2.3:443", true},
		{"duplicate", "CT=203.0.113.10:443,CT=198.51.100.2:80", false},
		{"custom name", "上海电信=203.0.113.10:443,北京移动=192.0.2.3:80", true},
		{"unsafe name", "name with space=203.0.113.10:443", false},
		{"too many", strings.Repeat("X=203.0.113.10:443,", 16) + "Y=203.0.113.10:443", false},
		{"host name", "CT=example.com:443", false},
		{"mapped IPv6", "CT=[::ffff:203.0.113.10]:443", false},
		{"signed port", "CT=203.0.113.10:+443", false},
		{"bad port", "CT=203.0.113.10:0", false},
		{"injection", "CT=203.0.113.10:443\nFENGHUO_TOKEN=x", false},
		{"empty entry", "CT=203.0.113.10:443,", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateProbeTargets(tc.value) == nil; got != tc.valid {
				t.Fatalf("validateProbeTargets(%q) valid=%t, want %t", tc.value, got, tc.valid)
			}
		})
	}
}

func TestHomepageProbeSelection(t *testing.T) {
	e := setup(t)
	s := &Server{Deps: Deps{Store: e.st}}
	cfg := e.st.Settings()
	cfg.ProbeTargets = "上海电信=203.0.113.1:443,上海联通=203.0.113.2:443,北京移动=203.0.113.3:443,北京电信=203.0.113.4:443"
	view := hub.View{}
	for _, name := range []string{"北京电信", "北京移动", "上海联通", "上海电信"} {
		view.Network = append(view.Network, hub.NetworkStat{Target: name, Fresh: true})
	}
	for _, tc := range []struct {
		cards string
		want  string
	}{{"", "上海电信,上海联通,北京移动"}, {"北京电信,上海电信", "北京电信,上海电信"}} {
		cfg.ProbeCards = tc.cards
		if err := e.st.SaveSettings(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		got := s.nodeForViewer(view, false)
		names := []string{}
		for _, p := range got.Network {
			names = append(names, p.Target)
		}
		if strings.Join(names, ",") != tc.want {
			t.Fatalf("got %v want %s", names, tc.want)
		}
	}
	cfg.ProbeCards = "missing"
	if err := validateSettings(&cfg); err == nil {
		t.Fatal("unknown homepage target accepted")
	}
	cfg.ProbeCards = "上海电信,上海联通,北京移动,北京电信"
	if err := validateSettings(&cfg); err == nil {
		t.Fatal("four homepage targets accepted")
	}
}
