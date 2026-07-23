package audit

// mailCapabilityForTool returns the exact local policy action guarded by a
// mail-domain tool. Unknown and non-mail tools return an empty capability.
func mailCapabilityForTool(toolName string) string {
	switch toolName {
	case "mail.list_folders", "mail.list_messages", "mail.get_message",
		"mail.search_messages", "mail.get_conversation", "mail.list_attachments",
		"mail.get_attachment":
		return "read"
	case "mail.create_draft", "mail.create_reply_draft", "mail.create_forward_draft",
		"mail.update_draft", "mail.delete_draft", "mail.add_attachment", "mail.remove_attachment":
		return "draft"
	case "mail.send_draft":
		return "send"
	case "mail.move_message":
		return "move"
	case "mail.archive_message":
		return "archive"
	case "mail.trash_message":
		return "trash"
	case "mail.restore_message":
		return "restore"
	case "mail.permanent_delete_message":
		return "permanent_delete"
	default:
		return ""
	}
}
