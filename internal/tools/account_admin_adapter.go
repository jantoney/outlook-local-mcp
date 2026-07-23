package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleAdminLogin starts the process-bound authentication session shared with
// the web UI. Browser and device-code flows progress asynchronously; auth_code
// returns a URL and is completed through account.complete_auth.
func HandleAdminLogin(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		session, err := admin.StartAuthentication(label)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.Marshal(session)
		return mcp.NewToolResultText(string(data) + "\nUse account.auth_status with this session_id until complete. For awaiting_code, open auth_url and call account.complete_auth with the full redirect URL."), nil
	}
}

// HandleAdminAuthStatus returns one secret-free process-bound auth session.
func HandleAdminAuthStatus(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("session_id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: session_id"), nil
		}
		session, ok := admin.Session(id)
		if !ok {
			return mcp.NewToolResultError("authentication session not found or expired"), nil
		}
		data, _ := json.Marshal(session)
		return mcp.NewToolResultText(string(data)), nil
	}
}

// HandleAdminCompleteAuth completes an auth_code session with a redirect URL.
func HandleAdminCompleteAuth(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("session_id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: session_id"), nil
		}
		redirectURL, err := request.RequireString("redirect_url")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: redirect_url"), nil
		}
		session, err := admin.CompleteAuthentication(id, redirectURL)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Account %q authenticated for the current required scopes.", session.Label)), nil
	}
}

// HandleAdminCancelAuth cancels an active process-bound auth session.
func HandleAdminCancelAuth(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("session_id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: session_id"), nil
		}
		if !admin.CancelAuthentication(id) {
			return mcp.NewToolResultError("authentication session not found"), nil
		}
		return mcp.NewToolResultText("Authentication session cancelled."), nil
	}
}

// HandleConfigureAccount adapts account.add to the shared administration
// module. It creates a disconnected account with explicit initial permissions;
// callers then invoke account.login to authenticate the deterministic union.
func HandleConfigureAccount(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		calendar, err := auth.ParseCalendarPolicy(request.GetString("calendar_policy", "off"))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		mail, _, err := mailPolicyFromArguments(auth.MailActionPolicy{}, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		account, err := admin.CreateAccount(accountadmin.CreateRequest{
			Label: label, ClientID: request.GetString("client_id", ""),
			TenantID: request.GetString("tenant_id", ""), AuthMethod: request.GetString("auth_method", ""),
			CalendarPolicy: calendar, MailPolicy: mail,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Account configured: %s\nAuthentication: required\nRequired scopes: %v\nCall account.login with label %q to authenticate.", account.Label, account.RequiredScopes, account.Label)), nil
	}
}

// HandleAdminSetPermissions adapts account.set_permissions to the shared
// module. Omitted mail switches are treated as false because the operation is
// an exact replacement, while calendar_policy is required.
func HandleAdminSetPermissions(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		calendarValue, err := request.RequireString("calendar_policy")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: calendar_policy"), nil
		}
		calendar, err := auth.ParseCalendarPolicy(calendarValue)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		mail, _, err := mailPolicyFromArguments(auth.MailActionPolicy{}, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		account, err := admin.SetPermissions(label, accountadmin.PermissionUpdate{Calendar: calendar, Mail: mail})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		message := fmt.Sprintf("Account %q permissions saved immediately.", label)
		if account.ReauthenticationRequired {
			message += " Re-authentication is required for the new scope union; call account.login."
		}
		return mcp.NewToolResultText(message), nil
	}
}

// HandleAdminSetMailPolicy preserves partial-update semantics while routing the
// mutation through the shared module and retaining the account's calendar policy.
func HandleAdminSetMailPolicy(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		current, ok := admin.Account(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		mail, supplied, err := mailPolicyFromArguments(current.MailPolicy, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !supplied {
			return mcp.NewToolResultError("at least one mail capability must be supplied"), nil
		}
		updated, err := admin.SetPermissions(label, accountadmin.PermissionUpdate{Calendar: current.CalendarPolicy, Mail: mail})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		message := fmt.Sprintf("Account %q own-mail policy saved immediately.", label)
		if updated.ReauthenticationRequired {
			message += " Re-authentication is required; call account.login."
		}
		return mcp.NewToolResultText(message), nil
	}
}

// HandleAdminRemove adapts account removal to the shared module.
func HandleAdminRemove(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		if err := admin.Remove(label); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Account removed: %s\nToken cache cleared.", label)), nil
	}
}

// HandleAdminLogout adapts account disconnection to the shared module.
func HandleAdminLogout(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		if err := admin.Logout(label); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Account %q disconnected. Configuration preserved.", label)), nil
	}
}
