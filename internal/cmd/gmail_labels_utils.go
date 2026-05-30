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
			if id, ok := nameToID[trimmed]; ok {
				out = append(out, id)
				continue
			}
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
	normalized := strings.ToLower(raw)
	if _, ok := allowedGmailLabelPalette[normalized]; !ok {
		return "", usagef("%s must be one of Gmail's label palette colors, for example #000000, #fce8b3, or #16a765", field)
	}
	return normalized, nil
}

var allowedGmailLabelPalette = map[string]struct{}{
	"#000000": {}, "#434343": {}, "#666666": {}, "#999999": {}, "#cccccc": {}, "#efefef": {}, "#f3f3f3": {}, "#ffffff": {},
	"#fb4c2f": {}, "#ffad47": {}, "#fad165": {}, "#16a766": {}, "#43d692": {}, "#4a86e8": {}, "#a479e2": {}, "#f691b3": {},
	"#f6c5be": {}, "#ffe6c7": {}, "#fef1d1": {}, "#b9e4d0": {}, "#c6f3de": {}, "#c9daf8": {}, "#e4d7f5": {}, "#fcdee8": {},
	"#efa093": {}, "#ffd6a2": {}, "#fce8b3": {}, "#89d3b2": {}, "#a0eac9": {}, "#a4c2f4": {}, "#d0bcf1": {}, "#fbc8d9": {},
	"#e66550": {}, "#ffbc6b": {}, "#fcda83": {}, "#44b984": {}, "#68dfa9": {}, "#6d9eeb": {}, "#b694e8": {}, "#f7a7c0": {},
	"#cc3a21": {}, "#eaa041": {}, "#f2c960": {}, "#149e60": {}, "#3dc789": {}, "#3c78d8": {}, "#8e63ce": {}, "#e07798": {},
	"#ac2b16": {}, "#cf8933": {}, "#d5ae49": {}, "#0b804b": {}, "#2a9c68": {}, "#285bac": {}, "#653e9b": {}, "#b65775": {},
	"#822111": {}, "#a46a21": {}, "#aa8831": {}, "#076239": {}, "#1a764d": {}, "#1c4587": {}, "#41236d": {}, "#83334c": {},
	"#464646": {}, "#e7e7e7": {}, "#0d3472": {}, "#b6cff5": {}, "#0d3b44": {}, "#98d7e4": {}, "#3d188e": {}, "#e3d7ff": {},
	"#711a36": {}, "#fbd3e0": {}, "#8a1c0a": {}, "#f2b2a8": {}, "#7a2e0b": {}, "#ffc8af": {}, "#7a4706": {}, "#ffdeb5": {},
	"#594c05": {}, "#fbe983": {}, "#684e07": {}, "#fdedc1": {}, "#0b4f30": {}, "#b3efd3": {}, "#04502e": {}, "#a2dcc1": {},
	"#c2c2c2": {}, "#4986e7": {}, "#2da2bb": {}, "#b99aff": {}, "#994a64": {}, "#f691b2": {}, "#ff7537": {}, "#ffad46": {},
	"#662e37": {}, "#ebdbde": {}, "#cca6ac": {}, "#094228": {}, "#42d692": {}, "#16a765": {},
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
		if strings.TrimSpace(label.Id) == strings.TrimSpace(name) {
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
