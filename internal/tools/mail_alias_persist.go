package tools

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// replaceMailAliases persists and publishes one account's shared-mail family
// as one serialized mutation. Authentication is cleared only when the complete
// account-wide delegated scope union changes. The caller must hold
// accountPolicyMutationMu from before it reads entry until this function
// returns.
func replaceMailAliases(registry *auth.AccountRegistry, accountsPath string, entry *auth.AccountEntry, aliases []resource.MailAlias) (bool, error) {
	oldScopes := auth.ScopesForAccountEntry(entry)
	newScopes := auth.OAuthScopeUnion(
		auth.ScopesForMailPolicy(entry.MailPolicy),
		auth.ScopesForCalendarAliases(entry.CalendarAliases),
		auth.ScopesForMailAliases(aliases),
	)
	scopesChanged := !auth.OAuthScopeSetEqual(oldScopes, newScopes)
	if err := auth.SetAccountMailAliases(accountsPath, entry.Label, aliases); err != nil {
		return false, err
	}
	if err := registry.Update(entry.Label, func(current *auth.AccountEntry) {
		current.MailAliases = append([]resource.MailAlias(nil), aliases...)
		if scopesChanged {
			current.Client = nil
			current.Credential = nil
			current.Authenticator = nil
			current.Authenticated = false
			current.Scopes = nil
		}
	}); err != nil {
		return false, err
	}
	if !scopesChanged {
		return false, nil
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
		return true, fmt.Errorf("policy persisted and account disconnected, but local cleanup was incomplete: %s", strings.Join(cleanupErrors, "; "))
	}
	return true, nil
}

// mailAliasByName returns one account-local shared-mail alias and its index.
// False means the selector is absent.
func mailAliasByName(aliases []resource.MailAlias, name string) (resource.MailAlias, int, bool) {
	for index, alias := range aliases {
		if alias.Alias == name {
			return alias, index, true
		}
	}
	return resource.MailAlias{}, -1, false
}
