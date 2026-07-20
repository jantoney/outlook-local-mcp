package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

// mailPolicyMutationMu serializes persistence and runtime registry changes so
// one successful policy mutation becomes visible as a single ordered update.
var mailPolicyMutationMu sync.Mutex

// HandleSetMailPolicy returns a handler that partially updates one account's
// independent own-mail action policy. It persists before runtime mutation,
// takes effect on the next request, and disconnects only when OAuth scopes
// change. Cleanup failures are reported after the account is safely denied.
//
// Parameters:
//   - registry: runtime account store updated after persistence succeeds.
//   - accountsPath: filesystem path of the persisted accounts file.
//
// Returns a local-only MCP handler. Errors are represented as tool results.
func HandleSetMailPolicy(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		mailPolicyMutationMu.Lock()
		defer mailPolicyMutationMu.Unlock()

		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		policy, supplied, err := mailPolicyFromArguments(entry.MailPolicy, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !supplied {
			return mcp.NewToolResultError("at least one mail capability must be supplied"), nil
		}
		if policy == entry.MailPolicy {
			return mcp.NewToolResultText(fmt.Sprintf("Account %q own-mail policy is unchanged.", label)), nil
		}
		if err := auth.SetAccountMailPolicy(accountsPath, label, policy); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("persist account %q mail policy: %s", label, err)), nil
		}

		calendarScopes := auth.ScopesForCalendarAliases(entry.CalendarAliases)
		oldScopes := auth.OAuthScopeUnion(auth.ScopesForMailPolicy(entry.MailPolicy), calendarScopes)
		newScopes := auth.OAuthScopeUnion(auth.ScopesForMailPolicy(policy), calendarScopes)
		scopesChanged := !auth.OAuthScopeSetEqual(oldScopes, newScopes)
		if err := registry.Update(label, func(current *auth.AccountEntry) {
			current.MailPolicy = policy
			current.MailProfile = auth.LegacyProfileForPolicy(policy)
			if scopesChanged {
				current.Client = nil
				current.Credential = nil
				current.Authenticator = nil
				current.Authenticated = false
				current.Scopes = nil
			}
		}); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("account %q policy was persisted but runtime update failed: %s", label, err)), nil
		}
		if !scopesChanged {
			return mcp.NewToolResultText(fmt.Sprintf("Account %q own-mail policy updated immediately; OAuth scopes are unchanged.", label)), nil
		}

		var cleanupErrors []string
		if err := auth.ClearTokenCache(entry.CacheName); err != nil {
			cleanupErrors = append(cleanupErrors, "token cache: "+err.Error())
		}
		if entry.AuthRecordPath != "" {
			if err := os.Remove(entry.AuthRecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrors = append(cleanupErrors, "authentication record: "+err.Error())
			}
		}
		if len(cleanupErrors) > 0 {
			return mcp.NewToolResultError(fmt.Sprintf("account %q policy was updated and disconnected, but local cleanup was incomplete: %s", label, strings.Join(cleanupErrors, "; "))), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Account %q own-mail policy updated. OAuth scopes changed, so local tokens were cleared; call account.login to reconnect.", label)), nil
	}
}
