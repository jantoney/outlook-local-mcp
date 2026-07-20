package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// ResolveMailAliasForCapability resolves one account-scoped shared mailbox and
// authorizes the exact requested local action. Tenant compatibility and policy
// are checked before callers may construct a Graph route.
func ResolveMailAliasForCapability(entry *AccountEntry, aliasName string, capability MailCapability) (resource.MailAlias, error) {
	if entry == nil {
		return resource.MailAlias{}, fmt.Errorf("shared-mail target requires a resolved account")
	}
	for _, alias := range entry.MailAliases {
		if alias.Alias != aliasName {
			continue
		}
		if err := resource.ValidateMailCompatibility(alias, string(entry.EffectiveTokenTenantContext())); err != nil {
			return resource.MailAlias{}, fmt.Errorf("account %q mail alias %q capability %q denied: %w", entry.Label, aliasName, capability, err)
		}
		if !alias.Policy.Allows(capability) {
			return resource.MailAlias{}, fmt.Errorf("account %q mail alias %q capability %q denied by target-local policy", entry.Label, aliasName, capability)
		}
		return alias, nil
	}
	return resource.MailAlias{}, fmt.Errorf("mail alias %q not found for account %q", aliasName, entry.Label)
}
