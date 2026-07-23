package resource

import "time"

// ValidationStatus identifies the latest direct Graph validation state for a
// configured shared resource. Only available permits operational routing.
type ValidationStatus string

const (
	// ValidationNotChecked indicates no direct validation has run this process.
	ValidationNotChecked ValidationStatus = "not_checked"
	// ValidationUnverified indicates configuration exists but its policy is off.
	ValidationUnverified ValidationStatus = "unverified"
	// ValidationReauthenticationRequired indicates required scopes are not active.
	ValidationReauthenticationRequired ValidationStatus = "reauthentication_required"
	// ValidationValidating indicates a bounded metadata request is in flight.
	ValidationValidating ValidationStatus = "validating"
	// ValidationAvailable indicates the latest direct metadata request succeeded.
	ValidationAvailable ValidationStatus = "available"
	// ValidationUnavailable indicates Graph reported the configured target missing.
	ValidationUnavailable ValidationStatus = "unavailable"
	// ValidationFailed indicates validation failed for another provider reason.
	ValidationFailed ValidationStatus = "validation_failed"
)

// Validation records the latest direct metadata validation outcome. Message is
// a sanitized operational explanation and CheckedAt is omitted before a result.
type Validation struct {
	Status    ValidationStatus `json:"status"`
	Message   string           `json:"message,omitempty"`
	CheckedAt time.Time        `json:"checked_at,omitempty"`
}

// AllowsOperations reports whether direct Graph validation currently permits
// routing. It returns false for unknown and in-progress states.
func (v Validation) AllowsOperations() bool { return v.Status == ValidationAvailable }
