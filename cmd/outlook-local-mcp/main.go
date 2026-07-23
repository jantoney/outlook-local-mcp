// Package main is the executable entry point for the Outlook Local MCP broker
// and its lightweight harness-facing stdio proxies.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/instancebroker"
)

// version is injected at build time with -ldflags and defaults to dev.
var version = "dev"

// main loads public configuration, derives the isolation identity, then runs
// either the private authoritative broker child or a lightweight stdio proxy.
// It exits nonzero on invalid configuration, startup, or transport failure.
func main() {
	cfg := config.LoadConfig()
	var err error
	cfg, err = config.ApplyCLIOverrides(cfg, os.Args[1:])
	if err != nil {
		slog.Error("command-line configuration failed", "error", err)
		os.Exit(1)
	}
	cfg.Version = version
	if err := config.ValidateConfig(cfg); err != nil {
		slog.Error("configuration validation failed", "error", err)
		os.Exit(1)
	}
	executable, err := os.Executable()
	if err != nil {
		slog.Error("resolve executable path failed", "error", err)
		os.Exit(1)
	}
	identity, err := instancebroker.NewIdentity(executable, cfg)
	if err != nil {
		slog.Error("derive broker identity failed", "error", err)
		os.Exit(1)
	}
	if instancebroker.IsBrokerProcess() {
		if err := runBroker(context.Background(), cfg, identity); err != nil {
			slog.Error("broker stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	input, output := instancebroker.DefaultIO()
	proxy, err := instancebroker.NewProxy(identity, executable, os.Args[1:], input, output)
	if err != nil {
		slog.Error("create MCP proxy failed", "error", err)
		os.Exit(1)
	}
	if err := proxy.Run(context.Background()); err != nil {
		slog.Error("MCP proxy stopped", "error", err)
		os.Exit(1)
	}
}
