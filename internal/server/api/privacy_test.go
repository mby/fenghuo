package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestPublicPrivacyAndInstallPreset(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	in := nodeInput{Name: "public", ExpireAt: 2000000000, TrafficLimit: 1024, Asset: store.Asset{Provider: "secret-provider", Region: "secret-region", Tags: "private-tag", Price: 120, Currency: "USD", BillingMonths: 12}}
	resp, body := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", in, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var made struct {
		Node  adminNode `json:"node"`
		Token string    `json:"token"`
	}
	if err := json.Unmarshal(body, &made); err != nil {
		t.Fatal(err)
	}
	report := proto.Report{V: proto.Version, Info: &proto.Info{OS: "private-os", Kernel: "private-kernel", AgentVersion: "private-version"}, Ping: []proto.Ping{{Name: "CT", MS: 18}}}
	e.do(t, "POST", e.pub.URL+"/api/v1/report", report, map[string]string{"Authorization": "Bearer " + made.Token})
	defaults := e.st.Settings()
	if defaults.AgentIdleInterval != 15 || defaults.PublicSystem || defaults.PublicExpiry {
		t.Fatalf("unsafe defaults: %+v", defaults)
	}
	for _, enabled := range []bool{false, true} {
		st := defaults
		st.PublicSystem, st.PublicExpiry, st.PublicTraffic, st.PublicPing = enabled, enabled, enabled, enabled
		st.AgentIdleInterval = 60
		st.ProbeTargets = "CT=203.0.113.10:443"
		resp, body = e.do(t, "PUT", e.adm.URL+"/admin/api/settings", st, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("settings: %d %s", resp.StatusCode, body)
		}
		_, body = e.do(t, "GET", e.pub.URL+"/api/v1/nodes", nil, nil)
		var list []publicNode
		if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 {
			t.Fatalf("nodes: %s %v", body, err)
		}
		if strings.Contains(string(body), "secret-provider") || strings.Contains(string(body), "private-tag") || strings.Contains(string(body), made.Token) || strings.Contains(string(body), "203.0.113.10") {
			t.Fatalf("asset/credential leak: %s", body)
		}
		if enabled {
			if list[0].Info.Kernel != "private-kernel" || list[0].ExpireAt != in.ExpireAt || list[0].Traffic.Limit != in.TrafficLimit || len(list[0].Network) != 1 || list[0].Network[0].Target != "CT" {
				t.Fatalf("visible fields: %s", body)
			}
			probe := list[0].Network[0]
			if probe.Samples != 1 || probe.Successes != 1 || len(probe.History) != 1 || probe.History[0].AvgMS != 18 {
				t.Fatalf("probe history: %+v", probe)
			}
		} else if list[0].Info.Kernel != "" || list[0].ExpireAt != 0 || list[0].Traffic.Limit != 0 || list[0].Visibility.Ping || len(list[0].Network) != 0 {
			t.Fatalf("privacy: %s", body)
		}
		resp, _ = e.do(t, "GET", e.pub.URL+"/api/v1/nodes/"+made.Node.UUID+"/ping", nil, nil)
		want := http.StatusNotFound
		if enabled {
			want = http.StatusOK
		}
		if resp.StatusCode != want {
			t.Fatalf("ping privacy: %d want %d", resp.StatusCode, want)
		}
		_, body = e.do(t, "GET", e.adm.URL+"/api/v1/nodes", nil, nil)
		if !strings.Contains(string(body), "private-kernel") {
			t.Fatalf("admin lost access: %s", body)
		}
		_, body = e.do(t, "GET", e.adm.URL+"/admin/api/nodes", nil, nil)
		if !strings.Contains(string(body), "secret-provider") {
			t.Fatalf("admin asset: %s", body)
		}
	}
	for _, seconds := range []int{3, 15, 30, 60, 7, -1} {
		st := defaults
		st.AgentIdleInterval = seconds
		resp, _ = e.do(t, "PUT", e.adm.URL+"/admin/api/settings", st, nil)
		valid := seconds == 3 || seconds == 15 || seconds == 30 || seconds == 60
		if (resp.StatusCode == 200) != valid {
			t.Fatalf("interval %d status %d", seconds, resp.StatusCode)
		}
	}
}

func TestAssetValidation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		asset store.Asset
		valid bool
	}{
		{"defaults", store.Asset{}, true},
		{"annual", store.Asset{Price: 12.345, Currency: " usd ", BillingMonths: 12}, true},
		{"negative", store.Asset{Price: -1}, false},
		{"nan", store.Asset{Price: math.NaN()}, false},
		{"inf", store.Asset{Price: math.Inf(1)}, false},
		{"currency", store.Asset{Currency: "USD;"}, false},
		{"billing", store.Asset{BillingMonths: 2}, false},
		{"long provider", store.Asset{Provider: strings.Repeat("x", 65)}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.asset
			err := validateAsset(&a)
			if (err == nil) != tt.valid {
				t.Fatalf("validation: %v", err)
			}
			if tt.name == "annual" && (a.Currency != "USD" || a.Price != 12.35) {
				t.Fatalf("normalization: %+v", a)
			}
		})
	}
}

func TestReportedStateSurvivesPrivacyAndRestart(t *testing.T) {
	e := setup(t)
	s := New(Deps{Store: e.st})
	for _, tt := range []struct {
		name    string
		updated int64
		want    bool
	}{{"never seen", 0, false}, {"known but not reconnected", 1, true}} {
		t.Run(tt.name, func(t *testing.T) {
			n := s.nodeForViewer(hub.View{Info: store.NodeInfo{UpdatedAt: tt.updated, AgentVersion: "private-version"}}, false)
			if n.HasReported != tt.want || n.Info.AgentVersion != "" || n.Online {
				t.Fatalf("redacted restart status: %+v", n)
			}
		})
	}
}
