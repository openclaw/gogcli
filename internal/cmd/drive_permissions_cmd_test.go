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

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestDrivePermissionsCmd_TextAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/id1/permissions"):
			if r.URL.Query().Get("pageSize") != "1" {
				t.Fatalf("expected pageSize=1, got: %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("pageToken") != "p1" {
				t.Fatalf("expected pageToken=p1, got: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "anyone", "role": "reader", "emailAddress": "a@b.com"},
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

	cmd := &DrivePermissionsCmd{}
	if execErr := runKong(t, cmd, []string{"--max", "1", "--page", "p1", "id1"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if !strings.Contains(textOut.String(), "ID") || !strings.Contains(textOut.String(), "TYPE") {
		t.Fatalf("unexpected table header: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "p1") || !strings.Contains(textOut.String(), "anyone") || !strings.Contains(textOut.String(), "reader") {
		t.Fatalf("missing permission row: %q", textOut.String())
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// JSON mode: JSON to stdout and no next-page hint to stderr.
	var jsonOut bytes.Buffer
	var errBuf2 bytes.Buffer
	ctx2 := withDriveTestService(newCmdRuntimeOutputContext(t, &jsonOut, &errBuf2), svc)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	cmd = &DrivePermissionsCmd{}
	if execErr := runKong(t, cmd, []string{"--max", "1", "--page", "p1", "id1"}, ctx2, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		FileID          string              `json:"fileId"`
		PermissionCount int                 `json:"permissionCount"`
		Permissions     []*drive.Permission `json:"permissions"`
		NextPageToken   string              `json:"nextPageToken"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut.String())
	}
	if parsed.FileID != "id1" || parsed.NextPageToken != "npt" || parsed.PermissionCount != 1 || len(parsed.Permissions) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestDrivePermissionsCmd_OmitsEmptyPageToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/id1/permissions"):
			if r.URL.Query().Get("pageSize") != "1" {
				t.Fatalf("expected pageSize=1, got: %q", r.URL.RawQuery)
			}
			if _, ok := r.URL.Query()["pageToken"]; ok {
				t.Fatalf("expected pageToken omitted, got: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "user", "role": "owner", "emailAddress": "a@b.com"},
				},
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
	var out bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &DrivePermissionsCmd{}
	if execErr := runKong(t, cmd, []string{"--max", "1", "id1"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	var parsed struct {
		PermissionCount int                 `json:"permissionCount"`
		Permissions     []*drive.Permission `json:"permissions"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out.String())
	}
	if parsed.PermissionCount != 1 || len(parsed.Permissions) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}
