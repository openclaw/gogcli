package cmd

import (
	"errors"
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

func looksLikeCustomLabelID(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(trimmed), "label_") {
		return false
	}

	_, err := strconv.ParseInt(trimmed[len("Label_"):], 10, 64)
	return err == nil
}

func ensureLabelNameAvailable(svc *gmail.Service, name string) error {
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	if _, ok := idMap[strings.ToLower(name)]; ok {
		return usagef("label already exists: %s", name)
	}
	return nil
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
