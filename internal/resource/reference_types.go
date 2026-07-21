package resource

import (
	"fmt"
	"strings"
)

// AccountID is the immutable local provenance identity of one signed-in
// account instance. The auth package aliases this transport-neutral type.
type AccountID string

// ResourceID is the immutable local identity of one configured shared
// resource. Alias renames preserve it; removal and recreation replace it.
type ResourceID string

// ResourceKind identifies the configured Outlook resource addressed by a
// reference.
type ResourceKind string

const (
	// ResourceKindMailbox identifies a shared mailbox in the owner view.
	ResourceKindMailbox ResourceKind = "mailbox"
	// ResourceKindOwnMailbox identifies the signed-in account's own mailbox.
	ResourceKindOwnMailbox ResourceKind = "own_mailbox"
	// ResourceKindOwnCalendar identifies the signed-in account's own calendar view.
	ResourceKindOwnCalendar ResourceKind = "own_calendar"
	// ResourceKindOwnerPrimaryCalendar identifies a resource owner's primary
	// calendar in the owner view.
	ResourceKindOwnerPrimaryCalendar ResourceKind = "owner_primary_calendar"
	// ResourceKindMountedCalendar identifies a selected calendar mounted in the
	// signed-in account's recipient view.
	ResourceKindMountedCalendar ResourceKind = "mounted_calendar"
)

// MailboxView identifies the Graph identity space in which referenced IDs are
// valid.
type MailboxView string

const (
	// MailboxViewRecipient identifies the signed-in account's `/me` view.
	MailboxViewRecipient MailboxView = "recipient"
	// MailboxViewOwner identifies a resource owner's `/users/{owner}` view.
	MailboxViewOwner MailboxView = "owner"
)

// ItemKind identifies the Outlook item represented by the final Graph ID in a
// signed reference.
type ItemKind string

// MailFolderClass records the authorization-relevant destination class that
// was resolved before a folder reference was signed.
type MailFolderClass string

const (
	// MailFolderClassOrdinary permits a caller-selected filing destination.
	MailFolderClassOrdinary MailFolderClass = "ordinary"
	// MailFolderClassReserved identifies a system or semantic destination that
	// requires another exact capability such as archive or trash.
	MailFolderClassReserved MailFolderClass = "reserved"
)

const (
	// ItemKindCalendar identifies a calendar container.
	ItemKindCalendar ItemKind = "calendar"
	// ItemKindEvent identifies a calendar event.
	ItemKindEvent ItemKind = "event"
	// ItemKindMailFolder identifies a mail folder.
	ItemKindMailFolder ItemKind = "mail_folder"
	// ItemKindMessage identifies a mail message.
	ItemKindMessage ItemKind = "message"
	// ItemKindDraft identifies a draft mail message.
	ItemKindDraft ItemKind = "draft"
	// ItemKindConversation identifies a mail conversation.
	ItemKindConversation ItemKind = "conversation"
	// ItemKindAttachment identifies an attachment whose chain also includes its
	// parent message.
	ItemKindAttachment ItemKind = "attachment"
)

// GraphID is one typed element of the ordered Graph identifier chain required
// to address a referenced item.
type GraphID struct {
	// Kind identifies what the Graph identifier addresses.
	Kind ItemKind `json:"kind"`
	// ID is the opaque Graph identifier in the reference's mailbox view.
	ID string `json:"id"`
}

