package safetyprofile

import "testing"

func TestParse_LockedFlagsValues(t *testing.T) {
	profile, err := Parse(`
name: locked
locked-flags:
  sanitize-content: true
  inline-max-bytes: 8388608
  format: metadata
gmail:
  get: true
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []LockedFlag{
		{Name: "format", Value: "metadata"},
		{Name: "inline-max-bytes", Value: "8388608"},
		{Name: "sanitize-content", Value: "true"},
	}
	if len(profile.LockedFlags) != len(want) {
		t.Fatalf("locked flags = %#v", profile.LockedFlags)
	}
	for i, w := range want {
		if profile.LockedFlags[i] != w {
			t.Fatalf("locked flag %d = %#v, want %#v", i, profile.LockedFlags[i], w)
		}
	}
}

func TestParse_LockedFlagsRejectsNonScalar(t *testing.T) {
	_, err := Parse(`
name: locked
locked-flags:
  sanitize-content:
    nested: true
gmail:
  get: true
`)
	if err == nil {
		t.Fatal("nested locked-flags value must be rejected")
	}
}

// locked-flags must not be flattened into command rules the way other top-level
// keys are.
func TestParse_LockedFlagsAreNotCommandRules(t *testing.T) {
	profile, err := Parse(`
name: locked
locked-flags:
  sanitize-content: true
gmail:
  get: true
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, rule := range append(profile.AllowRules, profile.DenyRules...) {
		if rule == "locked-flags.sanitize-content" || rule == "sanitize-content" {
			t.Fatalf("locked flag leaked into command rules: %v", rule)
		}
	}
}
