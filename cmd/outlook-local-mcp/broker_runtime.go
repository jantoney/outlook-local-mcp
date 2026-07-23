package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/desek/outlook-local-mcp/internal/audit"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/instancebroker"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/desek/outlook-local-mcp/internal/observability"
	"github.com/desek/outlook-local-mcp/internal/resource"
	internalserver "github.com/desek/outlook-local-mcp/internal/server"
	"github.com/desek/outlook-local-mcp/internal/webui"
	mcpserver "github.com/mark3labs/mcp-go/server"
	_ "github.com/microsoft/kiota-abstractions-go"
	_ "github.com/microsoft/kiota-authentication-azure-go"
	_ "github.com/microsoftgraph/msgraph-sdk-go-core"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// runBroker wins deterministic election before initializing mutable state,
// then owns the account registry, Graph clients, web UI, telemetry, and MCP
// server until all proxy leases expire or the process is signaled.
func runBroker(parent context.Context, cfg config.Config, identity instancebroker.Identity) error {
	listener, endpoint, err := instancebroker.Claim(identity)
	if err != nil {
		// Another candidate normally wins simultaneous startup. Proxies verify
		// its authenticated descriptor; this loser must never initialize state.
		return nil
	}
	defer listener.Close()
	defer instancebroker.RemoveEndpoint(identity.DescriptorPath(), endpoint)

	logging.InitLogger(cfg.LogLevel, cfg.LogFormat, cfg.LogSanitize, cfg.LogFile)
	defer logging.CloseLogFile()
	audit.InitAuditLog(cfg.AuditLogEnabled, cfg.AuditLogPath)
	defer func() {
		if closeErr := audit.CloseAuditLog(); closeErr != nil {
			slog.Warn("audit log close failed", "error", closeErr)
		}
	}()
	slog.Info("authoritative broker starting", "version", version, "pid", os.Getpid(), "read_only", cfg.ReadOnly, "web_ui_enabled", cfg.WebUIEnabled)

	shutdownOTEL, err := observability.InitOTEL(cfg)
	if err != nil {
		return fmt.Errorf("initialize OpenTelemetry: %w", err)
	}
	defer shutdownTelemetry(shutdownOTEL)
	meter := otel.Meter(cfg.OTELServiceName)
	tracer := otel.Tracer(cfg.OTELServiceName)
	metrics, err := observability.InitMetrics(meter)
	if err != nil {
		return fmt.Errorf("initialize metrics: %w", err)
	}

	cfg.TokenCacheBackend = auth.ResolveTokenCacheBackend(cfg.CacheName, cfg.TokenStorage)
	authRecordDir := auth.AuthRecordDir(cfg.AuthRecordPath)
	if err := auth.MigrateExplicitAccountPermissions(cfg.AccountsPath, cfg.CacheName, authRecordDir); err != nil {
		return fmt.Errorf("migrate account permissions: %w", err)
	}
	registry := auth.NewAccountRegistry()
	restored, total := auth.RestoreExplicitAccounts(cfg.AccountsPath, cfg.CacheName, authRecordDir, registry, auth.SetupCredentialForAccount, cfg.TokenStorage)
	slog.Info("accounts loaded", "restored", restored, "total", total, "accounts", registry.Count())

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	admin := accountadmin.New(ctx, cfg, registry)
	webListener, webServer, webURL := startWebUI(&cfg, admin, metrics, tracer)
	if webListener != nil {
		defer webListener.Close()
	}

	mcp := mcpserver.NewMCPServer("outlook-local", version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithResourceCapabilities(false, false),
		mcpserver.WithRecovery(),
		mcpserver.WithLogging(),
		mcpserver.WithElicitation(),
	)
	retryCfg := graph.RetryConfig{MaxRetries: cfg.MaxRetries, InitialBackoff: time.Duration(cfg.RetryBackoffMS) * time.Millisecond, Logger: slog.Default()}
	referenceKey, err := resource.LoadOrCreateSigningKey(filepath.Join(filepath.Dir(cfg.AccountsPath), "resource-reference.key"))
	if err != nil {
		return fmt.Errorf("initialize resource reference signing key: %w", err)
	}
	referenceCodec := resource.NewReferenceCodec(referenceKey)
	passThroughAuth := func(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return next }
	leases := instancebroker.NewLeases(30 * time.Second)
	cfg.BrokerRole = "broker"
	cfg.BrokerInstanceFingerprint = identity.Key
	cfg.BrokerStateFingerprint = identity.StateKey
	cfg.BrokerPID = endpoint.PID
	cfg.BrokerClientCount = func() int { return leases.Count(time.Now()) }
	cfg.BrokerUIOwner = webServer != nil
	catalog := internalserver.RegisterToolsWithAdminAndReplayCatalog(mcp, retryCfg, cfg.RequestTimeout, metrics, tracer, cfg.ReadOnly, passThroughAuth, registry, cfg, nil, admin, &referenceCodec)
	internalserver.RegisterResources(mcp)

	streamable := mcpserver.NewStreamableHTTPServer(mcp,
		mcpserver.WithStateful(true),
		mcpserver.WithSessionIdleTTL(2*time.Minute),
	)
	handler := instancebroker.NewHandler(endpoint, identity, leases, catalog.Sorted(), streamable)

	done := make(chan struct{})
	internalserver.AwaitShutdownSignal(cancel, cfg.ShutdownTimeout, done, shutdownOTEL)
	if webServer != nil {
		go serveWebUI(webServer, webListener, webURL)
	}
	admin.ValidateConnectedAccounts(ctx)
	err = instancebroker.Serve(ctx, listener, handler, leases, cfg.ShutdownTimeout)
	cancel()
	if webServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		if shutdownErr := webServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Warn("web UI shutdown incomplete", "error", shutdownErr)
		}
		shutdownCancel()
	}
	close(done)
	slog.Info("authoritative broker stopped")
	return err
}

// startWebUI binds and constructs the optional broker-owned management UI.
// Listener or construction failures are non-fatal and recorded in cfg logs.
func startWebUI(cfg *config.Config, admin *accountadmin.Module, metrics *observability.ToolMetrics, tracer trace.Tracer) (net.Listener, *webui.Server, string) {
	if !cfg.WebUIEnabled {
		return nil, nil, ""
	}
	listener, webURL, err := webui.Listen(cfg.WebUIPort)
	if err != nil {
		cfg.WebUIError = err.Error()
		slog.Error("web UI disabled after listener failure", "error", err)
		return nil, nil, ""
	}
	cfg.WebUIURL = webURL
	webServer, err := webui.NewObserved(*cfg, admin, webURL, metrics, tracer)
	if err != nil {
		cfg.WebUIError = err.Error()
		cfg.WebUIURL = ""
		_ = listener.Close()
		slog.Error("web UI disabled after initialization failure", "error", err)
		return nil, nil, ""
	}
	return listener, webServer, webURL
}

// serveWebUI runs the already initialized optional management adapter.
func serveWebUI(webServer *webui.Server, listener net.Listener, webURL string) {
	slog.Info("web UI available", "url", webURL)
	if err := webServer.Serve(listener); err != nil {
		slog.Error("web UI transport stopped", "error", err)
	}
}

// shutdownTelemetry flushes the broker's telemetry with a bounded timeout.
func shutdownTelemetry(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		slog.Error("OpenTelemetry shutdown failed", "error", err)
	}
}
