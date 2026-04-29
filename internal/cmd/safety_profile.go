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
	if !profile.allowsCommandPath(path) {
		return profile.commandPathError(path)
	}
	return nil
}

func bakedSafetyProfileError(path []string, profileName string, included bool) error {
	command := strings.Join(path, " ")
	if included {
		return usagef("command %q is blocked by baked safety profile %q", command, profileName)
	}
	return usagef("command %q is not included in baked safety profile %q", command, profileName)
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

func (p bakedSafetyProfile) allowsCommandPath(path []string) bool {
	if !p.enabled || len(path) == 0 {
		return true
	}
	if commandPathMatches(p.deny, path) {
		return false
	}
	if len(p.allow) == 0 {
		return true
	}
	return commandPathMatches(p.allow, path)
}

func (p bakedSafetyProfile) commandPathError(path []string) error {
	if commandPathMatches(p.deny, path) {
		return bakedSafetyProfileError(path, p.name, true)
	}
	return bakedSafetyProfileError(path, p.name, false)
}

func (p bakedSafetyProfile) commandNodeVisible(node *kong.Node) bool {
	if !p.enabled || node == nil {
		return true
	}
	if node.Type == kong.ApplicationNode {
		return true
	}
	path := commandNodePath(node)
	if len(path) > 0 && p.allowsCommandPath(path) {
		return true
	}
	return p.commandNodeHasVisibleChildren(node)
}

func (p bakedSafetyProfile) commandNodeBlockedForHelp(node *kong.Node) bool {
	if !p.enabled || node == nil || node.Type != kong.CommandNode {
		return false
	}
	path := commandNodePath(node)
	if len(path) == 0 || p.allowsCommandPath(path) {
		return false
	}
	return !p.commandNodeHasVisibleChildren(node)
}

func (p bakedSafetyProfile) commandNodeHasVisibleChildren(node *kong.Node) bool {
	for _, child := range node.Children {
		if child == nil || child.Type != kong.CommandNode {
			continue
		}
		if p.commandNodeVisible(child) {
			return true
		}
	}
	return false
}

func commandNodePath(node *kong.Node) []string {
	if node == nil {
		return nil
	}
	var rev []string
	for cur := node; cur != nil && cur.Type != kong.ApplicationNode; cur = cur.Parent {
		if cur.Type == kong.CommandNode && strings.TrimSpace(cur.Name) != "" {
			rev = append(rev, strings.ToLower(strings.TrimSpace(cur.Name)))
		}
	}
	path := make([]string, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}
	return path
}

func applySafetyProfileVisibility(root *kong.Node, profile bakedSafetyProfile) func() {
	if !profile.enabled || root == nil {
		return func() {}
	}
	type hiddenState struct {
		node   *kong.Node
		hidden bool
	}
	restore := []hiddenState{}
	var walk func(*kong.Node)
	walk = func(node *kong.Node) {
		for _, child := range node.Children {
			if child == nil || child.Type != kong.CommandNode {
				continue
			}
			restore = append(restore, hiddenState{node: child, hidden: child.Hidden})
			if !profile.commandNodeVisible(child) {
				child.Hidden = true
			}
			walk(child)
		}
	}
	walk(root)
	return func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i].node.Hidden = restore[i].hidden
		}
	}
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
