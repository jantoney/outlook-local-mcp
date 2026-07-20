package auth

// ScopesForMailPolicy returns the least broad own-resource delegated scopes
// required by policy. The result is newly allocated and does not authorize an
// action; callers must still enforce the exact target capability locally.
func ScopesForMailPolicy(policy MailActionPolicy) []string {
	scopes := []string{userReadScope, calendarScope}
	if policy.Send {
		return OAuthScopeUnion(scopes, []string{mailReadWriteScope, mailSendScope})
	}
	if policy.Draft || policy.Move || policy.Archive || policy.Trash ||
		policy.Restore || policy.PermanentDelete {
		return OAuthScopeUnion(scopes, []string{mailReadWriteScope})
	}
	if policy.Read {
		return OAuthScopeUnion(scopes, []string{mailScope})
	}
	return OAuthScopeUnion(scopes)
}
