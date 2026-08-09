package cmd

import (
	"context"
	"slices"

	"github.com/openclaw/gogcli/internal/outfmt"
)

const gmailDraftNotSentMarker = " [DRAFT — NOT SENT]"

func gmailHumanMessageStatusMarker(ctx context.Context, labelIDs []string) string {
	if outfmt.IsJSON(ctx) || outfmt.IsPlain(ctx) {
		return ""
	}
	if slices.Contains(labelIDs, gmailSystemLabelDraft) {
		return gmailDraftNotSentMarker
	}
	return ""
}
