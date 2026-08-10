package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/openclaw/gogcli/internal/safetyprofile"
)

func TestGenerateLockedFlagsEmitsHashedLookup(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:       "test",
		AllowRules: []string{"gmail.get"},
		LockedFlags: []safetyprofile.LockedFlag{
			{Name: "sanitize-content", Value: "true"},
			{Name: "inline-max-bytes", Value: "8388608"},
		},
	}

	out := generate(profile)

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}

	// The flag name must be hashed like a command rule, and the value emitted as the
	// literal the flag parser consumes.
	for _, flag := range profile.LockedFlags {
		want := fmt.Sprintf("\tcase 0x%016x:\n\t\treturn %q, true\n", safetyprofile.HashRule(flag.Name), flag.Value)
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("generated output missing case for %q:\n%s\n\nfull output:\n%s", flag.Name, want, out)
		}
		if bytes.Contains(out, []byte(`"`+flag.Name+`"`)) {
			t.Fatalf("locked flag name %q appears verbatim; it should only be hashed\n\nfull output:\n%s", flag.Name, out)
		}
	}
}

// A profile with no locked flags still needs the lookup, so safety_profile builds
// compile whether or not the profile uses the feature.
func TestGenerateWithoutLockedFlagsStillDefinesLookup(t *testing.T) {
	out := generate(&safetyprofile.Profile{Name: "test", AllowRules: []string{"gmail.get"}})

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}
	want := "func bakedSafetyLockedFlag(name string) (string, bool) {\n\treturn \"\", false\n}\n"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("generated output missing the empty lookup:\n%s", out)
	}
}
