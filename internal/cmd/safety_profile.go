package cmd

import (
	"hash/fnv"
	"strings"

	"github.com/alecthomas/kong"
)

type bakedSafetyProfile struct {
	enabled bool
	name    string
}

// bakedSafetyHashPath returns the FNV-64a hash of the dotted command path.
// The generated allow/deny matchers switch on these hashes so that rule
// strings never appear in the binary's data section. The build-time
// generator hashes via internal/safetyprofile.HashRule; both call hash/fnv
// over the same input, and TestSafetyProfileHashAgreement asserts they
// produce identical values.
func bakedSafetyHashPath(parts []string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join(parts, ".")))
	return h.Sum64()
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

// loadBakedSafetyProfile constructs a profile handle from the package-level
// hooks supplied by either the generated safety_profile_baked_gen.go (for
// safety_profile builds) or safety_profile_default.go (for stock and test
// builds). The error result is retained for compatibility with the upstream
// caller signatures; the profile is validated by cmd/bake-safety-profile at
// build time, so the runtime path cannot fail.
//
//nolint:unparam // error preserved to keep upstream caller signatures unchanged.
func loadBakedSafetyProfile() (bakedSafetyProfile, error) {
	return bakedSafetyProfile{
		enabled: bakedSafetyEnabled(),
		name:    bakedSafetyProfileName(),
	}, nil
}

func (p bakedSafetyProfile) allowsCommandPath(path []string) bool {
	if !p.enabled || len(path) == 0 {
		return true
	}
	if bakedSafetyDenyMatch(path) {
		return false
	}
	if !bakedSafetyHasAllowRules() {
		return true
	}
	return bakedSafetyAllowMatch(path)
}

func (p bakedSafetyProfile) commandPathError(path []string) error {
	if bakedSafetyDenyMatch(path) {
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
