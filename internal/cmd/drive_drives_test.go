package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestDriveDrivesCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/drives"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"drives": []map[string]any{
					{
						"id":          "0ABCD1234",
						"name":        "Engineering",
						"createdTime": "2024-01-15T10:30:00Z",
						"kind":        "drive#drive",
					},
					{
						"id":          "0EFGH5678",
						"name":        "Marketing",
						"createdTime": "2024-03-22T14:15:00Z",
						"kind":        "drive#drive",
					},
				},
				"nextPageToken": "npt123",
				"kind":          "drive#driveList",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}

	// Text mode: table to stdout + next page hint to stderr.
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DriveDrivesCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "NAME") || !strings.Contains(textOut, "CREATED") {
		t.Fatalf("unexpected table header: %q", textOut)
	}
	if !strings.Contains(textOut, "0ABCD1234") || !strings.Contains(textOut, "Engineering") {
		t.Fatalf("missing first drive row: %q", textOut)
	}
	if !strings.Contains(textOut, "0EFGH5678") || !strings.Contains(textOut, "Marketing") {
		t.Fatalf("missing second drive row: %q", textOut)
	}
	if !strings.Contains(textOut, "2024-01-15 10:30") || !strings.Contains(textOut, "2024-03-22 14:15") {
		t.Fatalf("missing formatted created times: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt123") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// JSON mode: JSON to stdout and no next-page hint to stderr.
	var errBuf2 bytes.Buffer
	u2, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf2, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx2 := ui.WithUI(context.Background(), u2)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveDrivesCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx2, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		Drives        []*drive.Drive `json:"drives"`
		NextPageToken string         `json:"nextPageToken"`
	}
	if unmarshalErr := json.Unmarshal([]byte(jsonOut), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, jsonOut)
	}
	if parsed.NextPageToken != "npt123" || len(parsed.Drives) != 2 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if parsed.Drives[0].Name != "Engineering" || parsed.Drives[1].Name != "Marketing" {
		t.Fatalf("unexpected drive names: %v, %v", parsed.Drives[0].Name, parsed.Drives[1].Name)
	}
}

func TestDriveDrivesCmd_Empty(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"drives": []map[string]any{},
			"kind":   "drive#driveList",
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}

	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	_ = captureStdout(t, func() {
		cmd := &DriveDrivesCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if !strings.Contains(errBuf.String(), "No shared drives") {
		t.Fatalf("expected 'No shared drives' message, got: %q", errBuf.String())
	}
}

func TestDriveDrivesCmd_WithQuery(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	var capturedQuery string
	var capturedFields string
	var capturedPageSize string
	var capturedPageToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("q")
		capturedFields = r.URL.Query().Get("fields")
		capturedPageSize = r.URL.Query().Get("pageSize")
		capturedPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"drives": []map[string]any{
				{
					"id":          "0ABCD1234",
					"name":        "Engineering",
					"createdTime": "2024-01-15T10:30:00Z",
				},
			},
			"kind": "drive#driveList",
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	_ = captureStdout(t, func() {
		cmd := &DriveDrivesCmd{}
		if execErr := runKong(t, cmd, []string{"--query", " name contains 'Eng' ", "--page", " tok ", "--max", "7"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if capturedQuery != "name contains 'Eng'" {
		t.Fatalf("expected query to be passed, got: %q", capturedQuery)
	}
	if capturedPageSize != "7" {
		t.Fatalf("expected pageSize=7, got: %q", capturedPageSize)
	}
	if got := strings.ReplaceAll(capturedFields, " ", ""); got != "nextPageToken,drives(id,name,createdTime)" {
		t.Fatalf("unexpected fields: %q", capturedFields)
	}
	if capturedPageToken != "tok" {
		t.Fatalf("expected page token to be passed, got: %q", capturedPageToken)
	}
}

func TestDriveDrivesCmd_TextPlain_MissingCreatedTime(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"drives": []map[string]any{
				{
					"id":   "0ABCD1234",
					"name": "Engineering",
				},
			},
			"kind": "drive#driveList",
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "test@example.com"}

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

	out := captureStdout(t, func() {
		cmd := &DriveDrivesCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(out, "ID\tNAME\tCREATED\n") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "0ABCD1234\tEngineering\t-\n") {
		t.Fatalf("missing row w/ '-' created time: %q", out)
	}
}
