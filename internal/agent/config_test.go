package agent

import "testing"

func TestParseTargets(t *testing.T) {
	got, err := ParseTargets(" CT=ct.example.com:80, CU=[2001:db8::1]:443 ,")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "CT" || got[1].Addr != "[2001:db8::1]:443" {
		t.Fatalf("got %+v", got)
	}
	for _, bad := range []string{"CT", "CT=host", "=host:80", "CT=a:1,CT=b:2", "CT=:80"} {
		if _, err := ParseTargets(bad); err == nil {
			t.Errorf("ParseTargets(%q) should fail", bad)
		}
	}
}

func TestNewClientRejectsPlainHTTP(t *testing.T) {
	if _, err := NewClient("http://probe.example.com", "t", false); err == nil {
		t.Fatal("plain http to a remote host must be rejected")
	}
	if _, err := NewClient("http://127.0.0.1:8080", "t", false); err != nil {
		t.Fatalf("loopback http should be allowed: %v", err)
	}
	c, err := NewClient("https://probe.example.com/base/?x=1", "t", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.endpoint != "https://probe.example.com/base/api/v1/report" {
		t.Fatalf("endpoint = %s", c.endpoint)
	}
}
