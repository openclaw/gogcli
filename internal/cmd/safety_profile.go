package cmd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"gopkg.in/yaml.v3"
)

type bakedSafetyProfile struct {
	enabled bool
	name    string
	allow   map[string]bool
	deny    map[string]bool
}

func enforceBakedSafetyProfile(kctx *kong.Context) error {
	profile, err := loadBakedSafetyProfile()
	if err != nil {
		return usagef("invalid baked safety profile: %v", err)
	}
	if !profile.enabled {
		return nil
	}

	path := commandPath(kctx.Command())
	if len(path) == 0 {
		return nil
	}
	command := strings.Join(path, " ")
	if commandPathMatches(profile.deny, path) {
		return usagef("command %q is blocked by baked safety profile %q", command, profile.name)
	}
	if len(profile.allow) > 0 && !commandPathMatches(profile.allow, path) {
		return usagef("command %q is not included in baked safety profile %q", command, profile.name)
	}
	return nil
}

func loadBakedSafetyProfile() (bakedSafetyProfile, error) {
	raw := strings.TrimSpace(bakedSafetyProfileYAML)
	if raw == "" {
		return bakedSafetyProfile{}, nil
	}
	profile, err := parseSafetyProfile(raw)
	if err != nil {
		return bakedSafetyProfile{}, err
	}
	return *profile, nil
}

func ValidateSafetyProfile(raw string) error {
	_, err := parseSafetyProfile(raw)
	return err
}

func parseSafetyProfile(raw string) (*bakedSafetyProfile, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, err
	}

	profile := &bakedSafetyProfile{
		enabled: true,
		name:    "unnamed",
		allow:   map[string]bool{},
		deny:    map[string]bool{},
	}

	if name, ok := root["name"].(string); ok && strings.TrimSpace(name) != "" {
		profile.name = strings.TrimSpace(name)
	}
	if err := addSafetyProfileList(profile.allow, root["allow"]); err != nil {
		return nil, fmt.Errorf("allow: %w", err)
	}
	if err := addSafetyProfileList(profile.deny, root["deny"]); err != nil {
		return nil, fmt.Errorf("deny: %w", err)
	}

	for key, value := range root {
		switch key {
		case "name", "description", "allow", "deny":
			continue
		}
		prefix := []string{key}
		if key == "aliases" {
			prefix = nil
		}
		if err := flattenSafetyProfileNode(profile, prefix, value); err != nil {
			return nil, err
		}
	}

	if len(profile.allow) == 0 && len(profile.deny) == 0 {
		return nil, fmt.Errorf("profile has no allow or deny entries")
	}
	return profile, nil
}

func addSafetyProfileList(out map[string]bool, value any) error {
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("expected list")
	}
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return fmt.Errorf("expected string item")
		}
		rule := normalizeSafetyProfileRule(s)
		if rule != "" {
			out[rule] = true
		}
	}
	return nil
}

func flattenSafetyProfileNode(profile *bakedSafetyProfile, prefix []string, value any) error {
	switch typed := value.(type) {
	case bool:
		rule := normalizeSafetyProfileRule(strings.Join(prefix, "."))
		if rule == "" {
			return fmt.Errorf("empty safety profile command path")
		}
		if typed {
			profile.allow[rule] = true
		} else {
			profile.deny[rule] = true
		}
		return nil
	case map[string]any:
		for key, child := range typed {
			next := append(append([]string{}, prefix...), key)
			if err := flattenSafetyProfileNode(profile, next, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported safety profile value at %q", strings.Join(prefix, "."))
	}
}

func normalizeSafetyProfileRule(rule string) string {
	rule = strings.TrimSpace(strings.ToLower(rule))
	rule = strings.ReplaceAll(rule, " ", ".")
	rule = strings.Trim(rule, ".")
	return rule
}
