package server

import "github.com/desek/outlook-local-mcp/internal/resource"

// mailSharedReadGuard authorizes own-mail and organizational shared-mail reads.
func mailSharedReadGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyMail, Capability: resource.TargetCapabilityRead,
		AllowedKinds: []resource.ResourceKind{resource.ResourceKindOwnMailbox, resource.ResourceKindMailbox},
	}
}

// mailSharedFolderReadGuard accepts an optional target-bound folder reference
// for shared list_messages calls and rejects shared raw folder IDs.
func mailSharedFolderReadGuard() TargetGuardConfig {
	config := mailSharedReadGuard()
	config.ReferenceArgument = "folder_ref"
	config.RawIDArgument = "folder_id"
	config.ItemKind = resource.ItemKindMailFolder
	return config
}

// mailSharedMessageReadGuard requires target-bound message provenance only
// when a shared alias is selected and preserves own raw message IDs.
func mailSharedMessageReadGuard() TargetGuardConfig {
	config := mailSharedReadGuard()
	config.ReferenceArgument = "resource_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "message_id"
	config.ItemKind = resource.ItemKindMessage
	return config
}

// mailSharedParentMessageGuard requires a named target-bound parent message
// reference for shared conversation and attachment reads.
func mailSharedParentMessageGuard(argument string) TargetGuardConfig {
	config := mailSharedReadGuard()
	config.ReferenceArgument = argument
	config.RequireSharedReference = true
	config.RawIDArgument = "message_id"
	config.ItemKind = resource.ItemKindMessage
	return config
}
