package cmd

import "testing"

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
	if _, err := resolveEventType("focus-time", false, true, false); err == nil {
		t.Fatalf("expected conflict with focus + ooo flags")
	}
	if _, err := resolveEventType("", true, true, false); err == nil {
		t.Fatalf("expected conflict with multiple flag sets")
	}
}
