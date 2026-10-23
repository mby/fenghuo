package notify

import (
	"encoding/json"
	"net/netip"
	"net/url"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	ok := []struct{ typ, cfg string }{
		{"telegram", `{"bot_token":"123456:ABCdefGhIJKlmNoPQRstuVWXyz0123456789","chat_id":"-100123"}`},
		{"webhook", `{"url":"https://api.day.app/key/{{title}}/{{text}}","method":"get"}`},
		{"webhook", `{"url":"https://example.com/hook","headers":{"Authorization":"Bearer x"},"body":"{\"msg\":\"{{text}}\"}"}`},
	}
	for _, c := range ok {
		if _, err := ValidateConfig(c.typ, json.RawMessage(c.cfg)); err != nil {
			t.Errorf("%s %s: %v", c.typ, c.cfg, err)
		}
	}
	bad := []struct{ typ, cfg string }{
		{"telegram", `{"bot_token":"nope","chat_id":"1"}`},
		{"telegram", `{"bot_token":"123456:ABCdefGhIJKlmNoPQRstuVWXyz0123456789","chat_id":""}`},
		{"webhook", `{"url":"ftp://example.com"}`},
		{"webhook", `{"url":"https://example.com","method":"DELETE"}`},
		{"webhook", `{"url":"https://example.com","extra":1}`},
		{"email", `{}`},
	}
	for _, c := range bad {
		if _, err := ValidateConfig(c.typ, json.RawMessage(c.cfg)); err == nil {
			t.Errorf("%s %s: expected error", c.typ, c.cfg)
		}
	}
}

func TestRender(t *testing.T) {
	vars := map[string]string{"title": `a "b"`, "text": "x/y z"}
	if got := render(`{"t":"{{title}}","u":"{{ unknown }}"}`, vars, jsonEscape); got != `{"t":"a \"b\"","u":"{{ unknown }}"}` {
		t.Fatalf("json render: %s", got)
	}
	if got := render("https://h/{{text}}", vars, url.PathEscape); got != "https://h/x%2Fy%20z" {
		t.Fatalf("url render: %s", got)
	}
}

func TestIsPublic(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "::ffff:127.0.0.1", "0.0.0.0"} {
		if isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s should not be public", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "2606:4700::1111"} {
		if !isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s should be public", s)
		}
	}
}
