package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
)

func executeCalendarColorsTest(t *testing.T, svc *calendar.Service, args ...string) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Calendar: func(context.Context, string) (*calendar.Service, error) {
			return svc, nil
		},
	}})
}

func TestCalendarColorsCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/colors") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"event": map[string]any{
					"1": map[string]string{
						"background": "#a4bdfc",
						"foreground": "#1d1d1d",
					},
					"2": map[string]string{
						"background": "#7ae7bf",
						"foreground": "#1d1d1d",
					},
				},
				"calendar": map[string]any{
					"1": map[string]string{
						"background": "#ac725e",
						"foreground": "#1d1d1d",
					},
					"2": map[string]string{
						"background": "#d06b64",
						"foreground": "#1d1d1d",
					},
				},
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

	result := executeCalendarColorsTest(t, svc, "--json", "--account", "a@b.com", "calendar", "colors")
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Event map[string]struct {
			Background string `json:"background"`
			Foreground string `json:"foreground"`
		} `json:"event"`
		Calendar map[string]struct {
			Background string `json:"background"`
			Foreground string `json:"foreground"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}

	// Verify event colors
	if len(parsed.Event) != 2 {
		t.Fatalf("expected 2 event colors, got %d", len(parsed.Event))
	}
	if parsed.Event["1"].Background != "#a4bdfc" {
		t.Errorf("unexpected event color 1 background: %q", parsed.Event["1"].Background)
	}
	if parsed.Event["1"].Foreground != "#1d1d1d" {
		t.Errorf("unexpected event color 1 foreground: %q", parsed.Event["1"].Foreground)
	}

	// Verify calendar colors
	if len(parsed.Calendar) != 2 {
		t.Fatalf("expected 2 calendar colors, got %d", len(parsed.Calendar))
	}
	if parsed.Calendar["1"].Background != "#ac725e" {
		t.Errorf("unexpected calendar color 1 background: %q", parsed.Calendar["1"].Background)
	}
	if parsed.Calendar["1"].Foreground != "#1d1d1d" {
		t.Errorf("unexpected calendar color 1 foreground: %q", parsed.Calendar["1"].Foreground)
	}
}

func TestCalendarColorsCmd_TableOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/colors") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"event": map[string]any{
					"1": map[string]string{
						"background": "#a4bdfc",
						"foreground": "#1d1d1d",
					},
					"2": map[string]string{
						"background": "#7ae7bf",
						"foreground": "#1d1d1d",
					},
				},
				"calendar": map[string]any{
					"1": map[string]string{
						"background": "#ac725e",
						"foreground": "#1d1d1d",
					},
				},
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

	result := executeCalendarColorsTest(t, svc, "--account", "a@b.com", "calendar", "colors")
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

	// Verify table headers and content
	if !strings.Contains(out, "EVENT COLORS:") {
		t.Errorf("output missing event colors header: %q", out)
	}
	if !strings.Contains(out, "CALENDAR COLORS:") {
		t.Errorf("output missing calendar colors header: %q", out)
	}
	if !strings.Contains(out, "ID") {
		t.Errorf("output missing ID column header: %q", out)
	}
	if !strings.Contains(out, "BACKGROUND") {
		t.Errorf("output missing BACKGROUND column header: %q", out)
	}
	if !strings.Contains(out, "FOREGROUND") {
		t.Errorf("output missing FOREGROUND column header: %q", out)
	}

	// Verify color values appear in output
	if !strings.Contains(out, "#a4bdfc") {
		t.Errorf("output missing event color background: %q", out)
	}
	if !strings.Contains(out, "#ac725e") {
		t.Errorf("output missing calendar color background: %q", out)
	}
	if !strings.Contains(out, "#1d1d1d") {
		t.Errorf("output missing foreground color: %q", out)
	}
}

func TestCalendarColorsCmd_EmptyColors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/colors") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"event":    map[string]any{},
				"calendar": map[string]any{},
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

	// Test JSON output with empty colors
	jsonResult := executeCalendarColorsTest(t, svc, "--json", "--account", "a@b.com", "calendar", "colors")
	if jsonResult.err != nil {
		t.Fatalf("Execute JSON: %v", jsonResult.err)
	}

	var parsed struct {
		Event    map[string]any `json:"event"`
		Calendar map[string]any `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(jsonResult.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonResult.stdout)
	}
	if len(parsed.Event) != 0 {
		t.Errorf("expected empty event colors, got %d", len(parsed.Event))
	}
	if len(parsed.Calendar) != 0 {
		t.Errorf("expected empty calendar colors, got %d", len(parsed.Calendar))
	}

	// Test table output with empty colors
	textResult := executeCalendarColorsTest(t, svc, "--account", "a@b.com", "calendar", "colors")
	if textResult.err != nil {
		t.Fatalf("Execute text: %v", textResult.err)
	}

	if !strings.Contains(textResult.stderr, "No colors available") {
		t.Errorf("expected 'No colors available' message, got: %q", textResult.stderr)
	}
}
