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
}
