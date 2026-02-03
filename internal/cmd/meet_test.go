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
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "spaces/space1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

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
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
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
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
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
