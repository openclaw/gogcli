package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
)

const (
	missingInlineImagesError       = "error"
	missingInlineImagesPlaceholder = "placeholder"
	maxInlineMIMEDepth             = 64
	maxInlineMIMEParts             = 1000
)

type inlineImageWarning struct {
	Code            string `json:"code"`
	SourceMessageID string `json:"sourceMessageId"`
	ContentID       string `json:"contentId"`
	Occurrences     int    `json:"occurrences"`
	Replacement     string `json:"replacement"`
}

func validateMissingInlineImagesPolicy(policy string, noQuote bool) (string, error) {
	switch policy {
	case "", missingInlineImagesError:
		return missingInlineImagesError, nil
	case missingInlineImagesPlaceholder:
		if noQuote {
			return "", usage("--missing-inline-images=placeholder cannot be combined with --no-quote")
		}
		return policy, nil
	default:
		return "", usage("--missing-inline-images must be error or placeholder")
	}
}

func prepareReplyInlineResources(ctx context.Context, svc *gmail.Service, msg *gmail.Message, info *replyInfo, policy string) error {
	if policy != missingInlineImagesPlaceholder {
		resources, err := preserveReferencedInlineResources(ctx, svc, msg.Id, msg.Payload, info.BodyHTML)
		info.InlineResources = resources
		return err
	}
	parts, err := strictInlineParts(msg.Payload)
	if err != nil {
		return err
	}
	missing := make(map[string]string)
	for _, id := range referencedContentIDs(info.BodyHTML) {
		if parts[canonicalContentID(id)] == nil {
			missing[canonicalContentID(id)] = id
		}
	}
	if len(missing) == 0 {
		resources, preserveErr := preserveReferencedInlineResources(ctx, svc, msg.Id, msg.Payload, info.BodyHTML)
		info.InlineResources = resources
		return preserveErr
	}
	updatedHTML, warnings, markers, err := replaceMissingInlineImages(info.BodyHTML, msg.Id, missing)
	if err != nil {
		return err
	}
	if validationErr := validateMissingInlineSource(ctx, svc, msg.Id, missing); validationErr != nil {
		return validationErr
	}
	resources, err := preserveReferencedInlineResources(ctx, svc, msg.Id, msg.Payload, updatedHTML)
	if err != nil {
		return err
	}
	info.BodyHTML = updatedHTML
	if info.Body == "" {
		info.Body = htmlToPlainText(updatedHTML)
	} else {
		info.Body += "\n\n" + strings.Join(markers, "\n")
	}
	info.InlineResources = resources
	info.InlineImageWarnings = warnings
	return nil
}

func strictInlineParts(payload *gmail.MessagePart) (map[string]*gmail.MessagePart, error) {
	parts := make(map[string]*gmail.MessagePart)
	count := 0
	var walk func(*gmail.MessagePart, int) error
	walk = func(part *gmail.MessagePart, depth int) error {
		if part == nil {
			return fmt.Errorf("missing MIME part")
		}
		count++
		if depth > maxInlineMIMEDepth || count > maxInlineMIMEParts {
			return fmt.Errorf("MIME structure exceeds inline-image validation limits")
		}
		ids := 0
		for _, header := range part.Headers {
			if header == nil || !strings.EqualFold(header.Name, "Content-ID") {
				continue
			}
			ids++
			value, idErr := strictInlineContentID(header.Value)
			if idErr != nil {
				return idErr
			}
			id := canonicalContentID(value)
			if ids > 1 || parts[id] != nil {
				return fmt.Errorf("ambiguous MIME Content-ID")
			}
			parts[id] = part
		}
		if len(part.Parts) == 0 {
			if part.Body == nil {
				return fmt.Errorf("MIME part has no body")
			}
			if part.Body.Data != "" {
				if _, decodeErr := gmailcontent.DecodeBase64URLBytes(part.Body.Data); decodeErr != nil {
					return fmt.Errorf("decode MIME part: %w", decodeErr)
				}
			} else if part.Body.Size > 0 && part.Body.AttachmentId == "" {
				return fmt.Errorf("MIME part has no body data")
			}
		}
		for _, child := range part.Parts {
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(payload, 0); err != nil {
		return nil, err
	}
	return parts, nil
}
