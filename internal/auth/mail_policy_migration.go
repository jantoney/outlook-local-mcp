package auth

// MigrateAccountMailPolicies rewrites legacy cumulative profiles as independent
// own-mail policies. Known profiles map deterministically; empty or malformed
// legacy values use defaultProfile to preserve the former startup behavior.
// Records already using the new representation are unchanged.
//
// Parameters:
//   - path: filesystem path of the persisted accounts file.
//   - defaultProfile: legacy default used when a record has no valid profile.
//
// Returns an error when accounts cannot be loaded or atomically persisted. Its
// only side effect is one accounts-file rewrite when migration is required.
func MigrateAccountMailPolicies(path string, defaultProfile MailProfile) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	changed := false
	for index := range accounts {
		if accounts[index].MailPolicy != nil {
			continue
		}
		profile := defaultProfile
		if parsed, parseErr := ParseMailProfile(accounts[index].MailProfile); parseErr == nil {
			profile = parsed
		}
		policy := MailPolicyFromProfile(profile)
		accounts[index].MailPolicy = &policy
		accounts[index].MailProfile = ""
		changed = true
	}
	if !changed {
		return nil
	}
	return SaveAccounts(path, accounts)
}
