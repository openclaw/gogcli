package cmd

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"google.golang.org/api/gmail/v1"
)

func parseDurationSeconds(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	if !strings.ContainsFunc(trimmed, unicode.IsLetter) {
		value, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(value) * time.Second, nil
	}
	return time.ParseDuration(trimmed)
}

func truncateUTF8Bytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", false
	}
	b := []byte(s)
	if len(b) <= maxBytes {
		return s, false
	}
	b = b[:maxBytes]
	for !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b), true
}

func formatUnixMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
}

func resolveLabelIDsWithService(svc *gmail.Service, labels []string) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	nameToID, err := fetchLabelNameToID(svc)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		if trimmed == "" {
			continue
		}
		if id, ok := nameToID[strings.ToLower(trimmed)]; ok {
			out = append(out, id)
			continue
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func stringSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
