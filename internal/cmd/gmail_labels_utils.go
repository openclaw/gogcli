package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

func resolveLabelIDs(labels []string, nameToID map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		if trimmed == "" {
			continue
		}
		if nameToID != nil {
			if id, ok := nameToID[strings.ToLower(trimmed)]; ok {
				out = append(out, id)
				continue
			}
		}
		out = append(out, trimmed)
	}
	return out
}

func resolveModifyLabelIDs(svc *gmail.Service, addLabels, removeLabels []string) ([]string, []string, error) {
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return nil, nil, err
	}

	return resolveLabelIDs(addLabels, idMap), resolveLabelIDs(removeLabels, idMap), nil
}

func resolveMutableGmailLabel(ctx context.Context, svc *gmail.Service, raw string) (*gmail.Label, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, usage("label is required")
	}

	label, err := svc.Users.Labels.Get("me", raw).Context(ctx).Do()
	if err == nil {
		return label, nil
	}
	if !isNotFoundAPIError(err) {
		return nil, err
	}
	if looksLikeCustomLabelID(raw) {
		return nil, fmt.Errorf("label not found: %s", raw)
	}

	idMap, mapErr := fetchLabelNameOnlyToID(svc)
	if mapErr != nil {
		return nil, mapErr
	}
	id, ok := idMap[strings.ToLower(raw)]
	if !ok {
		return nil, fmt.Errorf("label not found: %s", raw)
	}
	return svc.Users.Labels.Get("me", id).Context(ctx).Do()
}

func normalizeGmailLabelHexColor(raw, field string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) != 7 || raw[0] != '#' {
		return "", usagef("%s must be a #RRGGBB hex color", field)
	}
	for _, r := range raw[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return "", usagef("%s must be a #RRGGBB hex color", field)
		}
	}
	return strings.ToLower(raw), nil
}

func looksLikeCustomLabelID(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(trimmed), "label_") {
		return false
	}

	_, err := strconv.ParseInt(trimmed[len("Label_"):], 10, 64)
	return err == nil
}

func ensureLabelNameAvailable(svc *gmail.Service, name string) error {
	resp, err := svc.Users.Labels.List("me").Fields("labels(id,name)").Do()
	if err != nil {
		return err
	}

	want := strings.ToLower(strings.TrimSpace(name))
	wantCollision := gmailLabelNameCollisionKey(name)
	for _, label := range resp.Labels {
		if label == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(label.Id)) == want {
			return usagef("label already exists: %s", name)
		}
		labelName := strings.TrimSpace(label.Name)
		if strings.ToLower(labelName) == want || gmailLabelNameCollisionKey(labelName) == wantCollision {
			return usagef("label already exists: %s", name)
		}
	}
	return nil
}

func gmailLabelNameCollisionKey(name string) string {
	// Gmail accepts slash-separated nested labels, but the API rejects names that
	// collide after slash-to-hyphen normalization (for example a/b vs a-b).
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "/", "-")
}

func mapLabelCreateError(err error, name string) error {
	if err == nil {
		return nil
	}
	if isDuplicateLabelError(err) {
		return usagef("label already exists: %s", name)
	}
	return err
}

func isDuplicateLabelError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusConflict {
			if labelAlreadyExistsMessage(gerr.Message) {
				return true
			}
			for _, item := range gerr.Errors {
				if labelAlreadyExistsMessage(item.Message) || labelDuplicateReason(item.Reason) {
					return true
				}
			}
		}
		if labelAlreadyExistsMessage(gerr.Message) {
			return true
		}
		for _, item := range gerr.Errors {
			if labelAlreadyExistsMessage(item.Message) || labelDuplicateReason(item.Reason) {
				return true
			}
		}
	}
	return labelAlreadyExistsMessage(err.Error())
}

func labelAlreadyExistsMessage(msg string) bool {
	low := strings.ToLower(msg)
	if !strings.Contains(low, "label") {
		return false
	}
	return strings.Contains(low, "name exists") ||
		strings.Contains(low, "already exists") ||
		strings.Contains(low, "duplicate")
}

func labelDuplicateReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "duplicate", "alreadyexists":
		return true
	default:
		return false
	}
}
