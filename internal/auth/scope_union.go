package auth

import "sort"

var canonicalOAuthScopeOrder = map[string]int{
	"User.Read":                  0,
	"Calendars.Read":             10,
	"Calendars.ReadWrite":        11,
	"Calendars.Read.Shared":      12,
	"Calendars.ReadWrite.Shared": 13,
	"Mail.Read":                  20,
	"Mail.Read.Shared":           21,
	"Mail.ReadWrite":             22,
	"Mail.ReadWrite.Shared":      23,
	"Mail.Send":                  24,
	"Mail.Send.Shared":           25,
}

// OAuthScopeUnion returns the deterministic, deduplicated union of every
// supplied policy scope set. Known Microsoft Graph scopes use a stable
// capability order; future scopes sort lexically after them. Empty scopes are
// ignored. The returned slice is newly allocated.
func OAuthScopeUnion(scopeSets ...[]string) []string {
	unique := make(map[string]struct{})
	for _, scopes := range scopeSets {
		for _, scope := range scopes {
			if scope != "" {
				unique[scope] = struct{}{}
			}
		}
	}

	union := make([]string, 0, len(unique))
	for scope := range unique {
		union = append(union, scope)
	}
	sort.Slice(union, func(i, j int) bool {
		leftOrder, leftKnown := canonicalOAuthScopeOrder[union[i]]
		rightOrder, rightKnown := canonicalOAuthScopeOrder[union[j]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return union[i] < union[j]
	})
	return union
}

// OAuthScopeSetEqual reports whether two scope slices describe the same
// effective OAuth grant, independent of order and duplicate values.
func OAuthScopeSetEqual(left, right []string) bool {
	leftUnion := OAuthScopeUnion(left)
	rightUnion := OAuthScopeUnion(right)
	if len(leftUnion) != len(rightUnion) {
		return false
	}
	for index := range leftUnion {
		if leftUnion[index] != rightUnion[index] {
			return false
		}
	}
	return true
}
