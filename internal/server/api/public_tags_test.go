package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/server/store"
)

func TestPublicTagsValidation(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		invalid           bool
	}{
		{"empty", " , ，\n ", "", false},
		{"normalize", " alpha， 中文\nalpha, beta , ,中文", "alpha, 中文, beta", false},
		{"windows newline", "alpha\r\nbeta", "alpha, beta", false},
		{"six unique", "a,b,c,d,e,f,a", "a, b, c, d, e, f", false},
		{"unicode boundary", strings.Repeat("烽", 20), strings.Repeat("烽", 20), false},
		{"emoji boundary", strings.Repeat("🔥", 20), strings.Repeat("🔥", 20), false},
		{"too many", "a,b,c,d,e,f,g", "", true},
		{"too long", strings.Repeat("烽", 21), "", true},
		{"tab before trim", "\talpha", "", true},
		{"null", "a\x00b", "", true},
		{"delete", "a\x7fb", "", true},
		{"unicode control", "a\u0085b", "", true},
		{"bare carriage return", "a\rb", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := nodeInput{Name: "node", PublicTags: &tt.input, Asset: store.Asset{Tags: "private"}}
			err := in.validate()
			if (err != nil) != tt.invalid {
				t.Fatalf("validation: %v", err)
			}
			if !tt.invalid && (*in.PublicTags != tt.want || in.Tags != "private") {
				t.Fatalf("normalized public/private tags: %q / %q", *in.PublicTags, in.Tags)
			}
		})
	}
}

func TestPublicTagsCRUDAndPrivacy(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	resp, body := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{
		"name": "tagged", "public_tags": "公开， edge\n公开", "tags": "private-tag", "note": "private-note",
	}, nil)
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
	if made.Node.PublicTags != "公开, edge" || made.Node.Tags != "private-tag" {
		t.Fatalf("created tags: %+v", made.Node)
	}
	url := e.adm.URL + "/admin/api/nodes/" + strconv.FormatInt(made.Node.ID, 10)
	for _, tt := range []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"omitted", map[string]any{"name": "renamed", "tags": "private-tag", "note": "private-note"}, "公开, edge"},
		{"null preserves", map[string]any{"name": "renamed", "public_tags": nil, "tags": "private-tag", "note": "private-note"}, "公开, edge"},
		{"replace", map[string]any{"name": "renamed", "public_tags": " new，new\n备注 ", "tags": "private-tag", "note": "private-note"}, "new, 备注"},
		{"clear", map[string]any{"name": "renamed", "public_tags": "", "tags": "private-tag", "note": "private-note"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := e.do(t, "PUT", url, tt.input, nil)
			var updated adminNode
			if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &updated) != nil || updated.PublicTags != tt.want {
				t.Fatalf("update: %d %s", resp.StatusCode, body)
			}
			nodes, err := e.st.ListNodes(context.Background())
			if err != nil || len(nodes) != 1 || nodes[0].PublicTags != tt.want || nodes[0].Tags != "private-tag" {
				t.Fatalf("stored tags: %+v %v", nodes, err)
			}
			_, body = e.do(t, "GET", e.pub.URL+"/api/v1/nodes", nil, nil)
			var list []map[string]json.RawMessage
			if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 {
				t.Fatalf("public list: %s %v", body, err)
			}
			var tags string
			if err := json.Unmarshal(list[0]["public_tags"], &tags); err != nil || tags != tt.want {
				t.Fatalf("public tags contract: %s %v", body, err)
			}
			for _, field := range []string{"tags", "note", "last_ip", "token", "token_hash", "provider", "region"} {
				if _, ok := list[0][field]; ok {
					t.Fatalf("private field %s leaked: %s", field, body)
				}
			}
			for _, secret := range []string{"private-tag", "private-note", made.Token} {
				if strings.Contains(string(body), secret) {
					t.Fatalf("private value leaked: %s", body)
				}
			}
		})
	}
	for _, method := range []string{"POST", "PUT"} {
		target := url
		if method == "POST" {
			target = e.adm.URL + "/admin/api/nodes"
		}
		for _, invalid := range []map[string]any{
			{"name": "bad", "public_tags": "a,b,c,d,e,f,g"},
			{"name": "bad", "public_tags": "a\tb"},
			{"name": "bad", "public_tags": strings.Repeat("界", 21)},
			{"name": "bad", "public_tags": []string{"a"}},
			{"name": "bad", "public_tags": "a", "unknown": true},
			{"name": "bad", "public_tags": strings.Repeat(" ", smallBody)},
		} {
			resp, body := e.do(t, method, target, invalid, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("invalid %s: %d %s", method, resp.StatusCode, body)
			}
		}
	}
	resp, body = e.do(t, "DELETE", url, nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}
	nodes, err := e.st.ListNodes(context.Background())
	if err != nil || len(nodes) != 0 {
		t.Fatalf("deleted nodes: %+v %v", nodes, err)
	}
	resp, body = e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]any{"name": "legacy", "tags": "private-tag"}, nil)
	if resp.StatusCode != http.StatusCreated || json.Unmarshal(body, &made) != nil || made.Node.PublicTags != "" {
		t.Fatalf("private tags copied on create: %d %s", resp.StatusCode, body)
	}
}

func TestCapacityInfoVisibility(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", e.adm.URL+"/admin/api/login", map[string]string{"username": "admin", "password": "secret-password"}, nil)
	_, body := e.do(t, "POST", e.adm.URL+"/admin/api/nodes", map[string]string{"name": "capacity"}, nil)
	var made struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &made); err != nil {
		t.Fatal(err)
	}
	report := proto.Report{V: proto.Version, MemTotal: 8 << 30, DiskTotal: 100 << 30}
	resp, body := e.do(t, "POST", e.pub.URL+"/api/v1/report", report, map[string]string{"Authorization": "Bearer " + made.Token})
	if resp.StatusCode != proto.StatusIdle {
		t.Fatalf("report: %d %s", resp.StatusCode, body)
	}
	for _, enabled := range []bool{true, false} {
		settings := e.st.Settings()
		settings.PublicSystem = enabled
		if err := e.st.SaveSettings(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
		for _, endpoint := range []string{e.pub.URL + "/api/v1/nodes", e.adm.URL + "/api/v1/nodes", e.adm.URL + "/admin/api/nodes"} {
			_, body := e.do(t, "GET", endpoint, nil, nil)
			var list []publicNode
			if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 {
				t.Fatalf("nodes: %s %v", body, err)
			}
			wantMem, wantDisk := report.MemTotal, report.DiskTotal
			if !enabled && strings.HasPrefix(endpoint, e.pub.URL) {
				wantMem, wantDisk = 0, 0
			}
			if list[0].Info.MemTotal != wantMem || list[0].Info.DiskTotal != wantDisk {
				t.Fatalf("capacity visibility: %s", body)
			}
			var raw []struct {
				Info map[string]json.RawMessage `json:"info"`
			}
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"mem_total", "disk_total"} {
				if _, ok := raw[0].Info[field]; !ok {
					t.Fatalf("missing numeric field %s: %s", field, body)
				}
			}
		}
	}
}