// ReferenceClaims is the complete immutable provenance payload protected by a
// resource reference signature. It contains no authorization decision and
// grants no Graph access by itself.
type ReferenceClaims struct {
	// AccountID binds the reference to one signed-in account instance.
	AccountID AccountID `json:"account_id"`
	// ResourceID binds the reference to one shared resource instance.
	ResourceID ResourceID `json:"resource_id"`
	// ResourceKind binds the reference to the configured resource kind.
	ResourceKind ResourceKind `json:"resource_kind"`
	// MailboxView binds all Graph IDs to one recipient or owner identity space.
	MailboxView MailboxView `json:"mailbox_view"`
	// ItemKind identifies the referenced item's semantic kind.
	ItemKind ItemKind `json:"item_kind"`
	// GraphIDChain is the ordered Graph identifier chain needed to address the
	// item, ending with ItemKind.
	GraphIDChain []GraphID `json:"graph_id_chain"`
	// MailFolderClass is populated only for destination folder references that
	// were resolved against the mailbox's well-known reserved folders.
	MailFolderClass MailFolderClass `json:"mail_folder_class,omitempty"`
}

// validate checks that claims contain a complete, internally consistent
// provenance statement. It returns an error for malformed claims and has no
// side effects.
func (c ReferenceClaims) validate() error {
	if err := c.AccountID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(c.ResourceID)) == "" {
		return fmt.Errorf("resource identity must not be empty")
	}
	if err := validateResourceView(c.ResourceKind, c.MailboxView); err != nil {
		return err
	}
	if !validItemKind(c.ItemKind) {
		return fmt.Errorf("invalid item kind %q", c.ItemKind)
	}
	if len(c.GraphIDChain) == 0 {
		return fmt.Errorf("Graph ID chain must not be empty")
	}
	if c.ItemKind != ItemKindMailFolder && c.MailFolderClass != "" {
		return fmt.Errorf("mail folder class is valid only for folder references")
	}
	if c.MailFolderClass != "" && c.MailFolderClass != MailFolderClassOrdinary && c.MailFolderClass != MailFolderClassReserved {
		return fmt.Errorf("invalid mail folder class %q", c.MailFolderClass)
	}
	for index, graphID := range c.GraphIDChain {
		if !validItemKind(graphID.Kind) || strings.TrimSpace(graphID.ID) == "" {
			return fmt.Errorf("invalid Graph ID chain element %d", index)
		}
	}
	return validateGraphIDChain(c.ItemKind, c.GraphIDChain)
}

// validateGraphIDChain enforces the exact routing provenance required for an
// item kind. Attachments bind their parent message or draft; every other item
// is addressed by one ID and rejects unrelated prefixes.
func validateGraphIDChain(itemKind ItemKind, chain []GraphID) error {
	if itemKind == ItemKindAttachment {
		if len(chain) != 2 ||
			(chain[0].Kind != ItemKindMessage && chain[0].Kind != ItemKindDraft) ||
			chain[1].Kind != ItemKindAttachment {
			return fmt.Errorf("attachment Graph ID chain must be parent message or draft followed by attachment")
		}
		return nil
	}
	if len(chain) != 1 || chain[0].Kind != itemKind {
		return fmt.Errorf("Graph ID chain for %q must contain exactly that item", itemKind)
	}
	return nil
}

// validateResourceView verifies the only supported resource-kind and mailbox-
// view combinations. It returns an error for invalid or mismatched values.
func validateResourceView(kind ResourceKind, view MailboxView) error {
	switch kind {
	case ResourceKindMailbox, ResourceKindOwnerPrimaryCalendar:
		if view != MailboxViewOwner {
			return fmt.Errorf("resource kind %q requires owner mailbox view", kind)
		}
	case ResourceKindMountedCalendar, ResourceKindOwnMailbox, ResourceKindOwnCalendar:
		if view != MailboxViewRecipient {
			return fmt.Errorf("resource kind %q requires recipient mailbox view", kind)
		}
	default:
		return fmt.Errorf("invalid resource kind %q", kind)
	}
	return nil
}

// validItemKind reports whether kind is a supported signed-reference item
// discriminant. It has no side effects.
func validItemKind(kind ItemKind) bool {
	switch kind {
	case ItemKindCalendar, ItemKindEvent, ItemKindMailFolder, ItemKindMessage,
		ItemKindDraft, ItemKindConversation, ItemKindAttachment:
		return true
	default:
		return false
	}
}
