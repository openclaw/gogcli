package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	result := executeWithMeetTestService(t, []string{"--json", "--account", "a@b.com", "meet", "create"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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

func TestExecute_MeetCreate_OpenUsesRuntimeOperation(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	var openedURL string
	result := executeWithMeetTestOperations(
		t,
		[]string{"--json", "--account", "a@b.com", "meet", "create", "--open"},
		fixedMeetTestService(svc),
		func(_ context.Context, uri string) error {
			openedURL = uri
			return nil
		},
	)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if openedURL != "https://meet.google.com/abc-defg-hij" {
		t.Fatalf("opened URL = %q", openedURL)
	}
}

func TestExecute_MeetCreate_Text(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	result := executeWithMeetTestService(t, []string{"--plain", "--account", "a@b.com", "meet", "create"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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
	result := executeWithMeetTestOperations(
		t,
		[]string{"--json", "--dry-run", "--account", "a@b.com", "meet", "create"},
		unexpectedMeetTestService(t, "should not call API in dry-run mode"),
		nil,
	)
	if result.err != nil && ExitCode(result.err) != 0 {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}))

	result := executeWithMeetTestService(t, []string{
		"--json", "--account", "a@b.com",
		"meet", "get", "abc-defg-hij",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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

func TestExecute_MeetUpdate_JSON(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meetSpaceResponse())
		case r.URL.Path == "/v2/spaces/abc123" && r.Method == http.MethodPatch:
			if got := r.URL.Query().Get("updateMask"); got != "config.accessType" {
				t.Fatalf("update mask = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			response := meetSpaceResponse()
			response["config"] = map[string]any{
				"accessType":       "OPEN",
				"entryPointAccess": "ALL",
			}
			_ = json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))

	result := executeWithMeetTestService(t, []string{
		"--json", "--account", "a@b.com",
		"meet", "update", "abc-defg-hij", "--access", "open",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Updated bool `json:"updated"`
		Config  struct {
			AccessType string `json:"accessType"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if !parsed.Updated || parsed.Config.AccessType != "OPEN" {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}

func TestExecute_MeetEnd_JSON(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meetSpaceResponse())
		case r.URL.Path == "/v2/spaces/abc123:endActiveConference" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		default:
			http.NotFound(w, r)
		}
	}))

	result := executeWithMeetTestService(t, []string{
		"--json", "--force", "--account", "a@b.com",
		"meet", "end", "abc-defg-hij",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Ended       bool   `json:"ended"`
		MeetingCode string `json:"meeting_code"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if !parsed.Ended || parsed.MeetingCode != "abc-defg-hij" {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}

func TestExecute_MeetHistory_JSON(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithMeetTestService(t, []string{
		"--json", "--account", "a@b.com",
		"meet", "history", "abc-defg-hij",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithMeetTestService(t, []string{
		"--json", "--account", "a@b.com",
		"meet", "participants", "abc-defg-hij",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout

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

func TestExecute_MeetEmptyLists_JSON(t *testing.T) {
	participantListCalled := false
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(meetSpaceResponse())
		case r.URL.Path == "/v2/conferenceRecords" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case strings.Contains(r.URL.Path, "/participants"):
			participantListCalled = true
			http.Error(w, "unexpected participant list", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))

	tests := []struct {
		name  string
		args  []string
		field string
	}{
		{
			name:  "history",
			args:  []string{"meet", "history", "abc-defg-hij"},
			field: "conferences",
		},
		{
			name:  "participants",
			args:  []string{"meet", "participants", "abc-defg-hij"},
			field: "participants",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseArgs := append([]string{"--json", "--account", "a@b.com"}, tt.args...)
			result := executeWithMeetTestService(t, baseArgs, svc)
			if result.err != nil {
				t.Fatalf("Execute: %v", result.err)
			}
			assertMeetEmptyJSONArray(t, result.stdout, tt.field)

			failEmptyArgs := append(append([]string{}, baseArgs...), "--fail-empty")
			failEmptyResult := executeWithMeetTestService(t, failEmptyArgs, svc)
			if code := ExitCode(failEmptyResult.err); code != 3 {
				t.Fatalf("fail-empty exit = %d, want 3; err=%v", code, failEmptyResult.err)
			}
			assertMeetEmptyJSONArray(t, failEmptyResult.stdout, tt.field)
		})
	}
	if participantListCalled {
		t.Fatal("participant list API called without a conference record")
	}
}

func assertMeetEmptyJSONArray(t *testing.T, output, field string) {
	t.Helper()
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, output)
	}
	if got := string(parsed[field]); got != "[]" {
		t.Fatalf("%s = %s, want []", field, got)
	}
}
