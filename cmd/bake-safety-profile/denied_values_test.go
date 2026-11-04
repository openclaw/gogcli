package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/openclaw/gogcli/internal/safetyprofile"
)

func TestGenerateDeniedValuesEmitsHashedLookup(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:       "test",
		AllowRules: []string{"gmail.thread.modify"},
		DeniedValues: []safetyprofile.DeniedValue{
			{Path: "calendar.create", Flag: "send-updates", Values: []string{"all", "externalonly"}},
			{Path: "gmail.thread.modify", Flag: "add", Values: []string{"spam", "trash"}},
		},
	}

	out := generate(profile)

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}

	// Path and flag are hashed together like a command rule; the values are the
	// literals the runtime compares against.
	for _, rule := range profile.DeniedValues {
		quoted := ""
		for i, v := range rule.Values {
			if i > 0 {
				quoted += ", "
			}
			quoted += fmt.Sprintf("%q", v)
		}
		want := fmt.Sprintf("\tcase 0x%016x:\n\t\treturn []string{%s}\n", safetyprofile.HashRule(rule.Path+"."+rule.Flag), quoted)
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("generated output missing case for %s %s:\n%s\n\nfull output:\n%s", rule.Path, rule.Flag, want, out)
		}
		for _, verbatim := range []string{rule.Path, rule.Flag} {
			if bytes.Contains(out, []byte(`"`+verbatim+`"`)) {
				t.Fatalf("denied value rule component %q appears verbatim; it should only be hashed\n\nfull output:\n%s", verbatim, out)
			}
		}
	}
	if !bytes.Contains(out, []byte("func bakedSafetyDeniedValueRuleCount() int { return 2 }\n")) {
		t.Fatalf("generated output missing the rule count:\n%s", out)
	}
}

// A profile without denied values still needs both functions, so safety_profile
// builds compile whether or not the profile uses the feature.
func TestGenerateWithoutDeniedValuesStillDefinesLookup(t *testing.T) {
	out := generate(&safetyprofile.Profile{Name: "test", AllowRules: []string{"gmail.get"}})

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}
	for _, want := range []string{
		"func bakedSafetyDeniedValues(path []string, flag string) []string {\n\treturn nil\n}\n",
		"func bakedSafetyDeniedValueRuleCount() int { return 0 }\n",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("generated output missing %q:\n%s", want, out)
		}
	}
}
