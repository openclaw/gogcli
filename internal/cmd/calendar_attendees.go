package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"
)

func buildAttendees(csv string) []*calendar.EventAttendee {
	addrs := splitCSV(csv)
	if len(addrs) == 0 {
		return nil
	}
	out := make([]*calendar.EventAttendee, 0, len(addrs))
	for _, a := range addrs {
		attendee := parseAttendee(a)
		if attendee != nil {
			out = append(out, attendee)
		}
	}
	return out
}

// mergeAttendees preserves existing attendees (with all their metadata like responseStatus)
// and adds new attendees from the CSV string. Duplicates (by email) are skipped.
func mergeAttendees(existing []*calendar.EventAttendee, addCSV string) []*calendar.EventAttendee {
	out, _ := mergeAttendeesWithChange(existing, addCSV)
	return out
}

// mergeAttendeesWithChange returns the merged attendees and whether at least one attendee was added.
func mergeAttendeesWithChange(existing []*calendar.EventAttendee, addCSV string) ([]*calendar.EventAttendee, bool) {
	newAttendees := buildAttendees(addCSV)
	if len(newAttendees) == 0 {
		return existing, false
	}

	// Build a set of existing emails for deduplication
	existingEmails := make(map[string]bool, len(existing))
	for _, a := range existing {
		if a != nil && a.Email != "" {
			existingEmails[strings.ToLower(a.Email)] = true
		}
	}

	// Start with existing attendees (preserving all metadata)
	out := make([]*calendar.EventAttendee, 0, len(existing)+len(newAttendees))
	out = append(out, existing...)

	// Add new attendees that don't already exist
	added := false
	for _, attendee := range newAttendees {
		if attendee == nil || attendee.Email == "" {
			continue
		}
		email := attendee.Email
		if !existingEmails[strings.ToLower(email)] {
			attendee.ResponseStatus = taskStatusNeedsAction
			out = append(out, attendee)
			existingEmails[strings.ToLower(email)] = true
			added = true
		}
	}
	return out, added
}

func parseAttendee(s string) *calendar.EventAttendee {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	email := strings.TrimSpace(parts[0])
	if email == "" {
		return nil
	}

	attendee := &calendar.EventAttendee{Email: email}
	for _, p := range parts[1:] {
		raw := strings.TrimSpace(p)
		lower := strings.ToLower(raw)
		if lower == "optional" {
			attendee.Optional = true
			continue
		}
		if lower == "resource" {
			attendee.Resource = true
			continue
		}
		if strings.HasPrefix(lower, "comment=") {
			attendee.Comment = strings.TrimSpace(raw[len("comment="):])
		}
	}
	return attendee
}
