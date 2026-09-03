package cmd

var (
	gmailBasicMetadataHeaders = []string{"From", "To", "Cc", "Bcc", "Subject", "Date"}
	gmailReplyMetadataHeaders = []string{"Message-ID", "Message-Id", "References", "In-Reply-To", "From", "Reply-To", "To", "Cc", "Date", "Subject"}

	gmailAutoReplyMetadataHeaders = []string{
		"Message-ID", "Message-Id", "References", "In-Reply-To",
		"From", "Reply-To", "To", "Cc", "Date", "Subject",
		"Auto-Submitted", "Precedence", "List-Id", "List-Unsubscribe",
	}

	gmailMessageSummaryMetadataHeaders = []string{"From", "Subject", "Date"}
)

// gmailMessageSummaryFields is the partial-response selector for message
// summary fetches. A `fields` mask silently drops anything it does not name:
// the field arrives zeroed, with no API error, so the omission surfaces only
// against live Gmail. Every messageItem field read off the API response must
// appear here — internalDate backs internalDateIso.
const gmailMessageSummaryFields = "id,threadId,labelIds,internalDate,payload(headers)"

func defaultGmailGetMetadataHeaders() []string {
	headers := append([]string{}, gmailBasicMetadataHeaders...)
	headers = append(headers, "Reply-To", "Message-ID", "In-Reply-To", "References", "List-Unsubscribe")
	return headers
}
