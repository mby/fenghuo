// Package version holds the build version, injected at link time via
// -ldflags "-X github.com/mby/fenghuo/internal/version.Version=v1.2.3".
package version

var Version = "dev"
