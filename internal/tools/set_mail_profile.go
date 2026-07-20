package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleSetMailProfile returns a handler that changes one account's capability
// profile. It clears that account's local tokens and authentication record,
// persists the new profile, and leaves the account disconnected so a later
// account.login obtains consent and tokens for the new scope set.
func HandleSetMailProfile(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		profileName, err := request.RequireString("mail_profile")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: mail_profile"), nil
		}
		profile, err := auth.ParseMailProfile(profileName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		if entry.MailProfile == profile {
			return mcp.NewToolResultText(fmt.Sprintf("Account %q already uses mail profile %s.", label, profile)), nil
		}
		// Disconnect first so no concurrent request can use the former profile
		// while persistence and local credential cleanup are in progress.
		if err := registry.Update(label, func(current *auth.AccountEntry) {
			current.MailProfile = profile
			current.Client = nil
			current.Credential = nil
			current.Authenticator = nil
			current.Authenticated = false
			current.Scopes = nil
		}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		persistErr := auth.UpsertAccountConfig(accountsPath, auth.AccountConfig{
			AccountID: entry.AccountID, Label: label, ClientID: entry.ClientID, TenantID: entry.TenantID,
			AuthMethod: entry.AuthMethod, UPN: entry.Email, MailProfile: profile.String(),
		})
		var cleanupErrors []string
		if err := auth.ClearTokenCache(entry.CacheName); err != nil {
			cleanupErrors = append(cleanupErrors, "token cache: "+err.Error())
		}
		if entry.AuthRecordPath != "" {
			if err := os.Remove(entry.AuthRecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrors = append(cleanupErrors, "authentication record: "+err.Error())
			}
		}
		if persistErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("account %q was safely disconnected and local authentication was cleared, but persisting profile %s failed: %s", label, profile, persistErr)), nil
		}
		if len(cleanupErrors) > 0 {
			return mcp.NewToolResultError(fmt.Sprintf("account %q was disconnected at profile %s, but local cleanup was incomplete: %s", label, profile, strings.Join(cleanupErrors, "; "))), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(
			"Account %q mail profile changed to %s. Local tokens were cleared and the account was disconnected; call account.login to authenticate the new scopes. Microsoft consent is not revoked automatically.",
			label, profile)), nil
	}
}
