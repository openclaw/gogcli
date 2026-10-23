package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
)

type driveFieldsHit struct {
	lastFields atomic.Value // string
}

func newDriveFieldsTestServer(t *testing.T, handler func(r *http.Request) map[string]any, hit *driveFieldsHit) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hit != nil {
			hit.lastFields.Store(r.URL.Query().Get("fields"))
		}
		body := handler(r)
		if body == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// TestDriveGet_FieldsFlag proves --fields on `gog drive get` maps to the
// Drive API fields parameter, enabling requests for fields the hard-coded
// default did not include (e.g. thumbnailLink). Closes #486.
func TestDriveGet_FieldsFlag(t *testing.T) {
	t.Parallel()

	hit := &driveFieldsHit{}
	srv := newDriveFieldsTestServer(t, func(r *http.Request) map[string]any {
		if !strings.Contains(r.URL.Path, "/files/f1") {
			return nil
		}
		return map[string]any{
			"id":            "f1",
			"name":          "photo.png",
			"mimeType":      "image/png",
			"thumbnailLink": "https://drive.google.com/thumb/f1",
		}
	}, hit)
	defer srv.Close()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", drive.NewService)

	var stdout bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
	ctx = withDriveTestService(ctx, svc)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &DriveGetCmd{}, []string{"f1", "--fields", "id,name,thumbnailLink"}, ctx, flags); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, _ := hit.lastFields.Load().(string)
	if !strings.Contains(got, "thumbnailLink") {
		t.Fatalf("expected Drive API fields param to contain thumbnailLink, got: %q", got)
	}
	// Output should wrap under "file" per existing drive get -j contract.
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	file, ok := envelope["file"].(map[string]any)
	if !ok {
		t.Fatalf("expected file envelope, got: %v", envelope)
	}
	if file["thumbnailLink"] != "https://drive.google.com/thumb/f1" {
		t.Fatalf("expected thumbnailLink passthrough, got: %v", file["thumbnailLink"])
	}
}

// TestDriveLs_FieldsFlag proves --fields on `gog drive ls` maps through to
// the Drive API list fields parameter so consumers can request fields not
// in the hard-coded default set.
func TestDriveLs_FieldsFlag(t *testing.T) {
	t.Parallel()

	hit := &driveFieldsHit{}
	srv := newDriveFieldsTestServer(t, func(r *http.Request) map[string]any {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		if path != "/files" {
			return nil
		}
		return map[string]any{
			"files": []map[string]any{
				{
					"id":            "f1",
					"name":          "photo.png",
					"mimeType":      "image/png",
					"thumbnailLink": "https://drive.google.com/thumb/f1",
				},
			},
		}
	}, hit)
	defer srv.Close()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", drive.NewService)

	var stdout bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
	ctx = withDriveTestService(ctx, svc)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &DriveLsCmd{}, []string{"--fields", "files(id,name,thumbnailLink)"}, ctx, flags); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, _ := hit.lastFields.Load().(string)
	if !strings.Contains(got, "thumbnailLink") {
		t.Fatalf("expected Drive API fields param to contain thumbnailLink, got: %q", got)
	}
	if !strings.Contains(stdout.String(), "thumbnailLink") {
		t.Fatalf("expected thumbnailLink in output, got: %q", stdout.String())
	}
}

