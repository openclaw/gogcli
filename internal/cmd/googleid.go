package cmd

import (
	"net/url"
	"strings"
)

// normalizeGoogleID extracts a Drive/Docs/Sheets/Slides file ID from common Google URLs.
// If it can't recognize the input as a supported URL, it returns the trimmed input unchanged.
func normalizeGoogleID(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}

	u := parseMaybeURL(s)
	if u == nil {
		return s
	}

	host := strings.ToLower(strings.TrimSpace(u.Host))
	host = strings.TrimPrefix(host, "www.")

	// Query-based IDs
	if host == "drive.google.com" {
		if id := strings.TrimSpace(u.Query().Get("id")); id != "" {
			return id
		}
	}

	// Path-based IDs
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return s
	}

	switch host {
	case "docs.google.com":
		// Examples:
		// /document/d/<id>/edit
		// /spreadsheets/d/<id>/edit
		// /presentation/d/<id>/edit
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "d" {
				if id := strings.TrimSpace(parts[i+1]); id != "" {
					return id
				}
			}
		}
	case "drive.google.com":
		// Examples:
		// /file/d/<id>/view
		// /drive/folders/<id>
		// /drive/u/0/folders/<id>
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "d" {
				if id := strings.TrimSpace(parts[i+1]); id != "" {
					return id
				}
			}
		}
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "folders" {
				if id := strings.TrimSpace(parts[i+1]); id != "" {
					return id
				}
			}
		}
	}

	return s
}

func parseMaybeURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err == nil && u != nil && u.Scheme != "" && u.Host != "" {
		return u
	}

	// Handle scheme-less pastes like "docs.google.com/document/d/..."
	if strings.HasPrefix(s, "drive.google.com/") ||
		strings.HasPrefix(s, "www.drive.google.com/") ||
		strings.HasPrefix(s, "docs.google.com/") ||
		strings.HasPrefix(s, "www.docs.google.com/") ||
		strings.HasPrefix(s, "mail.google.com/") ||
		strings.HasPrefix(s, "www.mail.google.com/") ||
		strings.HasPrefix(s, "gmail.google.com/") ||
		strings.HasPrefix(s, "www.gmail.google.com/") ||
		strings.HasPrefix(s, "calendar.google.com/") ||
		strings.HasPrefix(s, "www.calendar.google.com/") {
		u2, err2 := url.Parse("https://" + s)
		if err2 == nil && u2 != nil && u2.Host != "" {
			return u2
		}
	}

	return nil
}
