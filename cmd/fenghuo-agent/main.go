// Command fenghuo-agent collects host metrics and reports them to a fenghuo
// server over HTTPS. It is send-only: it listens on no port, executes no
// commands and ignores everything the server returns besides the status code.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mby/fenghuo/internal/agent"
	"github.com/mby/fenghuo/internal/agent/collect"
	"github.com/mby/fenghuo/internal/flagenv"
	"github.com/mby/fenghuo/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fenghuo-agent:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("fenghuo-agent", flag.ExitOnError)
	server := fs.String("server", "", "server base URL, e.g. https://probe.example.com")
	token := fs.String("token", "", "node token; prefer the FENGHUO_TOKEN environment variable because flags are visible in the process list")
	interval := fs.Duration("interval", 3*time.Second, "report interval while someone is viewing the dashboard (1s-60s)")
	idleInterval := fs.Duration("idle-interval", 15*time.Second, "report interval while nobody is viewing (0 = always use -interval)")
	nic := fs.String("nic", "", "comma-separated interfaces counted for traffic (default: auto-detect physical interfaces)")
	ping := fs.String("ping", "", "TCP latency targets, e.g. \"CT=example.com:80,CU=1.2.3.4:443\"")
	pingInterval := fs.Duration("ping-interval", 30*time.Second, "TCP latency measurement interval (10s-10m)")
	allowHTTP := fs.Bool("allow-http", false, "allow plain http to a non-loopback server (sends the token unencrypted)")
	printOnce := fs.Bool("print", false, "collect once, print the report as JSON and exit")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: fenghuo-agent [flags]\n\nEvery flag can also be set with FENGHUO_<NAME>, e.g. FENGHUO_SERVER.\n\n")
		fs.PrintDefaults()
	}
	if err := flagenv.Parse(fs, os.Args[1:]); err != nil {
		return err
	}

	if *showVersion {
		fmt.Println("fenghuo-agent", version.Version)
		return nil
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if flagenv.Explicit(fs, "token") {
		log.Warn("the -token flag is visible to other users in the process list; use FENGHUO_TOKEN instead")
	}

	nics := agent.SplitList(*nic)
	if *printOnce {
		return printReport(nics)
	}

	targets, err := agent.ParseTargets(*ping)
	if err != nil {
		return err
	}
	cfg := agent.Config{
		Server:       *server,
		Token:        *token,
		Interval:     *interval,
		IdleInterval: *idleInterval,
		NICs:         nics,
		Ping:         targets,
		PingInterval: *pingInterval,
		AllowHTTP:    *allowHTTP,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agent.Run(ctx, cfg, log)
}

func printReport(nics []string) error {
	col, err := collect.New(nics)
	if err != nil {
		return err
	}
	time.Sleep(time.Second)
	rep, err := col.Collect(time.Now())
	if err != nil {
		return err
	}
	info := col.Info()
	rep.Info = &info
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
