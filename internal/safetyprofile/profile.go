// Package safetyprofile parses and hashes baked safety profile YAML for the
// build-time generator (cmd/bake-safety-profile). It is intentionally outside
// internal/cmd so the runtime CLI binary does not pull in the parser or its
// dependencies.
package safetyprofile

// Profile is the build-time intermediate representation that the generator
// consumes to emit the hash-based allow and deny switches. Rules are sorted
// and deduplicated; AllowAll captures the "all" / "*" wildcard so the
// generator can emit a constant-true matcher instead of enumerating every
// command.
type Profile struct {
	Name       string
	AllowAll   bool
	AllowRules []string
	DenyRules  []string
	// LockedFlags fix a flag to a value the command line cannot change. Sorted by
	// name so the generated switch is stable across builds.
	LockedFlags []LockedFlag
	// DeniedValues refuse specific values of string flags on specific commands.
	// Sorted by path then flag so the generated switch is stable across builds.
	DeniedValues []DeniedValue
}

// DeniedValue is one flag of one command and the values it may not be given.
// Path is the canonical dotted command path, Flag the canonical flag name, and
// Values the lowercased, deduplicated, sorted values to refuse.
type DeniedValue struct {
	Path   string
	Flag   string
	Values []string
}

// LockedFlag is one locked boolean flag. Value is "true" or "false" in the form
// the flag parser consumes.
type LockedFlag struct {
	Name  string
	Value string
}
