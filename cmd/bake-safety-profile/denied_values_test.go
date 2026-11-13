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
			{Path: "drive.upload", Flag: "name", Values: []string{"private,report.txt"}},
			{Path: "calendar.create", Flag: "focus-chat-status", Values: []string{"*"}},
		},
	}

	out := generate(profile)

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}

	// Path and flag are hashed together like a command rule, and each refused value is
	// hashed with them, so no part of a rule is a string an in-place edit could change.
	for _, rule := range profile.DeniedValues {
		anyValue := len(rule.Values) == 1 && rule.Values[0] == "*"
		want := fmt.Sprintf("\tcase 0x%016x:\n\t\treturn true, %t\n", safetyprofile.HashRule(rule.Path+"."+rule.Flag), anyValue)
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("generated output missing rule case for %s %s:\n%s\n\nfull output:\n%s", rule.Path, rule.Flag, want, out)
		}
		for _, verbatim := range append([]string{rule.Path, rule.Flag}, rule.Values...) {
			if bytes.Contains(out, []byte(`"`+verbatim+`"`)) {
				t.Fatalf("denied value rule component %q appears verbatim; it should only be hashed\n\nfull output:\n%s", verbatim, out)
			}
		}
		if anyValue {
			continue
		}
		for _, value := range rule.Values {
			h := fmt.Sprintf("0x%016x", safetyprofile.HashRule(safetyprofile.DeniedValueKey(rule.Path, rule.Flag, value)))
			if !bytes.Contains(out, []byte(h)) {
				t.Fatalf("generated output missing value hash %s for %s %s %q:\n%s", h, rule.Path, rule.Flag, value, out)
			}
		}
	}
	if !bytes.Contains(out, []byte("func bakedSafetyDeniedValueRuleCount() int { return 4 }\n")) {
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
		"func bakedSafetyDeniedValueRule(path []string, flag string) (bool, bool) {\n\treturn false, false\n}\n",
		"func bakedSafetyDeniedValueMatch(path []string, flag, value string) bool {\n\treturn false\n}\n",
		"func bakedSafetyDeniedValueRuleCount() int { return 0 }\n",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("generated output missing %q:\n%s", want, out)
		}
	}
}
