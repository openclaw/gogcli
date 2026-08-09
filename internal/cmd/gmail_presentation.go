package cmd

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func gmailDraftColumns() []outfmt.Column[*gmail.Draft] {
	return []outfmt.Column[*gmail.Draft]{
		{Header: "ID", Value: func(draft *gmail.Draft) string { return draft.Id }},
		{Header: "MESSAGE_ID", Value: func(draft *gmail.Draft) string {
			if draft.Message == nil {
				return ""
			}
			return draft.Message.Id
		}},
	}
}

func gmailLabelColumns() []outfmt.Column[*gmail.Label] {
	return []outfmt.Column[*gmail.Label]{
		{Header: "ID", Value: func(label *gmail.Label) string { return label.Id }},
		{Header: "NAME", Value: func(label *gmail.Label) string { return label.Name }},
		{Header: "TYPE", Value: func(label *gmail.Label) string { return label.Type }},
	}
}

func gmailMessageColumns(includeBody, includeAttachments, full bool) []outfmt.Column[messageItem] {
	columns := []outfmt.Column[messageItem]{
		{Header: "ID", Value: func(item messageItem) string { return item.ID }},
		{Header: "THREAD", Value: func(item messageItem) string { return item.ThreadID }},
		{Header: "DATE", Value: func(item messageItem) string { return item.Date }},
		{Header: "FROM", Value: func(item messageItem) string { return item.From }},
		{Header: "SUBJECT", Value: func(item messageItem) string { return item.Subject }},
		{Header: "LABELS", Value: func(item messageItem) string { return strings.Join(item.Labels, ",") }},
	}
	if includeBody {
		columns = append(columns, outfmt.Column[messageItem]{
			Header: "BODY",
			Value:  func(item messageItem) string { return sanitizeMessageBody(item.Body, full) },
		})
	}
	if includeAttachments {
		// No attachmentId: Gmail mints a fresh opaque token per fetch, so it's
		// neither stable nor a usable text handle. The id lives in JSON output.
		columns = append(columns, outfmt.Column[messageItem]{
			Header: "ATTACHMENTS",
			Value: func(item messageItem) string {
				parts := make([]string, len(item.Attachments))
				for i, a := range item.Attachments {
					parts[i] = fmt.Sprintf(
						"%s (%s, %s)",
						sanitizeGmailAttachmentTableValue(a.Filename),
						sanitizeGmailAttachmentTableValue(a.MimeType),
						a.SizeHuman,
					)
				}
				return strings.Join(parts, ", ")
			},
		})
	}
	return columns
}

func sanitizeGmailAttachmentTableValue(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, value)
}

func gmailThreadColumns() []outfmt.Column[threadItem] {
	return []outfmt.Column[threadItem]{
		{Header: "ID", Value: func(item threadItem) string { return item.ID }},
		{Header: "DATE", Value: func(item threadItem) string { return item.Date }},
		{Header: "FROM", Value: func(item threadItem) string { return item.From }},
		{Header: "SUBJECT", Value: func(item threadItem) string { return item.Subject }},
		{Header: "LABELS", Value: func(item threadItem) string { return strings.Join(item.Labels, ",") }},
		{Header: "THREAD", Value: func(item threadItem) string {
			if item.MessageCount > 1 {
				return fmt.Sprintf("[%d msgs]", item.MessageCount)
			}
			return "-"
		}},
	}
}

func gmailEmailStatusColumns() []outfmt.Column[gmailEmailStatusRow] {
	return []outfmt.Column[gmailEmailStatusRow]{
		{Header: "EMAIL", Value: func(row gmailEmailStatusRow) string { return sanitizeTab(row.Email) }},
		{Header: "STATUS", Value: func(row gmailEmailStatusRow) string { return sanitizeTab(row.Status) }},
	}
}

func compactGmailRows[T any](rows []*T) []*T {
	filtered := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
