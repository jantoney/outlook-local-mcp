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

// mailSharedDraftCreateGuard authorizes an own or shared mailbox for a new
// draft without requiring an existing item reference.
func mailSharedDraftCreateGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyMail, Capability: resource.TargetCapability(resource.MailCapabilityDraft),
		AllowedKinds: []resource.ResourceKind{resource.ResourceKindOwnMailbox, resource.ResourceKindMailbox},
	}
}

// mailSharedDraftSourceGuard authorizes reply and forward creation. Shared
// mailbox calls must prove the source message through a target-bound reference.
func mailSharedDraftSourceGuard() TargetGuardConfig {
	config := mailSharedDraftCreateGuard()
	config.ReferenceArgument = "message_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "message_id"
	config.ItemKind = resource.ItemKindMessage
	return config
}

// mailSharedDraftItemGuard authorizes mutations of one existing draft. Shared
// mailbox calls must supply a draft-kind reference and cannot use a raw ID.
func mailSharedDraftItemGuard() TargetGuardConfig {
	config := mailSharedDraftCreateGuard()
	config.ReferenceArgument = "draft_ref"
	config.RequireSharedReference = true
	config.RawIDArgument = "message_id"
	config.ItemKind = resource.ItemKindDraft
	return config
}

// mailMoveMessageGuard requires the exact move capability and a source
// message reference for both own and shared mailbox targets.
func mailMoveMessageGuard() TargetGuardConfig {
	return mailReferencedMessageMutationGuard(resource.MailCapabilityMove)
}

// mailReferencedMessageMutationGuard requires one exact mail capability and a
// target-bound source message reference for both own and shared mailboxes.
func mailReferencedMessageMutationGuard(capability resource.MailCapability) TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyMail, Capability: resource.TargetCapability(capability),
		AllowedKinds:      []resource.ResourceKind{resource.ResourceKindOwnMailbox, resource.ResourceKindMailbox},
		ReferenceArgument: "message_ref", RequireReference: true, ItemKind: resource.ItemKindMessage,
	}
}
