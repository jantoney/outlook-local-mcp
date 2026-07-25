package auth

// ScopesForMailPolicy returns the least broad own-resource delegated scopes
// required by policy. The result is newly allocated and does not authorize an
// action; callers must still enforce the exact target capability locally.
func ScopesForMailPolicy(policy MailActionPolicy) []string {
	scopes := []string{}
	if policy.Send {
		scopes = OAuthScopeUnion(scopes, []string{mailReadWriteScope, mailSendScope})
	} else if policy.Draft || policy.Move || policy.Archive || policy.Trash ||
		policy.Restore || policy.PermanentDelete {
		scopes = OAuthScopeUnion(scopes, []string{mailReadWriteScope})
	} else if policy.Read {
		scopes = OAuthScopeUnion(scopes, []string{mailScope})
	}
	if policy.Read {
		scopes = OAuthScopeUnion(scopes, []string{mailboxSettingsReadScope})
	}
	return OAuthScopeUnion(scopes)
}
