package cmd

import (
	"slices"
	"testing"
)

func TestNormalizeEventType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"focus-time", eventTypeFocusTime},
		{"FOCUS", eventTypeFocusTime},
		{"out-of-office", eventTypeOutOfOffice},
		{"ooo", eventTypeOutOfOffice},
		{"working-location", eventTypeWorkingLocation},
		{"wl", eventTypeWorkingLocation},
		{"default", eventTypeDefault},
		{"", ""},
	}
	for _, tc := range cases {
		got, err := normalizeEventType(tc.in)
		if err != nil {
			t.Fatalf("normalize %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalize %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveEventTypeConflicts(t *testing.T) {
	_, err := resolveEventType("focus-time", false, true, false)
	requireUsageError(t, err)
	_, err = resolveEventType("", true, true, false)
	requireUsageError(t, err)
	_, err = resolveEventType("nope", false, false, false)
	requireUsageError(t, err)
}

func TestNormalizeFilterEventType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"default", eventTypeDefault},
		{"birthday", eventTypeBirthday},
		{"BIRTHDAY", eventTypeBirthday},
		{"focus-time", eventTypeFocusTime},
		{"focusTime", eventTypeFocusTime},
		{"from-gmail", eventTypeFromGmail},
		{"fromGmail", eventTypeFromGmail},
		{"ooo", eventTypeOutOfOffice},
		{"out-of-office", eventTypeOutOfOffice},
		{"wl", eventTypeWorkingLocation},
		{"  workingLocation  ", eventTypeWorkingLocation},
	}
	for _, tc := range cases {
		got, err := normalizeFilterEventType(tc.in)
		if err != nil {
			t.Fatalf("normalizeFilterEventType(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeFilterEventType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeFilterEventTypeInvalid(t *testing.T) {
	// "gmail" is intentionally not an alias (too ambiguous); only "from-gmail"
	// and its spellings map to fromGmail.
	for _, in := range []string{"nope", "meeting", "gmail", ""} {
		if _, err := normalizeFilterEventType(in); err == nil {
			t.Fatalf("normalizeFilterEventType(%q): expected usage error", in)
		}
	}
}

func TestCreatableEventTypesAreAliased(t *testing.T) {
	// Invariant: every creatable type must be reachable through the shared
	// eventTypeAliases map, so normalizeEventType can never reject a type that
	// creatableEventTypes claims is creatable. (The reverse need not hold:
	// the alias map is a superset that also includes the read-only types.)
	canonical := make(map[string]bool, len(eventTypeAliases))
	for _, c := range eventTypeAliases {
		canonical[c] = true
	}
	for ct := range creatableEventTypes {
		if !canonical[ct] {
			t.Fatalf("creatable event type %q has no entry in eventTypeAliases", ct)
		}
	}
}

func TestNormalizeEventTypeRejectsFilterOnlyTypes(t *testing.T) {
	// birthday and fromGmail are valid filter values but are not user-creatable,
	// so the create/update normalizer must reject them.
	for _, in := range []string{"birthday", "fromGmail", "from-gmail"} {
		if _, err := normalizeEventType(in); err == nil {
			t.Fatalf("normalizeEventType(%q): expected usage error (not creatable)", in)
		}
	}
}

func TestResolveFilterEventTypes(t *testing.T) {
	// Repeated and comma-separated values, with aliases, are flattened,
	// canonicalized, and deduplicated in first-seen order.
	got, err := resolveFilterEventTypes([]string{"birthday, workingLocation", "wl", "focus-time"})
	if err != nil {
		t.Fatalf("resolveFilterEventTypes: %v", err)
	}
	want := []string{eventTypeBirthday, eventTypeWorkingLocation, eventTypeFocusTime}
	if !slices.Equal(got, want) {
		t.Fatalf("resolveFilterEventTypes = %v, want %v", got, want)
	}

	// Flag absent (nil or empty slice) leaves the request unfiltered.
	for _, in := range [][]string{nil, {}} {
		got, err := resolveFilterEventTypes(in)
		if err != nil {
			t.Fatalf("resolveFilterEventTypes(%v): %v", in, err)
		}
		if got != nil {
			t.Fatalf("resolveFilterEventTypes(%v) = %v, want nil", in, got)
		}
	}

	// A flag that is present but resolves to no values is a usage error.
	if _, err := resolveFilterEventTypes([]string{"", "  "}); err == nil {
		t.Fatal("resolveFilterEventTypes(blanks): expected usage error")
	}

	// An invalid value anywhere is surfaced as an error.
	if _, err := resolveFilterEventTypes([]string{"default", "bogus"}); err == nil {
		t.Fatal("resolveFilterEventTypes: expected error for invalid type")
	}
}
