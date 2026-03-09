package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	transparencyOpaque      = "opaque"
	transparencyTransparent = "transparent"
	visibilityPublic        = "public"
	sendUpdatesNone         = "none"
)

func validateColorId(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return "", fmt.Errorf("invalid color ID: %q (must be 1-11)", s)
	}
	if id < 1 || id > 11 {
		return "", fmt.Errorf("color ID must be 1-11 (got %d)", id)
	}
	return s, nil
}

func validateCalendarColorId(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return "", fmt.Errorf("invalid calendar color ID: %q (must be 1-24)", s)
	}
	if id < 1 || id > 24 {
		return "", fmt.Errorf("calendar color ID must be 1-24 (got %d)", id)
	}
	return s, nil
}

func validateVisibility(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", nil
	}
	valid := map[string]bool{
		"default":      true,
		"public":       true,
		"private":      true,
		"confidential": true,
	}
	if !valid[s] {
		return "", fmt.Errorf("invalid visibility: %q (must be default, public, private, or confidential)", s)
	}
	return s, nil
}

func validateTransparency(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", nil
	}
	switch s {
	case "busy":
		return transparencyOpaque, nil
	case "free":
		return transparencyTransparent, nil
	case transparencyOpaque, transparencyTransparent:
		return s, nil
	default:
		return "", fmt.Errorf("invalid transparency: %q (must be opaque/busy or transparent/free)", s)
	}
}

func validateSendUpdates(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	switch strings.ToLower(s) {
	case scopeAll:
		return scopeAll, nil
	case "externalonly":
		return "externalOnly", nil
	case sendUpdatesNone:
		return sendUpdatesNone, nil
	default:
		return "", fmt.Errorf("invalid send-updates value: %q (must be all, externalOnly, or none)", s)
	}
}
