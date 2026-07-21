package tools

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// accountPolicyMutationMu serializes account lifecycle, own-mail policy,
// calendar-alias, and mail-alias read-modify-persist-publish transactions in
// this process. Sharing the lock with account add/remove prevents a stale
// transaction from publishing into a recreated account that reused a label.
var accountPolicyMutationMu sync.Mutex

// replaceCalendarAliases persists and publishes one account's calendar alias
// family as one serialized mutation. It disconnects and clears local auth only
// when the account-wide OAuth scope union changes. The caller must hold
// accountPolicyMutationMu from before it reads entry until this function
// returns.
func replaceCalendarAliases(registry *auth.AccountRegistry, accountsPath string, entry *auth.AccountEntry, aliases []resource.CalendarAlias) (bool, error) {
	oldScopes := auth.ScopesForAccountEntry(entry)
	newScopes := auth.OAuthScopeUnion(
		auth.ScopesForMailPolicy(entry.MailPolicy),
		auth.ScopesForCalendarAliases(aliases),
		auth.ScopesForMailAliases(entry.MailAliases),
	)
	scopesChanged := !auth.OAuthScopeSetEqual(oldScopes, newScopes)
	if err := auth.SetAccountCalendarAliases(accountsPath, entry.Label, aliases); err != nil {
		return false, err
	}
	if err := registry.Update(entry.Label, func(current *auth.AccountEntry) {
		current.CalendarAliases = append([]resource.CalendarAlias(nil), aliases...)
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

// calendarAliasByName returns one alias and its index in an account-local
// family. The returned false value means no alias matched.
func calendarAliasByName(aliases []resource.CalendarAlias, name string) (resource.CalendarAlias, int, bool) {
	for index, alias := range aliases {
		if alias.Alias == name {
			return alias, index, true
		}
	}
	return resource.CalendarAlias{}, -1, false
}
