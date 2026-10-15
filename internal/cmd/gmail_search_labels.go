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

// gmailQueryField is one whitespace-separated field of a query. Whitespace
// inside double quotes does not split, and the quoted text is kept out of
// operator and bracket detection: `subject:"not for sale"` is a phrase, not
// a negation.
type gmailQueryField struct {
	text     string // the field as written
	unquoted string // the field with every "…" span removed
}

func gmailQueryFields(query string) []gmailQueryField {
	var fields []gmailQueryField
	var text, unquoted strings.Builder
	inQuote := false
	flush := func() {
		if text.Len() > 0 {
			fields = append(fields, gmailQueryField{text: text.String(), unquoted: unquoted.String()})
			text.Reset()
			unquoted.Reset()
		}
	}
	for _, r := range query {
		switch {
		case r == '"':
			inQuote = !inQuote
			text.WriteRune(r)
		case inQuote:
			text.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			text.WriteRune(r)
			unquoted.WriteRune(r)
		}
	}
	flush()
	return fields
}

// gmailQuerySystemLabelIDs returns the system labels that every message
// matching query must carry, so they can be sent as labelIds alongside q
// (Gmail ignores `is:unread` next to `in:spam` unless it is also a label
// constraint, see #449).
//
// Only terms that hold for every result are lifted. A term under NOT or `-`,
// a term inside a negated group such as `-(…)`, `NOT (…)` or `-subject:(…)`,
// and every term of a query that contains a disjunction (`OR`, `|` or `{…}`)
// are left for Gmail to interpret, because a labelIds constraint derived
// from them would be ANDed with q and narrow the result (#1212).
func gmailQuerySystemLabelIDs(query string) []string {
	fields := gmailQueryFields(query)
	for _, field := range fields {
		if strings.ContainsAny(field.unquoted, "{}|") || gmailQueryFieldIsOperator(field.unquoted, "or") {
			return nil
		}
	}

	var labels []string
	seen := map[string]bool{}
	depth := 0
	negatedDepth := -1 // bracket depth at which a negated group opened; -1 outside one
	negateNext := false
	for _, field := range fields {
		opens := strings.Count(field.unquoted, "(") + strings.Count(field.unquoted, "[")
		closes := strings.Count(field.unquoted, ")") + strings.Count(field.unquoted, "]")

		if operand, isNot := gmailQueryNotOperand(field.unquoted); isNot {
			switch {
			case operand == "":
				// Bare NOT: the next field is its operand.
				negateNext = true
			case negatedDepth < 0 && opens > closes:
				// NOT(zzqx …: the operand is a group that continues.
				negatedDepth = depth
			default:
				// NOT(in:inbox): the operand is complete in this field.
			}
		} else {
			token := strings.ToLower(strings.Trim(field.text, gmailQueryBracketChars))
			negated := negateNext || negatedDepth >= 0 || strings.HasPrefix(token, "-")
			negateNext = false
			if negated && negatedDepth < 0 && opens > closes {
				negatedDepth = depth
			}
			if !negated {
				if labelID := gmailQuerySystemLabelID(token); labelID != "" && !seen[labelID] {
					seen[labelID] = true
					labels = append(labels, labelID)
				}
			}
		}

		// Brackets closed by this field, whether it was a term or an inline NOT, may end
		// the negated group: `-(foo NOT(bar)) is:unread` lifts UNREAD.
		depth += opens - closes
		if negatedDepth >= 0 && depth <= negatedDepth {
			negatedDepth = -1
		}
	}
	return labels
}

// gmailQueryFieldIsOperator reports whether the unquoted text of a field is
// the query operator op, also when written against a bracket, as in
// `OR(in:inbox)`.
func gmailQueryFieldIsOperator(unquoted, op string) bool {
	for _, part := range strings.FieldsFunc(unquoted, func(r rune) bool {
		return strings.ContainsRune(gmailQueryBracketChars, r)
	}) {
		if strings.EqualFold(part, op) {
			return true
		}
	}
	return false
}

// gmailQueryNotOperand reports whether the unquoted text of a field starts
// with the NOT operator and returns what follows it in the same field:
// "" for a bare `NOT`, `(in:inbox)` for `NOT(in:inbox)`.
func gmailQueryNotOperand(unquoted string) (string, bool) {
	rest := strings.TrimLeft(unquoted, gmailQueryBracketChars)
	if len(rest) < 3 || !strings.EqualFold(rest[:3], "not") {
		return "", false
	}
	operand := rest[3:]
	if operand != "" && !strings.ContainsRune(gmailQueryBracketChars, rune(operand[0])) {
		return "", false // a word such as "nothing" or "note:"
	}
	return operand, true
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
