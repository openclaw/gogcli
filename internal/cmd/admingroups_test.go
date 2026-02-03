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

	"google.golang.org/api/groupssettings/v1"
	"google.golang.org/api/option"
)

func TestGroupsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/groups") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "engineering@example.com",
			"name":  "Engineering",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsCreateCmd{Email: "engineering@example.com", Name: "Engineering"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsSettingsCmd_Get(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/groups/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                "engineering@example.com",
			"whoCanJoin":           "INVITED_CAN_JOIN",
			"whoCanPostMessage":    "ALL_IN_DOMAIN_CAN_POST",
			"whoCanViewGroup":      "ALL_IN_DOMAIN_CAN_VIEW",
			"whoCanViewMembership": "ALL_IN_DOMAIN_CAN_VIEW",
		})
	})
	stubGroupsSettings(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsSettingsCmd{Group: "engineering@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "WhoCanJoin") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubGroupsSettings(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newGroupsSettings
	svc, err := groupssettings.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new groupssettings service: %v", err)
	}
	newGroupsSettings = func(context.Context, string) (*groupssettings.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newGroupsSettings = orig
		srv.Close()
	})
	return srv
}

func TestReadCSVEmails(t *testing.T) {
	content := "email\nalpha@example.com\nALPHA@example.com\nbeta@example.com\n"
	path := writeTempFile(t, content)
	got, err := readCSVEmails(path)
	if err != nil {
		t.Fatalf("readCSVEmails: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(got))
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "csv-*.csv")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return f.Name()
}
