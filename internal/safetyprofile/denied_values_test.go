package safetyprofile

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse_DeniedValues(t *testing.T) {
	profile, err := Parse(`
name: denied
denied-values:
  gmail.thread.modify:
    add: [TRASH, " Spam "]
    add-label: [TRASH, SPAM, trash]
  Calendar Create:
    send-updates: [all, externalOnly]
gmail:
  thread:
    modify: true
calendar:
  create: true
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []DeniedValue{
		{Path: "calendar.create", Flag: "send-updates", Values: []string{"all", "externalonly"}},
		{Path: "gmail.thread.modify", Flag: "add", Values: []string{"spam", "trash"}},
		{Path: "gmail.thread.modify", Flag: "add-label", Values: []string{"spam", "trash"}},
	}
	if !reflect.DeepEqual(profile.DeniedValues, want) {
		t.Fatalf("denied values = %#v, want %#v", profile.DeniedValues, want)
	}
}

func TestParse_DeniedValuesRejectsMalformedRules(t *testing.T) {
	cases := map[string]string{
		"non-list":       "gmail.thread.modify:\n    add: TRASH",
		"empty list":     "gmail.thread.modify:\n    add: []",
		"nested map":     "gmail.thread.modify:\n    add:\n      nested: [TRASH]",
		"empty string":   "gmail.thread.modify:\n    add: [\"\"]",
		"blank string":   "gmail.thread.modify:\n    add: [\"  \"]",
		"non-string":     "gmail.thread.modify:\n    add: [true]",
		"nested item":    "gmail.thread.modify:\n    add: [[TRASH]]",
		"wildcard path":  "'*':\n    add: [TRASH]",
		"all path":       "all:\n    add: [TRASH]",
		"empty flag":     "gmail.thread.modify:\n    \"\": [TRASH]",
		"flags not map":  "gmail.thread.modify: [TRASH]",
		"top not map":    "[gmail.thread.modify]",
		"empty path key": "\"\":\n    add: [TRASH]",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("name: denied\ngmail:\n  thread:\n    modify: true\ndenied-values:\n  " + strings.ReplaceAll(body, "\n", "\n  "))
			if err == nil {
				t.Fatal("malformed denied-values must be rejected")
			}
			if !strings.HasPrefix(err.Error(), "denied-values: ") {
				t.Fatalf("error must come from the denied-values parser, got: %v", err)
			}
		})
	}
}

func TestParse_DeniedValuesAnyValue(t *testing.T) {
	profile, err := Parse(`
name: denied
denied-values:
  calendar.create:
    focus-chat-status: ["*"]
calendar:
  create: true
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []DeniedValue{{Path: "calendar.create", Flag: "focus-chat-status", Values: []string{"*"}}}
	if !reflect.DeepEqual(profile.DeniedValues, want) {
		t.Fatalf("denied values = %#v, want %#v", profile.DeniedValues, want)
	}
}

// "*" means any value, so listing other values beside it is ambiguous.
func TestParse_DeniedValuesAnyValueMustStandAlone(t *testing.T) {
	for _, list := range []string{`["*", all]`, `[all, "*"]`, `[" * ", x]`} {
		_, err := Parse("name: denied\ncalendar:\n  create: true\ndenied-values:\n  calendar.create:\n    send-updates: " + list + "\n")
		if err == nil || !strings.HasPrefix(err.Error(), "denied-values: ") {
			t.Fatalf("%s must be rejected by the denied-values parser, got %v", list, err)
		}
	}
}

// denied-values must not be flattened into command rules the way other top-level
// keys are.
func TestParse_DeniedValuesAreNotCommandRules(t *testing.T) {
	profile, err := Parse(`
name: denied
denied-values:
  gmail.thread.modify:
    add: [TRASH]
gmail:
  thread:
    modify: true
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, rule := range append(profile.AllowRules, profile.DenyRules...) {
		if rule != "gmail.thread.modify" {
			t.Fatalf("denied value leaked into command rules: %q", rule)
		}
	}
}

func TestParse_WithoutDeniedValues(t *testing.T) {
	profile, err := Parse("name: plain\ngmail:\n  get: true\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(profile.DeniedValues) != 0 {
		t.Fatalf("denied values = %#v", profile.DeniedValues)
	}
}
