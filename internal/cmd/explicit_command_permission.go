package cmd

import "strings"

// Raw endpoint capabilities must not inherit a parent grant that denies a sibling mutation.
func enforceExplicitCommandPermission(flags *RootFlags, path []string) error {
	profile, err := loadBakedSafetyProfile()
	if err != nil {
		return err
	}
	if profile.enabled && (bakedSafetyDenyMatch(path) || !bakedSafetyAllowExactMatch(path)) {
		return profile.commandPathError(path)
	}
	if flags == nil {
		return nil
	}
	allow := parseEnabledCommands(flags.EnableCommands)
	exact := parseEnabledCommands(flags.EnableCommandsExact)
	deny := parseEnabledCommands(flags.DisableCommands)
	if commandPathMatches(deny, path) {
		return usagef("%s is disabled by command policy", strings.Join(path, " "))
	}
	if len(allow) == 0 && len(exact) == 0 && len(deny) == 0 {
		return nil
	}
	rule := strings.Join(path, ".")
	if allow[rule] || exact[rule] {
		return nil
	}
	if len(deny) == 0 && (allow["*"] || allow["all"] || exact["*"] || exact["all"]) {
		return nil
	}
	return usagef("%s requires explicit command-policy permission; add %s to --enable-commands or --enable-commands-exact", strings.Join(path, " "), rule)
}
