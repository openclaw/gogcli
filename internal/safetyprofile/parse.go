package safetyprofile

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const literalAll = "all"

// Parse validates the YAML form of a safety profile and returns the Profile
// the generator emits as code. Allow and deny lists are sorted and
// deduplicated; "all" / "*" entries on the allow side collapse into the
// AllowAll flag. Wildcards on the deny side or as a parent of a nested rule
// are rejected because they would be silent no-ops in the hashed runtime
// switch.
func Parse(raw string) (*Profile, error) {
	parsed, err := parseRaw(raw)
	if err != nil {
		return nil, err
	}
	out := &Profile{
		Name:     parsed.name,
		AllowAll: parsed.allow[literalAll] || parsed.allow["*"],
	}
	for k := range parsed.allow {
		if k == literalAll || k == "*" {
			continue
		}
		out.AllowRules = append(out.AllowRules, k)
	}
	for k := range parsed.deny {
		out.DenyRules = append(out.DenyRules, k)
	}
	sort.Strings(out.AllowRules)
	sort.Strings(out.DenyRules)
	return out, nil
}

type rawProfile struct {
	name  string
	allow map[string]bool
	deny  map[string]bool
}

func parseRaw(raw string) (*rawProfile, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, err
	}

	profile := &rawProfile{
		name:  "unnamed",
		allow: map[string]bool{},
		deny:  map[string]bool{},
	}

	if rawName, present := root["name"]; present {
		name, ok := rawName.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("name: expected non-empty string, got %T", rawName)
		}
		profile.name = strings.TrimSpace(name)
	}
	if err := addList(profile.allow, root["allow"]); err != nil {
		return nil, fmt.Errorf("allow: %w", err)
	}
	if err := addList(profile.deny, root["deny"]); err != nil {
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
		if err := flatten(profile, prefix, value); err != nil {
			return nil, err
		}
	}

	if profile.deny[literalAll] || profile.deny["*"] {
		return nil, fmt.Errorf("deny: wildcards %q and %q are not allowed; list specific commands or remove the entry", literalAll, "*")
	}

	if len(profile.allow) == 0 && len(profile.deny) == 0 {
		return nil, fmt.Errorf("profile has no allow or deny entries")
	}
	return profile, nil
}

func addList(out map[string]bool, value any) error {
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
		rule := normalize(s)
		if rule != "" {
			out[rule] = true
		}
	}
	return nil
}

func flatten(profile *rawProfile, prefix []string, value any) error {
	switch typed := value.(type) {
	case bool:
		rule := normalize(strings.Join(prefix, "."))
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
		if len(prefix) > 0 {
			root := normalize(prefix[0])
			if root == literalAll || root == "*" {
				return fmt.Errorf("safety profile rules cannot be nested under wildcard %q at %q; list specific commands", prefix[0], strings.Join(prefix, "."))
			}
		}
		for key, child := range typed {
			next := append(append([]string{}, prefix...), key)
			if err := flatten(profile, next, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported safety profile value at %q", strings.Join(prefix, "."))
	}
}

func normalize(rule string) string {
	rule = strings.TrimSpace(strings.ToLower(rule))
	rule = strings.ReplaceAll(rule, " ", ".")
	rule = strings.Trim(rule, ".")
	return rule
}
