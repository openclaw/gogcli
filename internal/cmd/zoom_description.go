package cmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/zoom"
)

// Zoom info is attached to Calendar events via the event description rather
// than conferenceData. Google's Calendar API rejects conferenceData writes that
// assert conferenceSolution.key.type="addOn" from non-Workspace-Marketplace
// OAuth clients (400 "Invalid conference data") and silently drops the field
// entirely when key.type is omitted. Description-mode preserves the Zoom join
// URL + meeting ID + passcode in a form that round-trips through Google's
// storage and renders as clickable text in every Calendar UI; the trade-off is
// no native "Join with Zoom" conference card.
//
// The description block is delimited by HTML comment markers so it can be
// detected, replaced, or removed on subsequent --regenerate-zoom / --remove-zoom
// operations without disturbing other description content the user has typed.

const (
	zoomBlockStartFmt = "<!-- gog-zoom-meeting:%s -->"
	zoomBlockEnd      = "<!-- /gog-zoom-meeting -->"
)

var (
	zoomDescriptionBlockRE      = regexp.MustCompile(`(?s)<!-- gog-zoom-meeting:([^\s>]+) -->.*?<!-- /gog-zoom-meeting -->`)
	zoomDescriptionURLRE        = regexp.MustCompile(`https?://[^\s<>"']+`)
	zoomDescriptionPasscodeRE   = regexp.MustCompile(`(?m)^(Passcode:\s*)\S+`)
	zoomDescriptionBlankLinesRE = regexp.MustCompile(`\n{3,}`)
)

// buildZoomDescriptionBlock formats a Zoom meeting into a description block
// with stable start/end markers. Returns the empty string when meeting is nil
// or has no join URL.
func buildZoomDescriptionBlock(meeting *zoom.Meeting) string {
	if meeting == nil {
		return ""
	}
	join := strings.TrimSpace(meeting.JoinURL)
	if join == "" {
		return ""
	}
	id := zoomMeetingID(meeting)
	pwd := ""
	if u, err := url.Parse(join); err == nil {
		pwd = u.Query().Get("pwd")
	}

	var b strings.Builder
	fmt.Fprintf(&b, zoomBlockStartFmt, id)
	b.WriteString("\nJoin Zoom Meeting: ")
	b.WriteString(join)
	if id != "" {
		b.WriteString("\nMeeting ID: ")
		b.WriteString(id)
	}
	if pwd != "" {
		b.WriteString("\nPasscode: ")
		b.WriteString(pwd)
	}
	b.WriteString("\n")
	b.WriteString(zoomBlockEnd)
	return b.String()
}

// applyZoomDescriptionBlock returns desc with the given Zoom block applied:
// any existing gog-managed Zoom block is removed first, then the new block is
// appended (separated from prior content by a blank line if prior content
// exists). Passing an empty block removes any existing block without appending.
func applyZoomDescriptionBlock(desc, block string) string {
	stripped := removeZoomDescriptionBlock(desc)
	if block == "" {
		return stripped
	}
	if strings.TrimSpace(stripped) == "" {
		return block
	}
	return strings.TrimRight(stripped, "\n") + "\n\n" + block
}

// removeZoomDescriptionBlock returns desc with any gog-managed Zoom block
// stripped out. Surrounding whitespace is trimmed so a description that
// consisted only of a Zoom block becomes empty rather than blank lines.
func removeZoomDescriptionBlock(desc string) string {
	if desc == "" {
		return ""
	}
	out := zoomDescriptionBlockRE.ReplaceAllString(desc, "")
	// Collapse runs of blank lines that may be left behind.
	out = zoomDescriptionBlankLinesRE.ReplaceAllString(out, "\n\n")
	return strings.Trim(out, "\n ")
}

func redactZoomDescription(desc string) string {
	if desc == "" {
		return ""
	}
	return zoomDescriptionBlockRE.ReplaceAllStringFunc(desc, func(block string) string {
		out := zoomDescriptionURLRE.ReplaceAllStringFunc(block, zoom.RedactZoomURL)
		return zoomDescriptionPasscodeRE.ReplaceAllString(out, "${1}REDACTED")
	})
}

// extractZoomMeetingIDFromDescription returns the meeting ID embedded in the
// gog-zoom-meeting start marker, if present.
func extractZoomMeetingIDFromDescription(desc string) (string, bool) {
	if desc == "" {
		return "", false
	}
	m := zoomDescriptionBlockRE.FindStringSubmatch(desc)
	if len(m) < 2 {
		return "", false
	}
	id := strings.TrimSpace(m[1])
	if id == "" {
		return "", false
	}
	return id, true
}

// descriptionHasZoomBlock reports whether the description contains a
// gog-managed Zoom block.
func descriptionHasZoomBlock(desc string) bool {
	return zoomDescriptionBlockRE.MatchString(desc)
}

// descriptionForPatch returns the description to base a description-mutation
// patch on. If the patch already carries a description (the caller is editing
// it), use that as the starting point; otherwise fall back to the existing
// event's description. The existing event is fetched once per patch flow.
func descriptionForPatch(existing, patch *calendar.Event) string {
	if patch != nil && strings.TrimSpace(patch.Description) != "" {
		return patch.Description
	}
	if existing != nil {
		return existing.Description
	}
	return ""
}
