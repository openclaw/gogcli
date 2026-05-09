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

func defaultGmailGetMetadataHeaders() []string {
	headers := append([]string{}, gmailBasicMetadataHeaders...)
	headers = append(headers, "Message-ID", "In-Reply-To", "References", "List-Unsubscribe")
	return headers
}
