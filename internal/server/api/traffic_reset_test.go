package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mby/fenghuo/internal/server/hub"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestTrafficResetDayValidation(t *testing.T) {
	for _, day := range []int{-1, 0, 1, 28, 29, 30, 31, 32} {
		t.Run(strconv.Itoa(day), func(t *testing.T) {
			in := nodeInput{Name: "node", TrafficResetDay: day}
			err := in.validate()
			if (err == nil) != (day >= 0 && day <= 31) {
				t.Fatalf("day %d: %v", day, err)
			}
			if day == 0 && in.TrafficResetDay != 1 {
				t.Fatal("default reset day changed")
			}
		})
	}
}

func TestTrafficResetDayCRUD(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	for _, day := range []int{29, 30, 31} {
		t.Run(strconv.Itoa(day), func(t *testing.T) {
			resp, body := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{"name": "node", "traffic_reset_day": day}, nil)
			var made struct {
				Node adminNode `json:"node"`
			}
			if resp.StatusCode != http.StatusCreated || json.Unmarshal(body, &made) != nil || made.Node.TrafficResetDay != day {
				t.Fatalf("create: %d %s", resp.StatusCode, body)
			}
			url := e.adm.URL + "/admin/api/nodes/" + strconv.FormatInt(made.Node.ID, 10)
			for _, update := range []int{29, 30, 31, 32, -1} {
				resp, body = e.do(t, "PUT", url, map[string]any{"name": "node", "traffic_reset_day": update}, nil)
				if update < 1 || update > 31 {
					if resp.StatusCode != http.StatusBadRequest {
						t.Fatalf("invalid update: %d %s", resp.StatusCode, body)
					}
					continue
				}
				var got adminNode
				if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &got) != nil || got.Traffic.ResetDay != update || got.Traffic.NextResetAt <= 0 {
					t.Fatalf("update: %d %s", resp.StatusCode, body)
				}
			}
			nodes, err := e.st.ListNodes(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range nodes {
				if n.ID == made.Node.ID && n.TrafficResetDay != 31 {
					t.Fatalf("stored reset day: %d", n.TrafficResetDay)
				}
			}
		})
	}
}

func TestTrafficResetDTOPrivacy(t *testing.T) {
	e := setup(t)
	s := New(Deps{Store: e.st})
	now := time.Date(2026, 2, 28, 0, 0, 0, 0, time.FixedZone("server", 5*3600+45*60))
	h := hub.New(now)
	h.Reload([]store.Node{{ID: 1, TrafficResetDay: 31}}, nil, now)
	v := h.Snapshot(now)[0]
	wantNext := time.Date(2026, 3, 31, 0, 0, 0, 0, now.Location()).Unix()
	for _, public := range []bool{true, false} {
		settings := e.st.Settings()
		settings.PublicTraffic = public
		if err := e.st.SaveSettings(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
		for _, admin := range []bool{false, true} {
			n := s.nodeForViewer(v, admin)
			wantTime, wantOffset := wantNext, 5*3600+45*60
			if !public && !admin {
				wantTime, wantOffset = 0, 0
			}
			if n.Traffic.NextResetAt != wantTime || n.Traffic.ResetUTCOffset != wantOffset {
				t.Fatalf("public=%v admin=%v traffic=%+v", public, admin, n.Traffic)
			}
			body, err := json.Marshal(n)
			if err != nil {
				t.Fatal(err)
			}
			var raw struct {
				Traffic map[string]json.RawMessage `json:"traffic"`
			}
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatal(err)
			}
			for field, want := range map[string]int64{"next_reset_at": wantTime, "reset_utc_offset": int64(wantOffset)} {
				var value int64
				if err := json.Unmarshal(raw.Traffic[field], &value); err != nil || value != want {
					t.Fatalf("%s=%s: %v", field, raw.Traffic[field], err)
				}
			}
		}
	}
	if admin := toAdmin(v); admin.Traffic.NextResetAt != wantNext || admin.Traffic.ResetUTCOffset != 20700 {
		t.Fatalf("admin DTO: %+v", admin.Traffic)
	}
}
