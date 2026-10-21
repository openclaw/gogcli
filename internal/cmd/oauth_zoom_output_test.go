package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/zoom"
)

type oauthOutputRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthOutputRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestZoomOAuthDoctorOutput(t *testing.T) {
	// The doctor constructs its own Zoom client. Redirect its default transport
	// to loopback and reject every unexpected request before any network call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, oauthOutputSecret)
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	base := srv.Client().Transport
	original := http.DefaultTransport
	http.DefaultTransport = oauthOutputRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Host != "zoom.us" || r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected Zoom request: %s %s", r.Method, r.URL.Path)
			return nil, errors.New("unexpected synthetic Zoom request")
		}
		local := r.Clone(r.Context())
		local.URL.Scheme = target.Scheme
		local.URL.Host = target.Host
		local.Host = target.Host
		return base.RoundTrip(local)
	})
	t.Cleanup(func() { http.DefaultTransport = original })

	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonMode), func(t *testing.T) {
			fixture := newZoomAuthFixture(t)
			store := fixture.store(t)
			if err := store.StoreCredentials("default", zoom.Metadata{AccountID: "synthetic-account", ClientID: "synthetic-client"}, "synthetic-client-secret"); err != nil {
				t.Fatal(err)
			}
			creds, err := store.LoadCredentials("default")
			if err != nil {
				t.Fatal(err)
			}
			client, err := zoom.NewClient("default", creds, store)
			if err != nil {
				t.Fatal(err)
			}
			cause := client.Validate(t.Context())
			if !errors.Is(cause, zoom.ErrZoomTokenRequestFailed) || !strings.Contains(cause.Error(), oauthOutputSecret) {
				t.Fatal("fixture must preserve the original Zoom token error and provider body")
			}
			var output bytes.Buffer
			ctx := fixture.context(newCmdRuntimeOutputContext(t, &output, &output))
			if jsonMode {
				ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			}
			if err := (&ZoomAuthDoctorCmd{}).Run(ctx, &RootFlags{}); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), oauthOutputSecret) || !strings.Contains(output.String(), "OAuth token request failed.") {
				t.Fatalf("unsafe or missing doctor diagnostic: %s", output.String())
			}
		})
	}
}

func TestExecute_ZoomOAuthLogOutput(t *testing.T) {
	setTestConfigHome(t)
	cause := fmt.Errorf("%w: %s", zoom.ErrZoomTokenRequestFailed, oauthOutputSecret)
	result := executeWithTestRuntime(t, []string{"--verbose", "--json", "--no-input", "--account", "reader@example.invalid", "gmail", "labels", "list"}, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) {
			slog.Debug("synthetic Zoom failure", "err", cause)
			return nil, cause
		},
	}})
	if ExitCode(result.err) != 1 || !errors.Is(result.err, zoom.ErrZoomTokenRequestFailed) || !strings.Contains(result.stderr, "synthetic Zoom failure") || strings.Contains(result.stdout+result.stderr, oauthOutputSecret) {
		t.Fatalf("unexpected Zoom log output: %s%s", result.stdout, result.stderr)
	}
	assertOnlyJSONError(t, result.stdout, 1)
}

func TestCalendarZoomOAuthCleanupOutput(t *testing.T) {
	zoomClient := &fakeZoomCalendarClient{deleteErr: fmt.Errorf("%w: %s", zoom.ErrZoomTokenRequestFailed, oauthOutputSecret)}
	var patched atomic.Bool
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/synthetic-calendar@example.invalid":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-calendar@example.invalid", "timeZone": "UTC"})
		case r.Method == http.MethodGet && path == "/calendars/synthetic-calendar@example.invalid/events/ev":
			_ = json.NewEncoder(w).Encode(zoomEventJSON("ev", "999"))
		case r.Method == http.MethodPatch && path == "/calendars/synthetic-calendar@example.invalid/events/ev":
			patched.Store(true)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
		default:
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})))
	defer srv.Close()
	svc := newCalendarServiceFromZoomTestServer(t, t.Context(), srv)
	var output, warnings bytes.Buffer
	ctx := withFakeZoomClient(withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &output, &warnings), svc), zoomClient)
	if err := runKong(t, &CalendarUpdateCmd{}, []string{"synthetic-calendar@example.invalid", "ev", "--remove-zoom"}, ctx, &RootFlags{Account: "reader@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if !patched.Load() || len(zoomClient.deleted) != 1 || zoomClient.deleted[0] != "999" {
		t.Fatal("cleanup failure must still permit the Calendar update")
	}
	if strings.Contains(output.String()+warnings.String(), oauthOutputSecret) || !strings.Contains(warnings.String(), "OAuth token request failed.") {
		t.Fatalf("unsafe or missing cleanup diagnostic: %s", warnings.String())
	}
}

func TestCalendarZoomOAuthRollbackOutput(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			zoomClient := &fakeZoomCalendarClient{deleteErr: fmt.Errorf("%w: %s", zoom.ErrZoomTokenRequestFailed, oauthOutputSecret)}
			srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
				switch {
				case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev", "summary": "Existing"})
				case (r.Method == http.MethodPost && path == "/calendars/cal@example.com/events") || (r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev"):
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"synthetic calendar write denied"}}`)
				default:
					t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			})))
			defer srv.Close()
			svc := newCalendarServiceFromZoomTestServer(t, t.Context(), srv)
			ctx := newZoomCalendarTestJSONContext(t, svc, zoomClient)
			var err error
			if operation == "create" {
				err = runKong(t, &CalendarCreateCmd{}, []string{"cal@example.com", "--summary", "Zoom", "--from", "2026-10-22T10:00:00Z", "--to", "2026-10-22T11:00:00Z", "--with-zoom"}, ctx, &RootFlags{Account: "reader@example.invalid"})
			} else {
				err = runKong(t, &CalendarUpdateCmd{}, []string{"cal@example.com", "ev", "--with-zoom"}, ctx, &RootFlags{Account: "reader@example.invalid"})
			}
			message := errorMessage(nil, err)
			for _, want := range []string{"Zoom meeting rollback failed", "synthetic calendar write denied", "OAuth token request failed.", "gog zoom auth setup"} {
				if !strings.Contains(message, want) {
					t.Errorf("formatted error missing %q: %s", want, message)
				}
			}
			if strings.Contains(message, oauthOutputSecret) {
				t.Fatalf("provider detail leaked: %s", message)
			}
			if !errors.Is(err, zoomClient.deleteErr) || zoomClient.created != 1 || len(zoomClient.deleted) != 1 {
				t.Fatalf("rollback cause or execution lost: created=%d deleted=%v", zoomClient.created, zoomClient.deleted)
			}
		})
	}
}
