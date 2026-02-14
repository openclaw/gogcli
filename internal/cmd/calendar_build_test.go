package cmd

import "testing"

func TestExtractTimezone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-01-08T11:00:00-05:00", "America/New_York"},
		{"2026-07-08T11:00:00-04:00", "America/New_York"},
		{"2026-01-08T11:00:00-06:00", "America/Chicago"},
		{"2026-07-08T11:00:00-05:00", "America/Chicago"},
		{"2026-01-08T11:00:00-07:00", "America/Denver"},
		{"2026-07-08T11:00:00-07:00", "America/Phoenix"},
		{"2026-01-08T11:00:00-08:00", "America/Los_Angeles"},
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
