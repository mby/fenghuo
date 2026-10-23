package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/mby/fenghuo/"

// forbiddenPackages would give the agent the ability to run commands,
// load code or expose services. The agent must stay send-only.
var forbiddenPackages = map[string]bool{
	"os/exec":        true,
	"plugin":         true,
	"net/rpc":        true,
	"net/http/cgi":   true,
	"net/http/fcgi":  true,
	"net/http/pprof": true,
	"net/smtp":       true,
}

// TestAgentDependencies is a security guard: the agent binary may only depend
// on the standard library and this module, and never on forbidden packages.
func TestAgentDependencies(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{.Standard}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, std, _ := strings.Cut(line, " ")
		if forbiddenPackages[path] {
			t.Errorf("agent depends on forbidden package %s", path)
		}
		if std != "true" && !strings.HasPrefix(path, module) {
			t.Errorf("agent depends on third-party package %s", path)
		}
	}
}

// forbiddenCalls would let the agent accept inbound connections.
var forbiddenCalls = []string{"net.Listen", "ListenAndServe", "ListenPacket", "http.Serve(", "ListenTCP", "ListenUDP", "ListenUnix"}

func TestAgentDoesNotListen(t *testing.T) {
	for _, dir := range []string{".", "../../internal/agent", "../../internal/proto"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, call := range forbiddenCalls {
				if strings.Contains(string(b), call) {
					t.Errorf("%s contains forbidden call %q", path, call)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
