package cmd

import (
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestMergeAttendees(t *testing.T) {
	tests := []struct {
		name     string
		existing []*calendar.EventAttendee
		addCSV   string
		wantLen  int
	}{
		{
			name:     "add to empty list",
			existing: nil,
			addCSV:   "a@test.com,b@test.com",
			wantLen:  2,
		},
		{
			name: "add to existing list",
			existing: []*calendar.EventAttendee{
				{Email: "existing@test.com", ResponseStatus: "accepted"},
			},
			addCSV:  "new@test.com",
			wantLen: 2,
		},
		{
			name: "skip duplicates case-insensitive",
			existing: []*calendar.EventAttendee{
				{Email: "Existing@Test.com", ResponseStatus: "accepted"},
			},
			addCSV:  "existing@test.com,new@test.com",
			wantLen: 2,
		},
		{
			name: "preserve existing metadata",
			existing: []*calendar.EventAttendee{
				{Email: "alice@test.com", ResponseStatus: "accepted", DisplayName: "Alice"},
			},
			addCSV:  "bob@test.com",
			wantLen: 2,
		},
		{
			name: "empty add string",
			existing: []*calendar.EventAttendee{
				{Email: "keep@test.com", ResponseStatus: "accepted"},
			},
			addCSV:  "",
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeAttendees(tt.existing, tt.addCSV)
			if len(got) != tt.wantLen {
				t.Errorf("mergeAttendees() returned %d attendees, want %d", len(got), tt.wantLen)
			}

			if tt.name == "preserve existing metadata" && len(got) > 0 {
				found := false
				for _, a := range got {
					if a.Email == "alice@test.com" {
						found = true
						if a.ResponseStatus != "accepted" {
							t.Errorf("existing attendee lost responseStatus")
						}
						if a.DisplayName != "Alice" {
							t.Errorf("existing attendee lost displayName")
						}
					}
				}
				if !found {
					t.Errorf("existing attendee alice@test.com not found in result")
				}
			}
		})
	}
}

func TestMergeAttendeesNewHaveNeedsAction(t *testing.T) {
	existing := []*calendar.EventAttendee{
		{Email: "existing@test.com", ResponseStatus: "accepted"},
	}
	got := mergeAttendees(existing, "new@test.com")

	for _, a := range got {
		if a.Email == "new@test.com" {
			if a.ResponseStatus != "needsAction" {
				t.Errorf("new attendee should have responseStatus=needsAction, got %q", a.ResponseStatus)
			}
			return
		}
	}
	t.Error("new attendee not found in result")
}

func TestMergeAttendeesWithChange(t *testing.T) {
	existing := []*calendar.EventAttendee{
		{Email: "existing@test.com", ResponseStatus: "accepted"},
	}

	if _, changed := mergeAttendeesWithChange(existing, "existing@test.com"); changed {
		t.Fatalf("expected no change for duplicate attendee")
	}

	out, changed := mergeAttendeesWithChange(existing, "new@test.com")
	if !changed {
		t.Fatalf("expected changed=true when adding new attendee")
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 attendees after merge, got %d", len(out))
	}
}
