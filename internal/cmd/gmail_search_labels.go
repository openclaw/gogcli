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

// gmailQueryBracketChars are the characters Gmail uses to group terms; they
// are also what a term is trimmed of before it is matched against a label.
const gmailQueryBracketChars = `"'(),[]`

// gmailQuerySystemLabelIDs returns the system labels that every message
// matching query must carry, so they can be sent as labelIds alongside q
// (Gmail ignores `is:unread` next to `in:spam` unless it is also a label
// constraint, see #449).
//
// Only terms that hold for every result are lifted. A term under NOT or `-`,
// a term inside a negated group such as `-(…)` or `-subject:(…)`, and every
// term of a query that contains a disjunction (`OR`, `|` or `{…}`) are left
// for Gmail to interpret, because a labelIds constraint derived from them
// would be ANDed with q and narrow the result (#1212).
func gmailQuerySystemLabelIDs(query string) []string {
	fields := strings.Fields(query)
	for _, field := range fields {
		if strings.ContainsAny(field, "{}|") || gmailQueryFieldIsOperator(field, "or") {
			return nil
		}
	}

	var labels []string
	seen := map[string]bool{}
	depth := 0
	negatedDepth := -1 // bracket depth at which a negated group opened; -1 outside one
	negateNext := false
	for _, field := range fields {
		opens := strings.Count(field, "(") + strings.Count(field, "[")
		closes := strings.Count(field, ")") + strings.Count(field, "]")
		if gmailQueryFieldIsOperator(field, "not") {
			negateNext = true
			if negatedDepth < 0 && opens > closes {
				negatedDepth = depth
			}
			depth += opens - closes
			continue
		}

		token := strings.ToLower(strings.Trim(field, gmailQueryBracketChars))
		negated := negateNext || negatedDepth >= 0 || strings.HasPrefix(token, "-")
		negateNext = false
		if negated && negatedDepth < 0 && opens > closes {
			negatedDepth = depth
		}
		depth += opens - closes

		if !negated {
			if labelID := gmailQuerySystemLabelID(token); labelID != "" && !seen[labelID] {
				seen[labelID] = true
				labels = append(labels, labelID)
			}
		}
		if negatedDepth >= 0 && depth <= negatedDepth {
			negatedDepth = -1
		}
	}
	return labels
}

// gmailQueryFieldIsOperator reports whether field is the query operator op,
// also when written against a bracket or quote, as in `OR(in:inbox)`.
func gmailQueryFieldIsOperator(field, op string) bool {
	for _, part := range strings.FieldsFunc(field, func(r rune) bool {
		return strings.ContainsRune(gmailQueryBracketChars, r)
	}) {
		if strings.EqualFold(part, op) {
			return true
		}
	}
	return false
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
