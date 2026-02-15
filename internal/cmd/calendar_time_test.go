package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarTimeCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users/me/calendarList/primary") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"summary":  "Primary Calendar",
				"timeZone": "America/Los_Angeles",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "time"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Timezone    string `json:"timezone"`
		CurrentTime string `json:"current_time"`
		Formatted   string `json:"formatted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	// Verify timezone
	if parsed.Timezone != "America/Los_Angeles" {
		t.Errorf("expected timezone America/Los_Angeles, got %q", parsed.Timezone)
	}

	// Verify current_time is valid RFC3339
	if _, err := time.Parse(time.RFC3339, parsed.CurrentTime); err != nil {
		t.Errorf("current_time is not valid RFC3339: %v", err)
	}

	// Verify formatted is not empty
	if parsed.Formatted == "" {
		t.Error("formatted time is empty")
	}
}

func TestCalendarTimeCmd_TableOutput(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users/me/calendarList/primary") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"summary":  "Primary Calendar",
				"timeZone": "America/New_York",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "calendar", "time"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Verify table output contains expected fields
	if !strings.Contains(out, "timezone") {
		t.Errorf("output missing timezone field: %q", out)
	}
	if !strings.Contains(out, "current_time") {
		t.Errorf("output missing current_time field: %q", out)
	}
	if !strings.Contains(out, "formatted") {
		t.Errorf("output missing formatted field: %q", out)
	}
	if !strings.Contains(out, "America/New_York") {
		t.Errorf("output missing timezone value: %q", out)
	}
}

func TestCalendarTimeCmd_WithTimezoneFlag(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	// No server needed since we're using --timezone flag
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("should not call calendar service when --timezone is provided")
		return nil, errors.New("unexpected calendar service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "time", "--timezone", "UTC"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Timezone    string `json:"timezone"`
		CurrentTime string `json:"current_time"`
		Formatted   string `json:"formatted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	// Verify timezone
	if parsed.Timezone != "UTC" {
		t.Errorf("expected timezone UTC, got %q", parsed.Timezone)
	}

	// Verify current_time is valid RFC3339 and ends with Z (UTC)
	parsedTime, err := time.Parse(time.RFC3339, parsed.CurrentTime)
	if err != nil {
		t.Errorf("current_time is not valid RFC3339: %v", err)
	}
	if parsedTime.Location().String() != "UTC" {
		t.Errorf("expected UTC timezone, got %q", parsedTime.Location().String())
	}
}

func TestCalendarTimeCmd_UsesEnvTimezone(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	origEnv := os.Getenv("GOG_TIMEZONE")
	defer os.Setenv("GOG_TIMEZONE", origEnv)

	envTZ := pickTimezoneExcluding(t)
	os.Setenv("GOG_TIMEZONE", envTZ)

	// No server needed since we're using env timezone
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("should not call calendar service when GOG_TIMEZONE is provided")
		return nil, errors.New("unexpected calendar service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "time"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Timezone    string `json:"timezone"`
		CurrentTime string `json:"current_time"`
		Formatted   string `json:"formatted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if parsed.Timezone != envTZ {
		t.Errorf("expected timezone %s, got %q", envTZ, parsed.Timezone)
	}
	if parsed.CurrentTime == "" || parsed.Formatted == "" {
		t.Errorf("expected current_time and formatted to be populated")
	}
}

func TestCalendarTimeCmd_WithTimezoneLocal(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	origEnv := os.Getenv("GOG_TIMEZONE")
	defer os.Setenv("GOG_TIMEZONE", origEnv)

	envTZ := pickNonLocalTimezone(t)
	os.Setenv("GOG_TIMEZONE", envTZ)

	// No server needed since --timezone local should force local
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("should not call calendar service when --timezone local is provided")
		return nil, errors.New("unexpected calendar service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "time", "--timezone", "local"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if parsed.Timezone != time.Local.String() {
		t.Errorf("expected timezone %q, got %q", time.Local.String(), parsed.Timezone)
	}
}

func TestCalendarTimeCmd_InvalidTimezone(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	// No server needed since we're testing error case
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatal("should not call calendar service when invalid timezone is provided")
		return nil, errors.New("unexpected calendar service call")
	}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--account", "a@b.com", "calendar", "time", "--timezone", "Invalid/Timezone"})
			if err == nil {
				t.Fatal("expected error for invalid timezone, got nil")
			}
		})
	})

	// Verify error message contains timezone information
	if !strings.Contains(stderr, "Invalid/Timezone") && !strings.Contains(stderr, "timezone") {
		t.Errorf("expected error message about invalid timezone, got: %q", stderr)
	}
}

func TestCalendarTimeCmd_CustomCalendar(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users/me/calendarList/custom-cal-id@example.com") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "custom-cal-id@example.com",
				"summary":  "Custom Calendar",
				"timeZone": "Europe/London",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "time", "--calendar", "custom-cal-id@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Timezone    string `json:"timezone"`
		CurrentTime string `json:"current_time"`
		Formatted   string `json:"formatted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	// Verify timezone from custom calendar
	if parsed.Timezone != "Europe/London" {
		t.Errorf("expected timezone Europe/London, got %q", parsed.Timezone)
	}
}
