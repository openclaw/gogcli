package cmd

import "testing"

func TestExtractTimezone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-01-08T11:00:00-05:00", "Etc/GMT+5"},
		{"2026-07-08T11:00:00-04:00", "Etc/GMT+4"},
		{"2026-01-08T11:00:00-06:00", "Etc/GMT+6"},
		{"2026-07-08T11:00:00-05:00", "Etc/GMT+5"},
		{"2026-01-08T11:00:00-07:00", "Etc/GMT+7"},
		{"2026-07-08T11:00:00-07:00", "Etc/GMT+7"},
		{"2026-01-08T11:00:00-08:00", "Etc/GMT+8"},
		{"2026-01-08T16:00:00Z", "UTC"},
		{"2026-01-08T11:00:00+00:00", "UTC"},
		{"invalid", ""},
		{"2026-01-08T11:00:00-04:00", "Etc/GMT+4"},
		{"2026-01-08T11:00:00+02:00", "Etc/GMT-2"},
		{"2026-01-08T11:00:00+05:30", ""}, // India - not mapped
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := extractTimezone(tc.input)
			if got != tc.expected {
				t.Errorf("extractTimezone(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestBuildEventDateTimeWithTimezone(t *testing.T) {
	edt, err := buildEventDateTimeWithTimezone(
		"2026-08-13T13:40:00+02:00",
		false,
		"Europe/Rome",
		"--start-timezone",
	)
	if err != nil {
		t.Fatalf("buildEventDateTimeWithTimezone: %v", err)
	}
	if edt.DateTime != "2026-08-13T13:40:00+02:00" {
		t.Fatalf("unexpected datetime: %#v", edt)
	}
	if edt.TimeZone != "Europe/Rome" {
		t.Fatalf("expected Europe/Rome timezone, got %#v", edt)
	}
}

func TestBuildEventDateTimeWithTimezoneRejectsInvalidInput(t *testing.T) {
	if _, err := buildEventDateTimeWithTimezone("2026-08-13", true, "Europe/Rome", "--start-timezone"); err == nil {
		t.Fatalf("expected all-day timezone error")
	}
	if _, err := buildEventDateTimeWithTimezone("2026-08-13T13:40:00+02:00", false, "Nope/Zone", "--start-timezone"); err == nil {
		t.Fatalf("expected invalid timezone error")
	} else if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestBuildAttachments(t *testing.T) {
	if got := buildAttachments(nil); got != nil {
		t.Fatalf("expected nil for empty input")
	}

	out := buildAttachments([]string{" https://example.com/a ", "", "https://example.com/b"})
	if len(out) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(out))
	}
	if out[0].FileUrl != "https://example.com/a" || out[1].FileUrl != "https://example.com/b" {
		t.Fatalf("unexpected urls: %#v", out)
	}
}

func TestBuildExtendedProperties(t *testing.T) {
	if got := buildExtendedProperties(nil, nil); got != nil {
		t.Fatalf("expected nil for empty properties")
	}

	props := buildExtendedProperties(
		[]string{" a = 1 ", "skip"},
		[]string{"b=2", " c = 3 "},
	)
	if props == nil || len(props.Private) != 1 || len(props.Shared) != 2 {
		t.Fatalf("unexpected props: %#v", props)
	}
	if props.Private["a"] != "1" {
		t.Fatalf("unexpected private props: %#v", props.Private)
	}
	if props.Shared["b"] != "2" || props.Shared["c"] != "3" {
		t.Fatalf("unexpected shared props: %#v", props.Shared)
	}
}

func TestResolveUnifiedTimezone(t *testing.T) {
	// Unset --timezone: pass the granular values through unchanged.
	startTZ, startFlag, endTZ, endFlag, err := resolveUnifiedTimezone("", "Europe/Rome", "America/New_York")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if startTZ != "Europe/Rome" || endTZ != "America/New_York" {
		t.Fatalf("passthrough changed values: start=%q end=%q", startTZ, endTZ)
	}
	if startFlag != "--start-timezone" || endFlag != "--end-timezone" {
		t.Fatalf("unexpected flag names: %q %q", startFlag, endFlag)
	}

	// --timezone alone: apply it to both endpoints with --timezone error attribution.
	startTZ, startFlag, endTZ, endFlag, err = resolveUnifiedTimezone("America/Los_Angeles", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if startTZ != "America/Los_Angeles" || endTZ != "America/Los_Angeles" {
		t.Fatalf("expected both zones set, got start=%q end=%q", startTZ, endTZ)
	}
	if startFlag != "--timezone" || endFlag != "--timezone" {
		t.Fatalf("expected --timezone flag names, got %q %q", startFlag, endFlag)
	}

	// --timezone combined with --start-timezone is a usage error.
	if _, _, _, _, err := resolveUnifiedTimezone("America/Los_Angeles", "Europe/Rome", ""); err == nil {
		t.Fatalf("expected error combining --timezone with --start-timezone")
	} else if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}

	// --timezone combined with --end-timezone is a usage error.
	if _, _, _, _, err := resolveUnifiedTimezone("America/Los_Angeles", "", "America/New_York"); err == nil {
		t.Fatalf("expected error combining --timezone with --end-timezone")
	}
}
