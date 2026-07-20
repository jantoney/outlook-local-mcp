package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// SetAccountMailAliases atomically replaces one account's complete shared-mail
// alias family. Validation and alias uniqueness checks happen before the shared
// account-file mutation lock is taken; failures leave persistence unchanged.
func SetAccountMailAliases(path, label string, aliases []resource.MailAlias) error {
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if err := alias.Validate(); err != nil {
			return err
		}
		if _, exists := seen[alias.Alias]; exists {
			return fmt.Errorf("mail alias %q already exists for account %q", alias.Alias, label)
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
		copyAliases := append([]resource.MailAlias(nil), aliases...)
		accounts[index].MailAliases = &copyAliases
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}

// AccountMailAliases loads a copy of one account's shared-mail alias family.
// Missing legacy data returns an empty slice; absent accounts and I/O failures
// return an error. The function performs no mutation.
func AccountMailAliases(path, label string) ([]resource.MailAlias, error) {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if account.Label != label {
			continue
		}
		if account.MailAliases == nil {
			return []resource.MailAlias{}, nil
		}
		return append([]resource.MailAlias(nil), (*account.MailAliases)...), nil
	}
	return nil, fmt.Errorf("account %q not found in accounts file", label)
}