// TestDriveGet_JSONKeepsRequestedFalseBools proves requested boolean fields
// survive JSON output when false. The SDK's drive.File tags them omitempty,
// so without ForceSendFields `trashed: false` and `starred: false` vanish.
func TestDriveGet_JSONKeepsRequestedFalseBools(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		body    map[string]any
		want    map[string]bool
		notWant []string
	}{
		{
			name:    "explicit fields",
			args:    []string{"f1", "--fields", "id,name,trashed,starred,md5Checksum"},
			want:    map[string]bool{"trashed": false, "starred": false},
			notWant: []string{"hasThumbnail", "explicitlyTrashed"},
		},
		{
			name:    "true value and nested wildcard",
			args:    []string{"f1", "--fields", "id,capabilities/*,trashed"},
			body:    map[string]any{"trashed": true, "starred": false, "capabilities": map[string]any{"canEdit": false}},
			want:    map[string]bool{"trashed": true},
			notWant: []string{"starred", "hasThumbnail"},
		},
		{
			// Drive omits these for items they don't apply to; absent must not become false.
			name:    "conditional bools stay absent",
			args:    []string{"f1", "--fields", "*"},
			body:    map[string]any{"trashed": false, "starred": false},
			want:    map[string]bool{"trashed": false, "starred": false},
			notWant: []string{"shared", "ownedByMe", "writersCanShare", "hasAugmentedPermissions"},
		},
		{
			name: "default fields",
			args: []string{"f1"},
			want: map[string]bool{"starred": false, "hasThumbnail": false},
			// trashed is not in the default mask.
			notWant: []string{"trashed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newDriveFieldsTestServer(t, func(r *http.Request) map[string]any {
				if !strings.Contains(r.URL.Path, "/files/f1") {
					return nil
				}
				if tc.body != nil {
					body := map[string]any{"id": "f1"}
					for k, v := range tc.body {
						body[k] = v
					}
					return body
				}
				return map[string]any{
					"id":           "f1",
					"name":         "a.tar.gz",
					"md5Checksum":  "abc",
					"trashed":      false,
					"starred":      false,
					"hasThumbnail": false,
				}
			}, nil)
			defer srv.Close()
			svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", drive.NewService)

			var stdout bytes.Buffer
			ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
			ctx = withDriveTestService(ctx, svc)
			flags := &RootFlags{Account: "a@b.com"}

			if err := runKong(t, &DriveGetCmd{}, tc.args, ctx, flags); err != nil {
				t.Fatalf("run: %v", err)
			}
			var envelope struct {
				File map[string]any `json:"file"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
			}
			for key, want := range tc.want {
				got, ok := envelope.File[key]
				if !ok {
					t.Fatalf("missing %q in output: %s", key, stdout.String())
				}
				if got != want {
					t.Fatalf("%q = %v, want %v", key, got, want)
				}
			}
			for _, key := range tc.notWant {
				if _, ok := envelope.File[key]; ok {
					t.Fatalf("unexpected %q in output: %s", key, stdout.String())
				}
			}
		})
	}
}

// TestDriveLs_JSONKeepsRequestedFalseBools proves list JSON keeps requested
// false bools. The list mask wraps file fields in files(...), so the get-path
// top-level scan does not see them.
func TestDriveLs_JSONKeepsRequestedFalseBools(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		args    []string
		body    map[string]any
		want    map[string]bool
		notWant []string
	}
	cases := make([]testCase, 0, 7)
	cases = append(cases, testCase{
		name: "default fields",
		args: nil,
		body: map[string]any{
			"id":           "f1",
			"name":         "photo.png",
			"hasThumbnail": false,
			"trashed":      false,
		},
		want:    map[string]bool{"hasThumbnail": false},
		notWant: []string{"trashed"},
	}, testCase{
		name: "files mask",
		args: []string{"--fields", "files(id,trashed)"},
		body: map[string]any{
			"id":           "f1",
			"trashed":      false,
			"hasThumbnail": false,
		},
		want:    map[string]bool{"trashed": false},
		notWant: []string{"hasThumbnail"},
	})
	for _, mask := range []string{"files/trashed", "files(id),files(trashed)", "files", "files/*", "*"} {
		cases = append(cases, testCase{
			name:    mask,
			args:    []string{"--fields", mask},
			body:    map[string]any{"id": "f1", "trashed": false},
			want:    map[string]bool{"trashed": false},
			notWant: []string{"ownedByMe", "shared", "writersCanShare", "hasAugmentedPermissions"},
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newDriveFieldsTestServer(t, func(r *http.Request) map[string]any {
				path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
				if path != "/files" {
					return nil
				}
				return map[string]any{"files": []map[string]any{tc.body}}
			}, nil)
			defer srv.Close()
			svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", drive.NewService)

			var stdout bytes.Buffer
			ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
			ctx = withDriveTestService(ctx, svc)
			flags := &RootFlags{Account: "a@b.com"}

			if err := runKong(t, &DriveLsCmd{}, tc.args, ctx, flags); err != nil {
				t.Fatalf("run: %v", err)
			}
			var envelope struct {
				Files []map[string]any `json:"files"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
			}
			if len(envelope.Files) != 1 {
				t.Fatalf("files = %s", stdout.String())
			}
			file := envelope.Files[0]
			for key, want := range tc.want {
				got, ok := file[key]
				if !ok {
					t.Fatalf("missing %q in output: %s", key, stdout.String())
				}
				if got != want {
					t.Fatalf("%q = %v, want %v", key, got, want)
				}
			}
			for _, key := range tc.notWant {
				if _, ok := file[key]; ok {
					t.Fatalf("unexpected %q in output: %s", key, stdout.String())
				}
			}
		})
	}
}
