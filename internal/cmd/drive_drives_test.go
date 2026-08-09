package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
)

var errUnexpectedDriveServiceCall = errors.New("unexpected drive service call")

func TestDriveDrivesCmd_TextAndJSON(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer closeSrv()

	flags := &RootFlags{Account: "test@example.com"}

	// Text mode: table to stdout + next page hint to stderr.
	var textOut bytes.Buffer
	var errBuf bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, &textOut, &errBuf), svc)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &DriveDrivesCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	if !strings.Contains(textOut.String(), "ID") || !strings.Contains(textOut.String(), "NAME") || !strings.Contains(textOut.String(), "CREATED") {
		t.Fatalf("unexpected table header: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "0ABCD1234") || !strings.Contains(textOut.String(), "Engineering") {
		t.Fatalf("missing first drive row: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "0EFGH5678") || !strings.Contains(textOut.String(), "Marketing") {
		t.Fatalf("missing second drive row: %q", textOut.String())
	}
	if !strings.Contains(textOut.String(), "2024-01-15 10:30") || !strings.Contains(textOut.String(), "2024-03-22 14:15") {
		t.Fatalf("missing formatted created times: %q", textOut.String())
	}
	if !strings.Contains(errBuf.String(), "--page npt123") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// JSON mode: JSON to stdout and no next-page hint to stderr.
	var jsonOut bytes.Buffer
	var errBuf2 bytes.Buffer
	ctx2 := withDriveTestService(newCmdRuntimeOutputContext(t, &jsonOut, &errBuf2), svc)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	cmd = &DriveDrivesCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx2, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		Drives        []*drive.Drive `json:"drives"`
		NextPageToken string         `json:"nextPageToken"`
	}
	if unmarshalErr := json.Unmarshal(jsonOut.Bytes(), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, jsonOut.String())
	}
	if parsed.NextPageToken != "npt123" || len(parsed.Drives) != 2 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if parsed.Drives[0].Name != "Engineering" || parsed.Drives[1].Name != "Marketing" {
		t.Fatalf("unexpected drive names: %v, %v", parsed.Drives[0].Name, parsed.Drives[1].Name)
	}
}

func TestDriveDrivesCmd_Empty(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"drives": []map[string]any{},
			"kind":   "drive#driveList",
		})
	}))
	defer closeSrv()

	flags := &RootFlags{Account: "test@example.com"}

	var errBuf bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, &errBuf), svc)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &DriveDrivesCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

	if !strings.Contains(errBuf.String(), "No shared drives") {
		t.Fatalf("expected 'No shared drives' message, got: %q", errBuf.String())
	}
}

func TestDriveDrivesCmd_WithQuery(t *testing.T) {
	var capturedQuery string
	var capturedFields string
	var capturedPageSize string
	var capturedPageToken string
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer closeSrv()

	flags := &RootFlags{Account: "test@example.com"}

	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &DriveDrivesCmd{}
	if execErr := runKong(t, cmd, []string{"--query", " name contains 'Eng' ", "--page", " tok ", "--max", "7"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}

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

func TestDriveDrivesCmd_InvalidMaxFailsBeforeService(t *testing.T) {
	ctx := withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*drive.Service, error) {
		t.Fatalf("expected max validation to fail before creating drive service")
		return nil, errUnexpectedDriveServiceCall
	})
	flags := &RootFlags{Account: "test@example.com"}

	for _, args := range [][]string{{"--max", "0"}, {"--max=-1"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			cmd := &DriveDrivesCmd{}
			err := runKong(t, cmd, args, ctx, flags)
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestDriveDrivesCmd_TextPlain_MissingCreatedTime(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer closeSrv()

	flags := &RootFlags{Account: "test@example.com"}

	var out bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})

	cmd := &DriveDrivesCmd{}
	if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if !strings.Contains(out.String(), "ID\tNAME\tCREATED\n") {
		t.Fatalf("missing header: %q", out.String())
	}
	if !strings.Contains(out.String(), "0ABCD1234\tEngineering\t-\n") {
		t.Fatalf("missing row w/ '-' created time: %q", out.String())
	}
}
