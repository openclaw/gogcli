package cmd

import (
	"encoding/base64"
	"strings"
	"unicode"
)

func normalizeGmailThreadID(input string) string {
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
	if host != "mail.google.com" && host != "gmail.google.com" {
		return s
	}

	// Query-based thread IDs (classic links use `th=`).
	if th := strings.TrimSpace(u.Query().Get("th")); looksLikeHexID(th) {
		return th
	}

	// Fragment-based thread IDs:
	// https://mail.google.com/mail/u/0/#inbox/<threadId>
	frag := strings.TrimSpace(u.Fragment)
	if frag == "" {
		return s
	}

	frag = strings.SplitN(frag, "?", 2)[0]
	parts := strings.Split(strings.Trim(frag, "/"), "/")
	if len(parts) == 0 {
		return s
	}

	last := strings.TrimSpace(parts[len(parts)-1])
	if looksLikeHexID(last) {
		return last
	}
	return s
}

func normalizeGmailMessageID(input string) string {
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
	if host != "mail.google.com" && host != "gmail.google.com" {
		return s
	}

	q := u.Query()
	if id := strings.TrimSpace(q.Get("message_id")); looksLikeHexID(id) {
		return id
	}
	if id := strings.TrimSpace(q.Get("msg")); looksLikeHexID(id) {
		return id
	}
	if raw := strings.TrimSpace(q.Get("permmsgid")); raw != "" {
		// Best-effort: some links use `permmsgid=msg-f:<id>`.
		if i := strings.LastIndex(raw, ":"); i != -1 && i+1 < len(raw) {
			raw = raw[i+1:]
		}
		raw = strings.TrimSpace(raw)
		if looksLikeHexID(raw) {
			return raw
		}
	}

	return s
}

func normalizeCalendarEventID(input string) string {
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
	if host != "calendar.google.com" {
		return s
	}

	// Query-based event IDs: `eid=` is base64-encoded and typically includes
	// "<eventId> <calendarId>".
	if eid := strings.TrimSpace(u.Query().Get("eid")); eid != "" {
		if decoded, ok := decodeBase64URLString(eid); ok {
			fields := strings.Fields(decoded)
			if len(fields) > 0 && strings.TrimSpace(fields[0]) != "" {
				return strings.TrimSpace(fields[0])
			}
		}
	}

	// Path-based event IDs:
	// https://calendar.google.com/calendar/u/0/r/eventedit/<eventId>
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "eventedit" {
			if id := strings.TrimSpace(parts[i+1]); id != "" {
				return id
			}
		}
	}

	return s
}

func decodeBase64URLString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// Some encodings include padding.
		b, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			return "", false
		}
	}
	return string(b), true
}

func looksLikeHexID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return false
	}
	for _, r := range s {
		if unicode.IsDigit(r) {
			continue
		}
		if r >= 'a' && r <= 'f' {
			continue
		}
		if r >= 'A' && r <= 'F' {
			continue
		}
		return false
	}
	return true
}
