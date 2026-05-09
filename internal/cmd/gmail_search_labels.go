package cmd

import (
	"strings"
)

const (
	gmailSystemLabelUnread    = "UNREAD"
	gmailSystemLabelStarred   = "STARRED"
	gmailSystemLabelImportant = "IMPORTANT"
	gmailSystemLabelInbox     = "INBOX"
	gmailSystemLabelSent      = "SENT"
	gmailSystemLabelDraft     = "DRAFT"
	gmailSystemLabelSpam      = "SPAM"
	gmailSystemLabelTrash     = "TRASH"
)

func gmailQuerySystemLabelIDs(query string) []string {
	fields := strings.Fields(query)
	for _, field := range fields {
		token := strings.ToLower(strings.TrimSpace(field))
		if token == "or" || strings.ContainsAny(token, "{}") {
			return nil
		}
	}

	var labels []string
	seen := map[string]bool{}
	for _, field := range fields {
		token := strings.ToLower(strings.Trim(field, `"'(),[]`))
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}
		if labelID := gmailQuerySystemLabelID(token); labelID != "" && !seen[labelID] {
			seen[labelID] = true
			labels = append(labels, labelID)
		}
	}
	return labels
}

func gmailQuerySystemLabelID(token string) string {
	switch token {
	case "is:unread", "label:unread":
		return gmailSystemLabelUnread
	case "is:starred", "label:starred":
		return gmailSystemLabelStarred
	case "is:important", "label:important":
		return gmailSystemLabelImportant
	case "in:inbox", "label:inbox":
		return gmailSystemLabelInbox
	case "in:sent", "label:sent":
		return gmailSystemLabelSent
	case "in:draft", "in:drafts", "label:draft", "label:drafts":
		return gmailSystemLabelDraft
	case "in:spam", "is:spam", "label:spam":
		return gmailSystemLabelSpam
	case "in:trash", "label:trash":
		return gmailSystemLabelTrash
	case "category:primary":
		return "CATEGORY_PERSONAL"
	case "category:social":
		return "CATEGORY_SOCIAL"
	case "category:promotions":
		return "CATEGORY_PROMOTIONS"
	case "category:updates":
		return "CATEGORY_UPDATES"
	case "category:forums":
		return "CATEGORY_FORUMS"
	default:
		return ""
	}
}
