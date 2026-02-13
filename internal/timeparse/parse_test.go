package timeparse

import (
	"testing"
	"time"
)

//nolint:wsl_v5
func TestParseDate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid", value: "2026-02-13"},
		{name: "trimmed", value: " 2026-02-13 "},
		{name: "invalid separator", value: "2026/02/13", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDate(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}

				return
			}
			if err != nil {
				t.Fatalf("ParseDate: %v", err)
			}
			if got.Format("2006-01-02") != "2026-02-13" {
				t.Fatalf("unexpected date: %s", got.Format("2006-01-02"))
			}
		})
	}
}

//nolint:wsl_v5
func TestParseDateTimeOrDate(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("Offset", -8*3600)
	testCases := []struct {
		name        string
		value       string
		wantErr     bool
		wantHasTime bool
		wantHour    int
		wantMin     int
		wantOffset  int
	}{
		{name: "rfc3339", value: "2026-02-13T10:20:30Z", wantHasTime: true, wantHour: 10, wantMin: 20, wantOffset: 0},
		{name: "rfc3339 nano", value: "2026-02-13T10:20:30.123456789Z", wantHasTime: true, wantHour: 10, wantMin: 20, wantOffset: 0},
		{name: "iso tz no colon", value: "2026-02-13T10:20:30-0800", wantHasTime: true, wantHour: 10, wantMin: 20, wantOffset: -8 * 3600},
		{name: "date only", value: "2026-02-13", wantHasTime: false, wantHour: 0, wantMin: 0, wantOffset: -8 * 3600},
		{name: "local datetime seconds", value: "2026-02-13T10:20:30", wantHasTime: true, wantHour: 10, wantMin: 20, wantOffset: -8 * 3600},
		{name: "local datetime minutes", value: "2026-02-13 10:20", wantHasTime: true, wantHour: 10, wantMin: 20, wantOffset: -8 * 3600},
		{name: "invalid", value: "nope", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDateTimeOrDate(tc.value, loc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDateTimeOrDate: %v", err)
			}
			if got.HasTime != tc.wantHasTime {
				t.Fatalf("HasTime=%v want %v", got.HasTime, tc.wantHasTime)
			}
			if got.Time.Hour() != tc.wantHour || got.Time.Minute() != tc.wantMin {
				t.Fatalf("unexpected time: %v", got.Time)
			}
			_, offset := got.Time.Zone()
			if offset != tc.wantOffset {
				t.Fatalf("offset=%d want %d", offset, tc.wantOffset)
			}
		})
	}
}

//nolint:wsl_v5
func TestParseRangeExpr(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 13, 15, 45, 0, 0, time.UTC)
	loc := time.FixedZone("Offset", -5*3600)
	testCases := []struct {
		name      string
		value     string
		wantErr   bool
		wantDay   int
		wantMonth time.Month
		wantHour  int
		wantWeek  time.Weekday
	}{
		{name: "now", value: "now", wantDay: 13, wantMonth: time.February, wantHour: 15, wantWeek: time.Friday},
		{name: "today", value: "today", wantDay: 13, wantMonth: time.February, wantHour: 0, wantWeek: time.Friday},
		{name: "tomorrow", value: "tomorrow", wantDay: 14, wantMonth: time.February, wantHour: 0, wantWeek: time.Saturday},
		{name: "weekday", value: "monday", wantDay: 16, wantMonth: time.February, wantHour: 0, wantWeek: time.Monday},
		{name: "next weekday", value: "next friday", wantDay: 20, wantMonth: time.February, wantHour: 0, wantWeek: time.Friday},
		{name: "date", value: "2026-02-01", wantDay: 1, wantMonth: time.February, wantHour: 0, wantWeek: time.Sunday},
		{name: "datetime", value: "2026-02-01T10:30:00", wantDay: 1, wantMonth: time.February, wantHour: 10, wantWeek: time.Sunday},
		{name: "invalid", value: "yolo", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRangeExpr(tc.value, now, loc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRangeExpr: %v", err)
			}
			if got.Day() != tc.wantDay || got.Month() != tc.wantMonth || got.Hour() != tc.wantHour || got.Weekday() != tc.wantWeek {
				t.Fatalf("unexpected parsed range: %v", got)
			}
		})
	}
}

//nolint:wsl_v5
func TestParseSince(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 13, 15, 45, 0, 0, time.UTC)
	loc := time.FixedZone("Offset", -8*3600)
	testCases := []struct {
		name     string
		value    string
		wantErr  bool
		want     time.Time
		wantNano bool
	}{
		{name: "duration", value: "24h", want: now.Add(-24 * time.Hour).UTC()},
		{name: "date", value: "2026-02-01", want: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{name: "rfc3339", value: "2026-02-01T10:20:30Z", want: time.Date(2026, 2, 1, 10, 20, 30, 0, time.UTC)},
		{name: "rfc3339nano", value: "2026-02-01T10:20:30.123456789Z", want: time.Date(2026, 2, 1, 10, 20, 30, 123456789, time.UTC), wantNano: true},
		{name: "local datetime", value: "2026-02-01 10:20", want: time.Date(2026, 2, 1, 18, 20, 0, 0, time.UTC)},
		{name: "invalid", value: "nope", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSince(tc.value, now, loc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSince: %v", err)
			}
			if !got.Time.Equal(tc.want) {
				t.Fatalf("time=%v want %v", got.Time, tc.want)
			}
			if got.UseRFC3339Nano != tc.wantNano {
				t.Fatalf("UseRFC3339Nano=%v want %v", got.UseRFC3339Nano, tc.wantNano)
			}
		})
	}
}
