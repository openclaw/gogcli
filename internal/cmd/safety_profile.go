package cmd

import (
	"fmt"
	"hash/fnv"
	"sort"
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

// lockedFlagNames records the flags enforceLockedFlags set, so a command rejecting a
// value it never received on the command line can say where that value came from.
var lockedFlagNames = map[string]bool{}

// lockedFlagsNote names the locked flags for display beneath a usage error. A command
// can reject a combination involving a value the caller never passed, so the note is
// what explains where that value came from. Empty when nothing is locked.
func lockedFlagsNote() string {
	if len(lockedFlagNames) == 0 {
		return ""
	}
	names := make([]string, 0, len(lockedFlagNames))
	for name := range lockedFlagNames {
		names = append(names, "--"+name)
	}
	sort.Strings(names)
	return fmt.Sprintf("note: %s locked by baked safety profile %q", strings.Join(names, ", "), bakedSafetyProfileName())
}

// verifyLockedFlagsExist refuses to run when a locked name matches no flag in the
// CLI. Enforcement only ever asks whether a given flag is locked, so a misspelled
// name would lock nothing at all and the profile would claim a guarantee it does not
// have. Counting the matches catches that without the names appearing in the binary.
func verifyLockedFlagsExist(kctx *kong.Context) error {
	want := bakedSafetyLockedFlagCount()
	if want == 0 {
		return nil
	}
	seen := map[string]bool{}
	var unsupported error
	var walk func(node *kong.Node)
	walk = func(node *kong.Node) {
		if node == nil {
			return
		}
		for _, flag := range node.Flags {
			if _, locked := bakedSafetyLockedFlag(flag.Name); !locked {
				continue
			}
			seen[flag.Name] = true
			if err := lockUnsupported(flag); err != nil && unsupported == nil {
				unsupported = err
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(kctx.Model.Node)
	if len(seen) < want {
		return usagef("baked safety profile %q locks %d flag(s) but only %d exist; check the locked-flags names", bakedSafetyProfileName(), want, len(seen))
	}
	return unsupported
}

// lockUnsupported names the flags a lock cannot reach, because their value is
// consumed before locks are applied. Refusing them is honest: the alternative is a
// profile that reports a guarantee the run never had.
func lockUnsupported(flag *kong.Flag) error {
	switch {
	case flag.Name == "home":
		// The layout resolver reads --home straight from argv before Kong parses, so
		// config and credential roots are already chosen by the time locks run.
		return usagef("baked safety profile %q locks --home, which is read before flags are parsed and cannot be locked", bakedSafetyProfileName())
	case flag.Required:
		// Kong rejects a missing required flag during parsing, so a locked one fails
		// when omitted and is refused as an override when supplied.
		return usagef("baked safety profile %q locks required flag --%s, which must be supplied on the command line", bakedSafetyProfileName(), flag.Name)
	}
	return nil
}

// enforceLockedFlags applies the profile's locked flag values and refuses a command
// line that sets one of them. The value is locked rather than merely defaulted so it
// holds without help from the environment, which the caller may not control.
func enforceLockedFlags(kctx *kong.Context) error {
	// Rebuilt per parse: carrying names over would let one run's note describe a
	// profile that is not in force.
	lockedFlagNames = map[string]bool{}
	if !bakedSafetyEnabled() {
		return nil
	}
	for _, flag := range kctx.Flags() {
		value, locked := bakedSafetyLockedFlag(flag.Name)
		if !locked {
			continue
		}
		if flagOnCommandLine(kctx, flag.Name) {
			return usagef("flag --%s is locked by baked safety profile %q", flag.Name, bakedSafetyProfileName())
		}
		if err := flag.Parse(kong.ScanFromTokens(kong.Token{Type: kong.FlagValueToken, Value: value}), flag.Target); err != nil {
			// The value is compiled policy data. Do not echo it through parser errors:
			// profiles can lock arbitrary strings, and the caller may not be allowed to
			// learn the configured value even when the profile is malformed.
			return usagef("locked flag --%s has a value that is invalid for that flag", flag.Name)
		}
		lockedFlagNames[flag.Name] = true
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
