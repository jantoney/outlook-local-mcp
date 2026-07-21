package server

import "github.com/desek/outlook-local-mcp/internal/resource"

// calendarSharedReadGuard authorizes own, owner-primary, and explicitly
// selected mounted-calendar reads.
func calendarSharedReadGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyCalendar, Capability: resource.TargetCapabilityRead,
		AllowedKinds: []resource.ResourceKind{
			resource.ResourceKindOwnCalendar,
			resource.ResourceKindOwnerPrimaryCalendar,
			resource.ResourceKindMountedCalendar,
		},
	}
}

// calendarSharedGetGuard adds conditional shared-event reference validation
// while preserving own raw event IDs.
func calendarSharedGetGuard() TargetGuardConfig {
	config := calendarSharedReadGuard()
	config.ReferenceArgument = "resource_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "event_id"
	config.ItemKind = resource.ItemKindEvent
	return config
}
