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

// calendarMountedManageGuard authorizes existing own writes and organizational
// mounted-calendar writes with exact manage capability.
func calendarMountedManageGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyCalendar, Capability: resource.TargetCapabilityManage,
		AllowedKinds: []resource.ResourceKind{
			resource.ResourceKindOwnCalendar,
			resource.ResourceKindMountedCalendar,
		},
	}
}

// calendarMountedManageEventGuard requires target-bound provenance only for
// shared mounted follow-up writes while preserving own raw event IDs.
func calendarMountedManageEventGuard() TargetGuardConfig {
	config := calendarMountedManageGuard()
	config.ReferenceArgument = "resource_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "event_id"
	config.ItemKind = resource.ItemKindEvent
	return config
}
