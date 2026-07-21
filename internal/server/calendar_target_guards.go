package server

import "github.com/desek/outlook-local-mcp/internal/resource"

// calendarOwnerReadGuard authorizes own and owner-primary calendar reads. The
// mounted-calendar kind is enabled separately by issue #9.
func calendarOwnerReadGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyCalendar, Capability: resource.TargetCapabilityRead,
		AllowedKinds: []resource.ResourceKind{
			resource.ResourceKindOwnCalendar,
			resource.ResourceKindOwnerPrimaryCalendar,
		},
	}
}

// calendarOwnerGetGuard adds conditional shared-event reference validation to
// the owner read policy while preserving own raw event IDs.
func calendarOwnerGetGuard() TargetGuardConfig {
	config := calendarOwnerReadGuard()
	config.ReferenceArgument = "resource_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "event_id"
	config.ItemKind = resource.ItemKindEvent
	return config
}
