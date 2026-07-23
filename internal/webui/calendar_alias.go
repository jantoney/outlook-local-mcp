package webui

import "strings"

// calendarAliasFromDisplayName converts a human calendar name into the local
// alias grammar. It returns "calendar" when the name has no ASCII letters or
// digits and performs no I/O.
func calendarAliasFromDisplayName(name string) string {
	var alias strings.Builder
	separator := false
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			if separator && alias.Len() > 0 && alias.Len() < 63 {
				alias.WriteByte('-')
			}
			separator = false
			if alias.Len() < 64 {
				alias.WriteRune(char)
			}
		default:
			separator = true
		}
	}
	value := strings.Trim(alias.String(), "-")
	if value == "" {
		return "calendar"
	}
	return value
}
