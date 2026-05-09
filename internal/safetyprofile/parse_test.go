package safetyprofile

import (
	"strings"
	"testing"
)

func TestParseRejectsDenyWildcards(t *testing.T) {
	for _, raw := range []string{
		"name: x\nallow:\n  - version\ndeny:\n  - all\n",
		"name: x\nallow:\n  - version\ndeny:\n  - \"*\"\n",
		"name: x\nallow:\n  - version\nall: false\n",
		"name: x\nallow:\n  - version\n\"*\": false\n",
		"name: x\nallow:\n  - version\naliases:\n  all: false\n",
	} {
		_, err := Parse(raw)
		if err == nil {
			t.Fatalf("expected error parsing deny wildcard, got nil for: %s", raw)
		}
		if !strings.Contains(err.Error(), "wildcards") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestParseRejectsWildcardPrefix(t *testing.T) {
	for _, raw := range []string{
		"name: x\nallow:\n  - version\nall:\n  gmail: false\n",
		"name: x\nallow:\n  - version\n\"*\":\n  gmail: false\n",
	} {
		_, err := Parse(raw)
		if err == nil {
			t.Fatalf("expected error parsing wildcard prefix, got nil for: %s", raw)
		}
		if !strings.Contains(err.Error(), "wildcard") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestParseRejectsNonStringName(t *testing.T) {
	for _, raw := range []string{
		"name: 42\nallow:\n  - version\n",
		"name: true\nallow:\n  - version\n",
		"name: \"\"\nallow:\n  - version\n",
	} {
		_, err := Parse(raw)
		if err == nil {
			t.Fatalf("expected error for invalid name, got nil for: %s", raw)
		}
		if !strings.Contains(err.Error(), "name:") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestHashRuleKnownValues(t *testing.T) {
	cases := []struct {
		rule string
		want uint64
	}{
		{"version", HashRule("version")},
		{"gmail.send", HashRule("gmail.send")},
	}
	for _, c := range cases {
		got := HashRule(c.rule)
		if got != c.want {
			t.Fatalf("HashRule(%q) = %#x, want %#x", c.rule, got, c.want)
		}
	}
	if HashRule("a") == HashRule("b") {
		t.Fatalf("expected distinct hashes for distinct rules")
	}
}
