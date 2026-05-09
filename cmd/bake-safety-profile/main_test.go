package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/safetyprofile"
)

func TestGenerateProducesParseableGoWithExpectedHashes(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:       "test",
		AllowAll:   false,
		AllowRules: []string{"version", "gmail.search", "gmail.drafts.create"},
		DenyRules:  []string{"gmail.send", "gmail.drafts.send"},
	}

	out := generate(profile)

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code does not parse as Go:\n%s\n\nerror: %v", out, err)
	}

	want := []string{
		`//go:build safety_profile`,
		`package cmd`,
		`const bakedSafetyProfileNameConst = "test"`,
		`func bakedSafetyEnabled() bool       { return true }`,
		`func bakedSafetyHasAllowRules() bool { return true }`,
	}
	for _, line := range want {
		if !bytes.Contains(out, []byte(line)) {
			t.Fatalf("generated output missing %q\n\nfull output:\n%s", line, out)
		}
	}

	for _, rule := range profile.AllowRules {
		hex := fmt.Sprintf("0x%016x", safetyprofile.HashRule(rule))
		if !bytes.Contains(out, []byte(hex)) {
			t.Fatalf("expected allow hash %s for rule %q in output", hex, rule)
		}
	}
	for _, rule := range profile.DenyRules {
		hex := fmt.Sprintf("0x%016x", safetyprofile.HashRule(rule))
		if !bytes.Contains(out, []byte(hex)) {
			t.Fatalf("expected deny hash %s for rule %q in output", hex, rule)
		}
	}

	for _, rule := range profile.AllowRules {
		if bytes.Contains(out, []byte(fmt.Sprintf("%q", rule))) {
			t.Fatalf("rule string %q must not appear in generated output", rule)
		}
	}
	for _, rule := range profile.DenyRules {
		if bytes.Contains(out, []byte(fmt.Sprintf("%q", rule))) {
			t.Fatalf("rule string %q must not appear in generated output", rule)
		}
	}
}

func TestGenerateSanitizesProfileNameInComment(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:      "bad\nname\rwith-controls",
		AllowAll:  true,
		DenyRules: []string{},
	}
	out := generate(profile)

	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code with control chars in name does not parse:\n%s\n\nerror: %v", out, err)
	}
	if bytes.Contains(out, []byte("\nname\r")) || bytes.Contains(out, []byte("\nname\n")) {
		t.Fatalf("comment header leaked control chars from profile name:\n%s", out)
	}
}

func TestGenerateAllowAllEmitsConstantTrue(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:      "full",
		AllowAll:  true,
		DenyRules: []string{},
	}
	out := string(generate(profile))

	allowFn := extractFunc(t, out, "bakedSafetyAllowMatch")
	if !strings.Contains(allowFn, "return true") || strings.Contains(allowFn, "switch ") {
		t.Fatalf("AllowAll allow matcher should be `return true` only, got:\n%s", allowFn)
	}

	denyFn := extractFunc(t, out, "bakedSafetyDenyMatch")
	if !strings.Contains(denyFn, "return false") {
		t.Fatalf("empty deny matcher should `return false`, got:\n%s", denyFn)
	}
}

func TestGenerateEmptyAllowEmitsConstantFalse(t *testing.T) {
	profile := &safetyprofile.Profile{
		Name:      "deny-only",
		AllowAll:  false,
		DenyRules: []string{"gmail.send"},
	}
	out := string(generate(profile))

	if !strings.Contains(out, "func bakedSafetyHasAllowRules() bool { return false }") {
		t.Fatalf("expected hasAllowRules=false for deny-only profile, got:\n%s", out)
	}
	allowFn := extractFunc(t, out, "bakedSafetyAllowMatch")
	if !strings.Contains(allowFn, "return false") || strings.Contains(allowFn, "switch ") {
		t.Fatalf("empty allow matcher should be `return false` only, got:\n%s", allowFn)
	}
}

func extractFunc(t *testing.T, src, name string) string {
	t.Helper()
	start := strings.Index(src, "func "+name+"(")
	if start < 0 {
		t.Fatalf("function %s not found in:\n%s", name, src)
	}
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatalf("function %s has unbalanced braces", name)
	return ""
}
