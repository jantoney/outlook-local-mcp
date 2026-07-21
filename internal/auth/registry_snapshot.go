package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

// snapshotAccountEntry returns a detached account value for use after the
// registry lock is released. Pointer collaborators are immutable handles;
// every mutable slice receives independent backing storage.
func snapshotAccountEntry(entry *AccountEntry) *AccountEntry {
	if entry == nil {
		return nil
	}
	return &AccountEntry{
		AccountID:          entry.AccountID,
		Label:              entry.Label,
		ClientID:           entry.ClientID,
		TenantID:           entry.TenantID,
		TokenTenantContext: entry.TokenTenantContext,
		AuthMethod:         entry.AuthMethod,
		MailProfile:        entry.MailProfile,
		MailPolicy:         entry.MailPolicy,
		CalendarAliases:    append([]resource.CalendarAlias(nil), entry.CalendarAliases...),
		MailAliases:        append([]resource.MailAlias(nil), entry.MailAliases...),
		Scopes:             append([]string(nil), entry.Scopes...),
		Credential:         entry.Credential,
		Authenticator:      entry.Authenticator,
		Client:             entry.Client,
		AuthRecordPath:     entry.AuthRecordPath,
		CacheName:          entry.CacheName,
		Authenticated:      entry.Authenticated,
		Email:              entry.Email,
	}
}
