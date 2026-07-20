package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TargetRequest declares the complete local authorization decision required
// before a Graph route may be selected.
type TargetRequest struct {
	// Family selects the separate calendar or mail alias namespace.
	Family resource.TargetFamily
	// SharedResource is an optional account-scoped alias; empty selects own `/me`.
	SharedResource string
	// Capability is the exact local action required on the selected target.
	Capability resource.TargetCapability
	// Reference is an optional signed item provenance token.
	Reference string
	// RequireReference rejects a missing signed reference for target-bound actions.
	RequireReference bool
	// ItemKind is the exact referenced item kind expected by the operation.
	ItemKind resource.ItemKind
}

// ResolvedTarget contains the one immutable authorized target and, when
// supplied, verified reference claims. It contains no Graph request builder.
type ResolvedTarget struct {
	// Target is the selected own or account-scoped shared resource snapshot.
	Target resource.Target
	// Claims contains verified target-bound item provenance, or nil when the
	// operation does not consume a reference.
	Claims *resource.ReferenceClaims
}

// ResolveTarget validates alias existence, family, tenant compatibility, exact
// capability, and signed-reference provenance without constructing a Graph
// route or making network traffic.
func ResolveTarget(entry *AccountEntry, request TargetRequest, codec *resource.ReferenceCodec) (ResolvedTarget, error) {
	if entry == nil {
		return ResolvedTarget{}, fmt.Errorf("target resolution requires an account")
	}
	target, err := resolveTargetIdentity(entry, request)
	if err != nil {
		return ResolvedTarget{}, err
	}
	if !target.Allows(request.Capability) {
		return ResolvedTarget{}, fmt.Errorf("account %q target %q capability %q denied by target-local policy", entry.Label, targetName(target), request.Capability)
	}
	claims, err := validateTargetReference(target, request, codec)
	if err != nil {
		return ResolvedTarget{}, err
	}
	return ResolvedTarget{Target: target, Claims: claims}, nil
}

// resolveTargetIdentity selects own resources or exactly one account-scoped
// alias family and applies token-context compatibility before authorization.
func resolveTargetIdentity(entry *AccountEntry, request TargetRequest) (resource.Target, error) {
	if request.SharedResource == "" {
		return resource.NewOwnTarget(entry.AccountID, request.Family, entry.MailPolicy)
	}
	switch request.Family {
	case resource.TargetFamilyCalendar:
		for _, alias := range entry.CalendarAliases {
			if alias.Alias != request.SharedResource {
				continue
			}
			if err := resource.ValidateCalendarCompatibility(alias, string(entry.EffectiveTokenTenantContext())); err != nil {
				return resource.Target{}, fmt.Errorf("account %q calendar alias %q capability %q denied: %w", entry.Label, alias.Alias, request.Capability, err)
			}
			return resource.TargetFromCalendarAlias(entry.AccountID, alias)
		}
		if hasMailAlias(entry, request.SharedResource) {
			return resource.Target{}, fmt.Errorf("shared resource %q is a mail alias, not a calendar alias", request.SharedResource)
		}
	case resource.TargetFamilyMail:
		for _, alias := range entry.MailAliases {
			if alias.Alias != request.SharedResource {
				continue
			}
			if err := resource.ValidateMailCompatibility(alias, string(entry.EffectiveTokenTenantContext())); err != nil {
				return resource.Target{}, fmt.Errorf("account %q mail alias %q capability %q denied: %w", entry.Label, alias.Alias, request.Capability, err)
			}
			return resource.TargetFromMailAlias(entry.AccountID, alias)
		}
		if hasCalendarAlias(entry, request.SharedResource) {
			return resource.Target{}, fmt.Errorf("shared resource %q is a calendar alias, not a mail alias", request.SharedResource)
		}
	default:
		return resource.Target{}, fmt.Errorf("invalid target family %q", request.Family)
	}
	return resource.Target{}, fmt.Errorf("shared resource %q not found for account %q", request.SharedResource, entry.Label)
}

// validateTargetReference verifies signed claims and binds them to the current
// account, target identity, kind, view, and expected item kind.
func validateTargetReference(target resource.Target, request TargetRequest, codec *resource.ReferenceCodec) (*resource.ReferenceClaims, error) {
	if request.Reference == "" {
		if request.RequireReference {
			return nil, fmt.Errorf("a signed resource_ref is required")
		}
		return nil, nil
	}
	if codec == nil {
		return nil, fmt.Errorf("resource reference verification is unavailable")
	}
	claims, err := codec.Verify(request.Reference)
	if err != nil {
		return nil, err
	}
	if claims.AccountID != target.AccountID || claims.ResourceID != target.ResourceID ||
		claims.ResourceKind != target.Kind || claims.MailboxView != target.View {
		return nil, fmt.Errorf("resource reference does not match the resolved target")
	}
	if request.ItemKind != "" && claims.ItemKind != request.ItemKind {
		return nil, fmt.Errorf("resource reference item kind %q does not match required %q", claims.ItemKind, request.ItemKind)
	}
	return &claims, nil
}

// targetName returns a stable error-safe selector for own or aliased targets.
func targetName(target resource.Target) string {
	if target.Alias != "" {
		return target.Alias
	}
	return string(target.Kind)
}

// hasMailAlias reports whether name exists in the separate shared-mail family.
func hasMailAlias(entry *AccountEntry, name string) bool {
	for _, alias := range entry.MailAliases {
		if alias.Alias == name {
			return true
		}
	}
	return false
}

// hasCalendarAlias reports whether name exists in the calendar alias family.
func hasCalendarAlias(entry *AccountEntry, name string) bool {
	for _, alias := range entry.CalendarAliases {
		if alias.Alias == name {
			return true
		}
	}
	return false
}
