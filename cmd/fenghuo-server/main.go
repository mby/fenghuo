// Command fenghuo-server receives agent reports and serves the dashboard.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mby/fenghuo/internal/server"
	"github.com/mby/fenghuo/internal/server/config"
	"github.com/mby/fenghuo/internal/version"
)

const usage = `Usage: fenghuo-server [command] [flags]

Commands:
  serve            start the server (default)
  reset-password   set a new random admin password and sign out all sessions
  disable-totp     turn off two-factor authentication for the admin
  version          print the version

Run "fenghuo-server serve -h" to list flags.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fenghuo-server:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "version":
		fmt.Println("fenghuo-server", version.Version)
		return nil
	case "help":
		fmt.Print(usage)
		return nil
	}

	cfg, err := config.Parse("fenghuo-server "+cmd, args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "serve":
		log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
		return server.Run(ctx, cfg, log)
	case "reset-password":
		pw, err := server.ResetPassword(ctx, cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Username: admin\nNew password: %s\nNo restart is required.\n", pw)
		return nil
	case "disable-totp":
		if err := server.DisableTOTP(ctx, cfg); err != nil {
			return err
		}
		fmt.Println("Two-factor authentication disabled.")
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}
