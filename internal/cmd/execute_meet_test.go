package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/meet/v2"
	"google.golang.org/api/option"
)

func newTestMeetService(t *testing.T, handler http.Handler) {
	t.Helper()

	origNew := newMeetService
	t.Cleanup(func() { newMeetService = origNew })

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := meet.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	newMeetService = func(context.Context, string) (*meet.Service, error) { return svc, nil }
}

func meetSpaceResponse() map[string]any {
	return map[string]any{
		"name":        "spaces/abc123",
		"meetingUri":  "https://meet.google.com/abc-defg-hij",
		"meetingCode": "abc-defg-hij",
		"config": map[string]any{
			"accessType":       "TRUSTED",
			"entryPointAccess": "ALL",
		},
	}
}

func TestExecute_MeetCreate_JSON(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "meet", "create"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Created     bool   `json:"created"`
		MeetingURI  string `json:"meeting_uri"`
		MeetingCode string `json:"meeting_code"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if !parsed.Created {
		t.Fatal("expected created=true")
	}

	if parsed.MeetingCode != "abc-defg-hij" {
		t.Fatalf("unexpected meeting_code: %q", parsed.MeetingCode)
	}

	if parsed.MeetingURI != "https://meet.google.com/abc-defg-hij" {
		t.Fatalf("unexpected meeting_uri: %q", parsed.MeetingURI)
	}
}

func TestExecute_MeetCreate_Text(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "meet", "create"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "meeting_code\tabc-defg-hij") {
		t.Fatalf("expected meeting_code in output, got: %q", out)
	}

	if !strings.Contains(out, "meeting_uri\thttps://meet.google.com/abc-defg-hij") {
		t.Fatalf("expected meeting_uri in output, got: %q", out)
	}

	if !strings.Contains(out, "access\ttrusted") {
		t.Fatalf("expected access in output, got: %q", out)
	}
}

func TestExecute_MeetCreate_DryRun(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not call API in dry-run mode")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--json", "--dry-run", "--account", "a@b.com", "meet", "create"})
			if err != nil && ExitCode(err) != 0 {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DryRun bool   `json:"dry_run"`
		Op     string `json:"op"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if !parsed.DryRun {
		t.Fatal("expected dry_run=true")
	}

	if parsed.Op != "meet.spaces.create" {
		t.Fatalf("unexpected op: %q", parsed.Op)
	}
}

func TestExecute_MeetGet_JSON(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "meet", "get", "abc-defg-hij"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		MeetingCode string `json:"meeting_code"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if parsed.MeetingCode != "abc-defg-hij" {
		t.Fatalf("unexpected meeting_code: %q", parsed.MeetingCode)
	}
}

func TestExecute_MeetHistory_JSON(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meetSpaceResponse())
		case r.URL.Path == "/v2/conferenceRecords" && r.Method == http.MethodGet:
			if got, want := r.URL.Query().Get("filter"), `space.name = "spaces/abc123"`; got != want {
				t.Fatalf("unexpected filter: %q, want %q", got, want)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"conferenceRecords": []map[string]any{
					{
						"name":      "conferenceRecords/rec1",
						"space":     "spaces/abc123",
						"startTime": "2026-03-20T10:00:00Z",
						"endTime":   "2026-03-20T11:00:00Z",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "meet", "history", "abc-defg-hij"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		MeetingCode string `json:"meeting_code"`
		Conferences []struct {
			Name string `json:"name"`
		} `json:"conferences"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if parsed.MeetingCode != "abc-defg-hij" {
		t.Fatalf("unexpected meeting_code: %q", parsed.MeetingCode)
	}

	if len(parsed.Conferences) != 1 || parsed.Conferences[0].Name != "conferenceRecords/rec1" {
		t.Fatalf("unexpected conferences: %#v", parsed.Conferences)
	}
}

func TestExecute_MeetParticipants_JSON(t *testing.T) {
	newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meetSpaceResponse())
		case r.URL.Path == "/v2/conferenceRecords" && r.Method == http.MethodGet:
			if got, want := r.URL.Query().Get("filter"), `space.name = "spaces/abc123"`; got != want {
				t.Fatalf("unexpected filter: %q, want %q", got, want)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"conferenceRecords": []map[string]any{
					{
						"name":      "conferenceRecords/rec1",
						"space":     "spaces/abc123",
						"startTime": "2026-03-20T10:00:00Z",
						"endTime":   "2026-03-20T11:00:00Z",
					},
				},
			})
		case r.URL.Path == "/v2/conferenceRecords/rec1/participants" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"participants": []map[string]any{
					{
						"name":              "conferenceRecords/rec1/participants/p1",
						"earliestStartTime": "2026-03-20T10:00:00Z",
						"latestEndTime":     "2026-03-20T11:00:00Z",
						"signedinUser": map[string]any{
							"displayName": "Dan Wager",
							"user":        "users/123",
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "meet", "participants", "abc-defg-hij"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		MeetingCode  string `json:"meeting_code"`
		Participants []struct {
			SignedinUser struct {
				DisplayName string `json:"displayName"`
			} `json:"signedinUser"`
		} `json:"participants"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if parsed.MeetingCode != "abc-defg-hij" {
		t.Fatalf("unexpected meeting_code: %q", parsed.MeetingCode)
	}

	if len(parsed.Participants) != 1 || parsed.Participants[0].SignedinUser.DisplayName != "Dan Wager" {
		t.Fatalf("unexpected participants: %#v", parsed.Participants)
	}
}
