package auth

// PublishEmail stores a lazily resolved address only when the current account
// still has the expected immutable identity. It prevents a stale lookup from
// updating a removed and recreated label.
func (r *AccountRegistry) PublishEmail(label string, accountID AccountID, email string) error {
	if email == "" {
		return nil
	}
	return r.Update(label, func(entry *AccountEntry) {
		if entry.AccountID == accountID && entry.Email == "" {
			entry.Email = email
		}
	})
}
