package cmd

import (
	"encoding/base64"
	"math/big"
	"slices"
	"strconv"
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
		if id, ok := decodeGmailViewToken(s, gmailThreadTokenPrefixes); ok {
			return id
		}
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
	if id, ok := decodeGmailViewToken(last, gmailThreadTokenPrefixes); ok {
		return id
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
		if id, ok := decodeGmailViewToken(s, gmailMessageTokenPrefixes); ok {
			return id
		}
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

	frag := strings.SplitN(strings.TrimSpace(u.Fragment), "?", 2)[0]
	if parts := strings.Split(strings.Trim(frag, "/"), "/"); len(parts) > 0 {
		if id, ok := decodeGmailViewToken(parts[len(parts)-1], gmailMessageTokenPrefixes); ok {
			return id
		}
	}

	return s
}

const (
	gmailViewTokenAlphabet = "BCDFGHJKLMNPQRSTVWXZbcdfghjklmnpqrstvwxz"
	base64StdAlphabet      = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

var (
	gmailThreadTokenPrefixes  = []string{"f:", "thread-f:"}
	gmailMessageTokenPrefixes = []string{"msg-f:"}
)

// decodeGmailViewToken resolves the opaque token of current Gmail web URLs
// (e.g. #inbox/FMfcgz...) to a hex API ID. The token is a base64 string
// re-encoded in a consonant-only base-40 alphabet; its payload is
// "<prefix><decimal id>".
func decodeGmailViewToken(token string, prefixes []string) (string, bool) {
	token = strings.TrimSpace(token)
	if len(token) < 16 || looksLikeHexID(token) {
		return "", false
	}

	n := new(big.Int)
	base := big.NewInt(int64(len(gmailViewTokenAlphabet)))
	for _, r := range token {
		i := strings.IndexRune(gmailViewTokenAlphabet, r)
		if i < 0 {
			return "", false
		}
		n.Mul(n, base).Add(n, big.NewInt(int64(i)))
	}

	var b64 []byte
	sixtyFour := big.NewInt(64)
	rem := new(big.Int)
	for n.Sign() > 0 {
		n.DivMod(n, sixtyFour, rem)
		b64 = append(b64, base64StdAlphabet[rem.Int64()])
	}
	slices.Reverse(b64)
	payload, err := base64.RawStdEncoding.DecodeString(string(b64))
	if err != nil {
		return "", false
	}

	for _, prefix := range prefixes {
		dec, ok := strings.CutPrefix(string(payload), prefix)
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(dec, 10, 64)
		if err != nil {
			return "", false
		}
		return strconv.FormatUint(id, 16), true
	}
	return "", false
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
