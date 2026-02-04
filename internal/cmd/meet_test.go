package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/meet/v2"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// Helper functions

func testMeetContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func testMeetContextWithStdout(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func stubMeet(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newMeetService
	svc, err := meet.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new meet service: %v", err)
	}
	newMeetService = func(context.Context, string) (*meet.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newMeetService = orig
		srv.Close()
	})
	return srv
}

// MeetSpacesListCmd tests

func TestMeetSpacesListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/conferenceRecords") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conferenceRecords": []map[string]any{
				{"name": "conferenceRecords/1", "space": "spaces/space1", "startTime": "2026-01-01T00:00:00Z"},
			},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextJSON(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		ConferenceRecords []struct {
			Name      string `json:"name"`
			Space     string `json:"space"`
			StartTime string `json:"startTime"`
		} `json:"conferenceRecords"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.ConferenceRecords) != 1 {
		t.Fatalf("expected 1 record, got %d", len(parsed.ConferenceRecords))
	}
	if parsed.ConferenceRecords[0].Space != "spaces/space1" {
		t.Fatalf("unexpected space: %s", parsed.ConferenceRecords[0].Space)
	}
}

func TestMeetSpacesListCmd_EmptyResults(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/conferenceRecords") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conferenceRecords": []map[string]any{},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesListCmd{}

	// Empty results should not error, just show message on stderr
	if err := cmd.Run(testMeetContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMeetSpacesListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/conferenceRecords") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conferenceRecords": []map[string]any{
				{"name": "conferenceRecords/1", "space": "spaces/space1", "startTime": "2026-01-01T00:00:00Z"},
			},
			"nextPageToken": "token123",
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesListCmd{}

	ctx := testContextJSON(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["nextPageToken"] != "token123" {
		t.Fatalf("unexpected nextPageToken: %v", parsed["nextPageToken"])
	}
}

func TestMeetSpacesListCmd_WithPaging(t *testing.T) {
	var gotPageSize, gotPageToken string

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/conferenceRecords") {
			http.NotFound(w, r)
			return
		}
		gotPageSize = r.URL.Query().Get("pageSize")
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conferenceRecords": []map[string]any{
				{"name": "conferenceRecords/1", "space": "spaces/space1", "startTime": "2026-01-01T00:00:00Z"},
			},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesListCmd{Max: 25, Page: "mytoken"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testMeetContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageSize != "25" {
		t.Fatalf("unexpected pageSize: %q", gotPageSize)
	}
	if gotPageToken != "mytoken" {
		t.Fatalf("unexpected pageToken: %q", gotPageToken)
	}
}

func TestMeetSpacesListCmd_DeduplicatesSpaces(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/conferenceRecords") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conferenceRecords": []map[string]any{
				{"name": "conferenceRecords/1", "space": "spaces/space1", "startTime": "2026-01-01T10:00:00Z", "endTime": "2026-01-01T11:00:00Z"},
				{"name": "conferenceRecords/2", "space": "spaces/space1", "startTime": "2026-01-02T10:00:00Z", "endTime": "2026-01-02T11:00:00Z"},
				{"name": "conferenceRecords/3", "space": "spaces/space2", "startTime": "2026-01-01T10:00:00Z"},
			},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testMeetContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should show each space once with the latest record
	space1Count := strings.Count(out, "spaces/space1")
	space2Count := strings.Count(out, "spaces/space2")
	if space1Count != 1 || space2Count != 1 {
		t.Fatalf("expected each space once, got space1=%d space2=%d in output=%q", space1Count, space2Count, out)
	}

	// Should show the later date for space1 (2026-01-02)
	if !strings.Contains(out, "2026-01-02") {
		t.Fatalf("expected newer date for space1, got=%q", out)
	}
}

// MeetSpacesGetCmd tests

func TestMeetSpacesGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/spaces/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/abc123",
			"meetingCode": "abc-defg-hij",
			"meetingUri":  "https://meet.google.com/abc-defg-hij",
			"config": map[string]any{
				"accessType": "TRUSTED",
			},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesGetCmd{Space: "abc123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "spaces/abc123") || !strings.Contains(out, "abc-defg-hij") || !strings.Contains(out, "TRUSTED") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestMeetSpacesGetCmd_EmptySpace(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesGetCmd{Space: ""}

	err := cmd.Run(testMeetContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty space")
	}
	if !strings.Contains(err.Error(), "space is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetSpacesGetCmd_WithSpacePrefix(t *testing.T) {
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/spaces/") {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/abc123",
			"meetingCode": "abc-defg-hij",
			"meetingUri":  "https://meet.google.com/abc-defg-hij",
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesGetCmd{Space: "spaces/abc123"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should not double the prefix
	if strings.Contains(gotPath, "spaces/spaces/") {
		t.Fatalf("unexpected double prefix in path: %q", gotPath)
	}
}

func TestMeetSpacesGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/spaces/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/abc123",
			"meetingCode": "abc-defg-hij",
			"meetingUri":  "https://meet.google.com/abc-defg-hij",
			"config": map[string]any{
				"accessType": "OPEN",
			},
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesGetCmd{Space: "abc123"}

	ctx := testContextJSON(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["name"] != "spaces/abc123" {
		t.Fatalf("unexpected name: %v", parsed["name"])
	}
}

// MeetSpacesCreateCmd tests

func TestMeetSpacesCreateCmd(t *testing.T) {
	var gotAccess string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/spaces") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Config struct {
				AccessType string `json:"accessType"`
			} `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotAccess = payload.Config.AccessType
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "spaces/space1", "meetingUri": "https://meet.google.com/abc-defg-hij"})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesCreateCmd{AccessType: "OPEN"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotAccess != "OPEN" {
		t.Fatalf("unexpected access type: %q", gotAccess)
	}
	if !strings.Contains(out, "Created space") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestMeetSpacesCreateCmd_NoAccessType(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/spaces") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/newspace",
			"meetingCode": "new-meet-ing",
			"meetingUri":  "https://meet.google.com/new-meet-ing",
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesCreateCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextJSON(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Name        string `json:"name"`
		MeetingCode string `json:"meetingCode"`
		MeetingURI  string `json:"meetingUri"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Name != "spaces/newspace" {
		t.Fatalf("unexpected name: %s", parsed.Name)
	}
}

func TestMeetSpacesCreateCmd_InvalidAccessType(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesCreateCmd{AccessType: "INVALID"}

	err := cmd.Run(testMeetContext(t), flags)
	if err == nil {
		t.Fatal("expected error for invalid access type")
	}
	if !strings.Contains(err.Error(), "invalid --access-type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetSpacesCreateCmd_AllAccessTypes(t *testing.T) {
	for _, accessType := range []string{"OPEN", "TRUSTED", "RESTRICTED", "open", "trusted", "restricted"} {
		t.Run(accessType, func(t *testing.T) {
			var gotAccess string
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/spaces") {
					http.NotFound(w, r)
					return
				}
				var payload struct {
					Config struct {
						AccessType string `json:"accessType"`
					} `json:"config"`
				}
				_ = json.NewDecoder(r.Body).Decode(&payload)
				gotAccess = payload.Config.AccessType
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name":       "spaces/space1",
					"meetingUri": "https://meet.google.com/abc-defg-hij",
				})
			})
			stubMeet(t, h)

			flags := &RootFlags{Account: "user@example.com"}
			cmd := &MeetSpacesCreateCmd{AccessType: accessType}

			_ = captureStdout(t, func() {
				if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
					t.Fatalf("Run: %v", err)
				}
			})

			expected := strings.ToUpper(accessType)
			if gotAccess != expected {
				t.Fatalf("expected access type %q, got %q", expected, gotAccess)
			}
		})
	}
}

func TestMeetSpacesCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/spaces") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/newspace",
			"meetingCode": "new-meet-ing",
			"meetingUri":  "https://meet.google.com/new-meet-ing",
		})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesCreateCmd{}

	ctx := testContextJSON(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["name"] != "spaces/newspace" {
		t.Fatalf("unexpected name: %v", parsed["name"])
	}
}

// MeetSpacesEndCmd tests

func TestMeetSpacesEndCmd(t *testing.T) {
	var ended bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/spaces/space1:endActiveConference") {
			http.NotFound(w, r)
			return
		}
		ended = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesEndCmd{Space: "space1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !ended {
		t.Fatalf("expected end request")
	}
	if !strings.Contains(out, "Ended active conference") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestMeetSpacesEndCmd_EmptySpace(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesEndCmd{Space: ""}

	err := cmd.Run(testMeetContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty space")
	}
	if !strings.Contains(err.Error(), "space is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetSpacesEndCmd_WhitespaceOnlySpace(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesEndCmd{Space: "   "}

	err := cmd.Run(testMeetContext(t), flags)
	if err == nil {
		t.Fatal("expected error for whitespace-only space")
	}
	if !strings.Contains(err.Error(), "space is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMeetSpacesEndCmd_WithSpacePrefix(t *testing.T) {
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":endActiveConference") {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesEndCmd{Space: "spaces/abc123"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testMeetContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should not double the prefix
	if strings.Contains(gotPath, "spaces/spaces/") {
		t.Fatalf("unexpected double prefix in path: %q", gotPath)
	}
}

func TestMeetSpacesEndCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":endActiveConference") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubMeet(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &MeetSpacesEndCmd{Space: "abc123"}

	ctx := testContextJSON(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["space"] != "spaces/abc123" {
		t.Fatalf("unexpected space: %v", parsed["space"])
	}
	if parsed["ended"] != true {
		t.Fatalf("unexpected ended: %v", parsed["ended"])
	}
}
