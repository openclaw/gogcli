//go:build !safety_profile

package cmd

// bakedSafetyTestProfile is the test-only override that backs the
// bakedSafety* package-level functions in non-safety builds. Production
// safety_profile builds compile safety_profile_baked_gen.go instead, which
// resolves these functions to a generated hash switch and never reads this
// variable. Tests in this package mutate the struct via withBakedSafetyProfile
// to set up scenarios; stock binaries leave it zeroed and the profile reports
// disabled.
var bakedSafetyTestProfile struct {
	enabled       bool
	name          string
	hasAllowRules bool
	allowAll      bool
	allow         map[string]bool
	deny          map[string]bool
}

func bakedSafetyEnabled() bool       { return bakedSafetyTestProfile.enabled }
func bakedSafetyProfileName() string { return bakedSafetyTestProfile.name }
func bakedSafetyHasAllowRules() bool { return bakedSafetyTestProfile.hasAllowRules }

func bakedSafetyAllowMatch(path []string) bool {
	if bakedSafetyTestProfile.allowAll {
		return true
	}
	return commandPathMatches(bakedSafetyTestProfile.allow, path)
}

func bakedSafetyDenyMatch(path []string) bool {
	return commandPathMatches(bakedSafetyTestProfile.deny, path)
}
