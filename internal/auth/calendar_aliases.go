package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// SetAccountCalendarAliases atomically replaces one account's complete
// calendar-alias family after validating every immutable record and enforcing
// account-local alias uniqueness. It returns an error without writing when the
// account is absent or any record is invalid.
func SetAccountCalendarAliases(path, label string, aliases []resource.CalendarAlias) error {
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if err := alias.Validate(); err != nil {
			return err
		}
		if _, exists := seen[alias.Alias]; exists {
			return fmt.Errorf("calendar alias %q already exists for account %q", alias.Alias, label)
		}
		seen[alias.Alias] = struct{}{}
	}

	accountsMutationMu.Lock()
	defer accountsMutationMu.Unlock()

	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for index := range accounts {
		if accounts[index].Label != label {
			continue
		}
		copyAliases := append([]resource.CalendarAlias(nil), aliases...)
		accounts[index].CalendarAliases = &copyAliases
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}

// AccountCalendarAliases loads a copy of one account's calendar alias family.
// It returns an error when persistence fails or the account does not exist and
// performs no mutation.
func AccountCalendarAliases(path, label string) ([]resource.CalendarAlias, error) {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if account.Label == label {
			if account.CalendarAliases == nil {
				return []resource.CalendarAlias{}, nil
			}
			return append([]resource.CalendarAlias(nil), (*account.CalendarAliases)...), nil
		}
	}
	return nil, fmt.Errorf("account %q not found in accounts file", label)
}
