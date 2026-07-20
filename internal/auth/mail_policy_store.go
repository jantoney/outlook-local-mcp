package auth

import "fmt"

// SetAccountMailPolicy atomically persists one account's independent own-mail
// policy using the new representation and removes its legacy profile field.
// Concurrent calls in this process are serialized around the complete
// read-modify-write operation.
//
// Parameters:
//   - path: filesystem path of the persisted accounts file.
//   - label: account selector whose policy is replaced.
//   - policy: complete replacement own-mail action policy.
//
// Returns an error when the account is absent or persistence fails. Its only
// side effect on success is an atomic accounts-file rewrite.
func SetAccountMailPolicy(path string, label string, policy MailActionPolicy) error {
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
		policyCopy := policy
		accounts[index].MailPolicy = &policyCopy
		accounts[index].MailProfile = ""
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}
