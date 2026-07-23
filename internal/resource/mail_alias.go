package resource

import "fmt"

// MailAlias is one account-scoped shared mailbox target. ResourceID, Owner,
// Kind, and View are immutable after creation; Alias and Policy are mutable
// only through explicit lifecycle operations.
type MailAlias struct {
	// ResourceID is the immutable local provenance identity.
	ResourceID ResourceID `json:"resource_id"`
	// Alias is the mutable human-facing account-scoped selector.
	Alias string `json:"alias"`
	// Owner is the immutable Graph-compatible mailbox owner locator.
	Owner string `json:"owner"`
	// Kind fixes this target as a shared owner-view mailbox.
	Kind ResourceKind `json:"kind"`
	// View fixes every returned identifier to the owner's mailbox view.
	View MailboxView `json:"view"`
	// Policy is the target-local independent action matrix.
	Policy MailActionPolicy `json:"policy"`

	// Validation is the latest direct mailbox metadata reachability result.
	Validation Validation `json:"validation"`
}

// NewMailAlias creates one owner-view mailbox alias with every action disabled.
// It returns an error for invalid identity, alias, or owner fields and performs
// no persistence or Graph traffic.
func NewMailAlias(id ResourceID, alias, owner string) (MailAlias, error) {
	mailAlias := MailAlias{
		ResourceID: id,
		Alias:      alias,
		Owner:      owner,
		Kind:       ResourceKindMailbox,
		View:       MailboxViewOwner,
		Policy:     MailActionPolicy{},
	}
	if err := mailAlias.Validate(); err != nil {
		return MailAlias{}, err
	}
	return mailAlias, nil
}

// Validate verifies the immutable owner-view mailbox identity and mutable
// selector. The policy needs no structural validation because all fields are
// independent booleans. It returns an error without side effects on failure.
func (a MailAlias) Validate() error {
	if err := a.ResourceID.Validate(); err != nil {
		return err
	}
	if !aliasPattern.MatchString(a.Alias) {
		return fmt.Errorf("invalid mail alias %q: must match %s", a.Alias, aliasPattern.String())
	}
	if a.Owner == "" {
		return fmt.Errorf("mail alias owner must not be empty")
	}
	if a.Kind != ResourceKindMailbox || a.View != MailboxViewOwner {
		return fmt.Errorf("mail aliases require mailbox kind and owner view")
	}
	return nil
}

// Rename returns a copy with a new selector while preserving immutable target
// identity, owner, kind, view, and policy. Invalid selectors return an error.
func (a MailAlias) Rename(alias string) (MailAlias, error) {
	a.Alias = alias
	if err := a.Validate(); err != nil {
		return MailAlias{}, err
	}
	return a, nil
}

// WithPolicy returns a copy with an exact replacement action matrix while
// preserving immutable target identity and routing fields.
func (a MailAlias) WithPolicy(policy MailActionPolicy) MailAlias {
	a.Policy = policy
	return a
}

// ValidateMailCompatibility rejects shared-mail configuration and use unless
// authoritative token evidence classifies the account as organizational. It
// performs no Graph traffic or mutation.
func ValidateMailCompatibility(alias MailAlias, tokenTenantContext string) error {
	if err := alias.Validate(); err != nil {
		return err
	}
	if tokenTenantContext != "organizational" {
		return fmt.Errorf("shared-mail aliases require an organizational token context")
	}
	return nil
}
