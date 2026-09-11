package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestDriveLsCmd_TextAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/drive/v3/files" || r.URL.Path == "/files"):
			if got := r.URL.Query().Get("orderBy"); got != "modifiedTime desc" {
				t.Errorf("default orderBy = %q", got)
			}
			if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
				http.Error(w, errMsg, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":            "f1",
						"name":          "Doc",
						"mimeType":      "application/pdf",
						"size":          "1024",
						"modifiedTime":  "2025-12-12T14:37:47Z",
						"hasThumbnail":  true,
						"thumbnailLink": "https://thumb.example/f1",
						"owners": []map[string]any{
							{"emailAddress": "owner@example.com"},
						},
					},
					{
						"id":           "d1",
						"name":         "Folder",
						"mimeType":     "application/vnd.google-apps.folder",
						"size":         "0",
						"modifiedTime": "2025-12-11T00:00:00Z",
					},
					{
						"id":           "s1",
						"name":         "Folder shortcut",
						"mimeType":     driveMimeShortcut,
						"modifiedTime": "2025-12-10T00:00:00Z",
						"shortcutDetails": map[string]any{
							"targetId":       "d1",
							"targetMimeType": driveMimeFolder,
						},
					},
				},
				"nextPageToken": "npt",
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

	flags := &RootFlags{Account: "a@b.com"}

	// Text mode: table to stdout + next page hint to stderr.
	var textOut bytes.Buffer
	var errBuf bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, &textOut, &errBuf), svc)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &DriveLsCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	if !strings.Contains(textOut.String(), "ID") || !strings.Contains(textOut.String(), "NAME") || !strings.Contains(textOut.String(), "OWNER") {
		t.Fatalf("unexpected table header: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "f1") || !strings.Contains(textOut.String(), "Doc") || !strings.Contains(textOut.String(), "1.0 KB") || !strings.Contains(textOut.String(), "owner@example.com") {
		t.Fatalf("missing file row: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "d1") || !strings.Contains(textOut.String(), "Folder") || !strings.Contains(textOut.String(), "folder") {
		t.Fatalf("missing folder row: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "s1") || !strings.Contains(textOut.String(), "shortcut") || !strings.Contains(textOut.String(), "d1") {
		t.Fatalf("missing shortcut row: %q", textOut.String())
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// JSON mode: JSON to stdout and no next-page hint to stderr.
	var jsonOut bytes.Buffer
	var errBuf2 bytes.Buffer
	ctx2 := withDriveTestService(newCmdRuntimeOutputContext(t, &jsonOut, &errBuf2), svc)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	cmd = &DriveLsCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx2, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		Files         []*drive.File `json:"files"`
		NextPageToken string        `json:"nextPageToken"`
	}
	if unmarshalErr := json.Unmarshal(jsonOut.Bytes(), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, jsonOut.String())
	}
	if parsed.NextPageToken != "npt" || len(parsed.Files) != 3 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if !parsed.Files[0].HasThumbnail || parsed.Files[0].ThumbnailLink != "https://thumb.example/f1" {
		t.Fatalf("expected thumbnail fields in json, got %#v", parsed.Files[0])
	}
	if got := driveShortcutTargetID(parsed.Files[2]); got != "d1" {
		t.Fatalf("shortcut target = %q, want d1", got)
	}

	// Plain mode: stable TSV (tabs preserved).
	var plainOut bytes.Buffer
	var errBuf3 bytes.Buffer
	ctx3 := withDriveTestService(newCmdRuntimeOutputContext(t, &plainOut, &errBuf3), svc)
	ctx3 = outfmt.WithMode(ctx3, outfmt.Mode{Plain: true})

	cmd = &DriveLsCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx3, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if !strings.Contains(plainOut.String(), "ID\tNAME\tTYPE\tSIZE\tMODIFIED\tOWNER\n") {
		t.Fatalf("expected TSV header, got: %q", plainOut.String())
	}
	if strings.Contains(plainOut.String(), "TARGET_ID") {
		t.Fatalf("plain output schema changed unexpectedly: %q", plainOut.String())
	}
}

func TestDriveLsCmd_NoAllDrives(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if errMsg := driveAllDrivesQueryError(r, false); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &DriveLsCmd{}
	if execErr := runKong(t, cmd, []string{"--no-all-drives"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
}

func TestDriveLsCmd_Sort(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--sort", "modifiedByMeTime"}, "modifiedByMeTime desc"},
		{[]string{"--sort", "name_natural", "--order", "asc"}, "name_natural"},
		{[]string{"--order", "asc"}, "modifiedTime"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("orderBy"); got != tc.want {
					t.Errorf("orderBy = %q, want %q", got, tc.want)
				}
				if got := r.URL.Query().Get("pageToken"); got != "next-page" {
					t.Errorf("pageToken = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"files":[]}`)
			}))
			defer srv.Close()
			svc, err := drive.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
			args := slices.Clone(tc.args)
			args = append(args, "--page", "next-page")
			if err := runKong(t, &DriveLsCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDriveLsCmd_InvalidSort(t *testing.T) {
	for _, args := range [][]string{{"--sort", "unknown"}, {"--order", "sideways"}} {
		if err := runKong(t, &DriveLsCmd{}, args, context.Background(), &RootFlags{}); err == nil {
			t.Fatalf("expected invalid sort options to fail: %v", args)
		}
	}
}
