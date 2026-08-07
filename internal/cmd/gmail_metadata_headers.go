package cmd

import "google.golang.org/api/googleapi"

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

// gmailAttachmentPartFields selects one MIME part's attachment metadata
// (filename, mimeType, size, attachmentId) but never body/data.
const gmailAttachmentPartFields = "filename,mimeType,body(size,attachmentId)"

// gmailMessageAttachmentFields extends the summary mask with attachment metadata
// at the payload root and every nested part. Gmail field masks can't recurse, so
// the parts tree is expanded to a fixed depth deeper than real multipart mail
// nests. Paired with format=full, so --include-attachments lists attachments
// without pulling bodies.
var gmailMessageAttachmentFields = googleapi.Field(buildGmailMessageAttachmentFields(8))

func buildGmailMessageAttachmentFields(depth int) string {
	parts := gmailAttachmentPartFields
	for i := 0; i < depth; i++ {
		parts = gmailAttachmentPartFields + ",parts(" + parts + ")"
	}
	return "id,threadId,labelIds,internalDate,payload(" + gmailAttachmentPartFields + ",headers,parts(" + parts + "))"
}

func defaultGmailGetMetadataHeaders() []string {
	headers := append([]string{}, gmailBasicMetadataHeaders...)
	headers = append(headers, "Message-ID", "In-Reply-To", "References", "List-Unsubscribe")
	return headers
}
