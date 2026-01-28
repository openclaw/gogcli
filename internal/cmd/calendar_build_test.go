package cmd

import (
	"testing"
	"time"
)

func TestExtractTimezone(t *testing.T) {
	// Test UTC cases - always returns "UTC"
	utcTests := []string{
		"2026-01-08T16:00:00Z",
		"2026-01-08T11:00:00+00:00",
		"2026-07-15T09:30:00Z",
	}
	for _, input := range utcTests {
		t.Run("UTC_"+input, func(t *testing.T) {
			got := extractTimezone(input)
			if got != "UTC" {
				t.Errorf("extractTimezone(%q) = %q, want %q", input, got, "UTC")
			}
		})
	}

	// Test local timezone - when offset matches local and IANA name is available
	t.Run("local_timezone_match", func(t *testing.T) {
		localName := time.Local.String()
		// Skip if local timezone doesn't have a real IANA name
		if localName == "Local" || localName == "" {
			t.Skip("local timezone doesn't have IANA name available")
		}

		// Create a time in the local timezone and format it as RFC3339
		now := time.Now()
		localTime := now.Format(time.RFC3339)
		got := extractTimezone(localTime)
		if got != localName {
			t.Errorf("extractTimezone(%q) = %q, want %q (local timezone)", localTime, got, localName)
		}
	})

	// Test with explicit timezone to ensure non-UTC handling works
	t.Run("explicit_timezone_europe_berlin", func(t *testing.T) {
		loc, err := time.LoadLocation("Europe/Berlin")
		if err != nil {
			t.Skip("Europe/Berlin timezone not available")
		}

		// Temporarily set local timezone for this test
		origLocal := time.Local
		time.Local = loc
		defer func() { time.Local = origLocal }()

		// Winter time: +01:00
		input := "2026-01-15T14:00:00+01:00"
		got := extractTimezone(input)
		if got != "Europe/Berlin" {
			t.Errorf("extractTimezone(%q) = %q, want %q", input, got, "Europe/Berlin")
		}
	})

	// Test invalid input
	t.Run("invalid", func(t *testing.T) {
		if got := extractTimezone("invalid"); got != "" {
			t.Errorf("extractTimezone(invalid) = %q, want empty", got)
		}
	})

	// Non-matching offsets return empty (caller uses calendar/configured timezone)
	t.Run("non_matching_offset", func(t *testing.T) {
		// Use +05:30 (India) which is unlikely to be the local timezone
		input := "2026-01-08T11:00:00+05:30"
		testTime, _ := time.Parse(time.RFC3339, input)
		_, localOffset := testTime.In(time.Local).Zone()
		_, inputOffset := testTime.Zone()
		if localOffset == inputOffset {
			t.Skip("local timezone matches test offset, skipping")
		}
		got := extractTimezone(input)
		if got != "" {
			t.Errorf("extractTimezone(%q) = %q, want empty for non-matching offset", input, got)
		}
	})
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
