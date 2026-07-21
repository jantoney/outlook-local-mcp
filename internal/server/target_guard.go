package server

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TargetGuardConfig declares the exact resource family, capability, and signed
// reference contract required by one target-aware verb.
type TargetGuardConfig struct {
	// Family selects the separate calendar or mail alias namespace.
	Family resource.TargetFamily
	// Capability is the exact target-local action required by the verb.
	Capability resource.TargetCapability
	// AllowedKinds restricts the verb to exact resource kinds after alias
	// resolution and before route selection. Empty accepts every family kind.
	AllowedKinds []resource.ResourceKind
	// ReferenceArgument names the optional signed-reference input. Empty uses
	// `resource_ref` when RequireReference is true and otherwise ignores refs.
	ReferenceArgument string
	// RequireReference rejects calls without a signed target-bound reference.
	RequireReference bool
	// RequireSharedReference requires provenance only when shared_resource is
	// selected, preserving own-resource raw-ID compatibility.
	RequireSharedReference bool
	// RawIDArgument names an own-resource raw-ID input that shared calls reject.
	RawIDArgument string
	// ItemKind is the exact signed item kind consumed by the verb.
	ItemKind resource.ItemKind
}

// ResolvedTargetGuard returns middleware that resolves and authorizes one
// immutable target, validates optional signed provenance, selects one typed SDK
// root, records audit evidence, and only then publishes target context. It makes
// no Graph request itself.
func ResolvedTargetGuard(registry *auth.AccountRegistry, config TargetGuardConfig, codec *resource.ReferenceCodec, handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if registry == nil {
			return mcp.NewToolResultError("resolved target requires an account registry"), nil
		}
		accountInfo, ok := auth.AccountInfoFromContext(ctx)
		if !ok || accountInfo.Label == "" {
			return mcp.NewToolResultError("resolved target requires a selected account"), nil
		}
		sharedResource := request.GetString("shared_resource", "")
		requireReference := config.RequireReference || (config.RequireSharedReference && sharedResource != "")
		referenceArgument := config.ReferenceArgument
		if referenceArgument == "" && requireReference {
			referenceArgument = "resource_ref"
		}
		if sharedResource != "" && config.RawIDArgument != "" && request.GetString(config.RawIDArgument, "") != "" {
			return mcp.NewToolResultError(fmt.Sprintf("shared resource follow-up requires %s and does not accept %s", referenceArgument, config.RawIDArgument)), nil
		}
		reference := ""
		if referenceArgument != "" {
			reference = request.GetString(referenceArgument, "")
		}
		targetRequest := auth.TargetRequest{
			Family: config.Family, SharedResource: sharedResource,
			Capability: config.Capability, Reference: reference,
			RequireReference: requireReference, ItemKind: config.ItemKind,
		}
		resolved, client, err := registry.ResolveRegisteredTarget(accountInfo.Label, targetRequest, codec)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !targetKindAllowed(resolved.Target.Kind, config.AllowedKinds) {
			return mcp.NewToolResultError(fmt.Sprintf("resource kind %q is not supported by this operation", resolved.Target.Kind)), nil
		}
		root, err := graph.SelectUserRoot(client, resolved.Target)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		auth.RecordAuditTarget(ctx, auth.AuditTarget{
			Alias: resolved.Target.Alias, ResourceID: resolved.Target.ResourceID,
			ResourceKind: resolved.Target.Kind, MailboxView: resolved.Target.View,
			MailPolicy: resolved.Target.MailPolicy, Compatibility: "compatible",
		})
		ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{
			Target: resolved.Target, Root: root, Claims: resolved.Claims,
			Reauthorize: targetReauthorizer(registry, accountInfo.Label, client, targetRequest, codec, resolved.Target),
		})
		return handler(ctx, request)
	}
}

// targetReauthorizer returns a request-local callback that proves current
// authority and route identity again before a later committed Graph stage.
func targetReauthorizer(registry *auth.AccountRegistry, label string, originalClient *msgraphsdk.GraphServiceClient, request auth.TargetRequest, codec *resource.ReferenceCodec, target resource.Target) func() error {
	return func() error {
		resolved, currentClient, err := registry.ResolveRegisteredTarget(label, request, codec)
		if err != nil {
			return fmt.Errorf("target authority changed before dispatch: %w", err)
		}
		if currentClient != originalClient {
			return fmt.Errorf("account %q connection changed before dispatch", label)
		}
		if resolved.Target != target {
			return fmt.Errorf("target route changed before dispatch")
		}
		return nil
	}
}

// targetKindAllowed reports whether kind is in the operation allowlist. An
// empty allowlist preserves family-wide behavior for existing guard users.
func targetKindAllowed(kind resource.ResourceKind, allowed []resource.ResourceKind) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if kind == candidate {
			return true
		}
	}
	return false
}
