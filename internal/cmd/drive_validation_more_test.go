package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDriveCommands_MissingAccount(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{}

	cases := []struct {
		name string
		run  func() error
	}{
		{"ls", func() error { return (&DriveLsCmd{}).Run(ctx, flags) }},
		{"search", func() error { return (&DriveSearchCmd{}).Run(ctx, flags) }},
		{"get", func() error { return (&DriveGetCmd{}).Run(ctx, flags) }},
		{"download", func() error { return (&DriveDownloadCmd{}).Run(ctx, flags) }},
		{"upload", func() error { return (&DriveUploadCmd{}).Run(ctx, flags) }},
		{"mkdir", func() error { return (&DriveMkdirCmd{}).Run(ctx, flags) }},
		{"delete", func() error { return (&DriveDeleteCmd{}).Run(ctx, flags) }},
		{"move", func() error { return (&DriveMoveCmd{}).Run(ctx, flags) }},
		{"rename", func() error { return (&DriveRenameCmd{}).Run(ctx, flags) }},
		{"share", func() error { return (&DriveShareCmd{}).Run(ctx, flags) }},
		{"unshare", func() error { return (&DriveUnshareCmd{}).Run(ctx, flags) }},
		{"permissions", func() error { return (&DrivePermissionsCmd{Max: 100}).Run(ctx, flags) }},
		{"url", func() error { return (&DriveURLCmd{}).Run(ctx, flags) }},
	}

	for _, tc := range cases {
		if err := tc.run(); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestDriveCommands_UsageErrors(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	cases := []struct {
		name string
		run  func() error
	}{
		{"search missing query", func() error { return (&DriveSearchCmd{}).Run(ctx, flags) }},
		{"get missing file", func() error { return (&DriveGetCmd{}).Run(ctx, flags) }},
		{"download missing file", func() error { return (&DriveDownloadCmd{}).Run(ctx, flags) }},
		{"upload missing path", func() error { return (&DriveUploadCmd{}).Run(ctx, flags) }},
		{"mkdir missing name", func() error { return (&DriveMkdirCmd{}).Run(ctx, flags) }},
		{"delete missing file", func() error { return (&DriveDeleteCmd{}).Run(ctx, flags) }},
		{"move missing file", func() error { return (&DriveMoveCmd{}).Run(ctx, flags) }},
		{"move missing parent", func() error { return (&DriveMoveCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"rename missing file", func() error { return (&DriveRenameCmd{}).Run(ctx, flags) }},
		{"rename missing name", func() error { return (&DriveRenameCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"share missing file", func() error { return (&DriveShareCmd{}).Run(ctx, flags) }},
		{"share missing target", func() error { return (&DriveShareCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"share invalid role", func() error { return (&DriveShareCmd{FileID: "f1", Email: "x@y.com", Role: "nope"}).Run(ctx, flags) }},
		{"unshare missing file", func() error { return (&DriveUnshareCmd{}).Run(ctx, flags) }},
		{"unshare missing perm", func() error { return (&DriveUnshareCmd{FileID: "f1"}).Run(ctx, flags) }},
		{"permissions missing file", func() error { return (&DrivePermissionsCmd{Max: 100}).Run(ctx, flags) }},
	}

	for _, tc := range cases {
		if err := tc.run(); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestDriveListSearchInvalidMaxFailsBeforeService(t *testing.T) {
	ctx := withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*drive.Service, error) {
		t.Fatalf("expected max validation to fail before creating drive service")
		return nil, errors.New("unexpected drive service call")
	})
	flags := &RootFlags{Account: "a@b.com"}
	cases := []struct {
		name string
		cmd  any
		args []string
	}{
		{name: "ls zero", cmd: &DriveLsCmd{}, args: []string{"--max", "0"}},
		{name: "ls negative", cmd: &DriveLsCmd{}, args: []string{"--max=-1"}},
		{name: "search zero", cmd: &DriveSearchCmd{}, args: []string{"query", "--max", "0"}},
		{name: "search negative", cmd: &DriveSearchCmd{}, args: []string{"query", "--max=-1"}},
		{name: "permissions zero", cmd: &DrivePermissionsCmd{}, args: []string{"file123", "--max", "0"}},
		{name: "permissions negative", cmd: &DrivePermissionsCmd{}, args: []string{"file123", "--max=-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runKong(t, tc.cmd, tc.args, ctx, flags)
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestDriveScanInvalidBoundsFailBeforeServiceOrDryRun(t *testing.T) {
	ctx := withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*drive.Service, error) {
		t.Fatalf("expected scan validation to fail before creating drive service")
		return nil, errors.New("unexpected drive service call")
	})
	flags := &RootFlags{Account: "a@b.com"}
	dryRunFlags := &RootFlags{Account: "a@b.com", DryRun: true}
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "tree max", run: func() error { return (&DriveTreeCmd{Max: -1}).Run(ctx, flags) }, want: "--max must be >= 0"},
		{name: "tree depth", run: func() error { return (&DriveTreeCmd{Depth: -1}).Run(ctx, flags) }, want: "--depth must be >= 0"},
		{name: "inventory max", run: func() error { return (&DriveInventoryCmd{Max: -1}).Run(ctx, flags) }, want: "--max must be >= 0"},
		{name: "inventory depth", run: func() error { return (&DriveInventoryCmd{Depth: -1}).Run(ctx, flags) }, want: "--depth must be >= 0"},
		{name: "du max", run: func() error { return (&DriveDuCmd{Max: -1}).Run(ctx, flags) }, want: "--max must be >= 0"},
		{name: "du depth", run: func() error { return (&DriveDuCmd{Depth: -1}).Run(ctx, flags) }, want: "--depth must be >= 0"},
		{name: "audit sharing max", run: func() error { return (&DriveAuditSharingCmd{Max: -1}).Run(ctx, flags) }, want: "--max must be >= 0"},
		{name: "audit sharing depth", run: func() error { return (&DriveAuditSharingCmd{Depth: -1}).Run(ctx, flags) }, want: "--depth must be >= 0"},
		{name: "audit user max", run: func() error { return (&DriveAuditUserCmd{User: "user@example.com", Max: -1}).Run(ctx, flags) }, want: "--max must be >= 0"},
		{name: "audit user depth", run: func() error { return (&DriveAuditUserCmd{User: "user@example.com", Depth: -1}).Run(ctx, flags) }, want: "--depth must be >= 0"},
		{name: "bulk remove max", run: func() error { return (&DriveBulkRemovePublicCmd{Max: -1}).Run(ctx, dryRunFlags) }, want: "--max must be >= 0"},
		{name: "bulk remove depth", run: func() error { return (&DriveBulkRemovePublicCmd{Depth: -1}).Run(ctx, dryRunFlags) }, want: "--depth must be >= 0"},
		{name: "bulk update max", run: func() error {
			return (&DriveBulkUpdateRoleCmd{From: "reader", To: "writer", Max: -1}).Run(ctx, dryRunFlags)
		}, want: "--max must be >= 0"},
		{name: "bulk update depth", run: func() error {
			return (&DriveBulkUpdateRoleCmd{From: "reader", To: "writer", Depth: -1}).Run(ctx, dryRunFlags)
		}, want: "--depth must be >= 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestDriveShare_DefaultRole(t *testing.T) {
	ctx := withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*drive.Service, error) {
		return nil, errors.New("no service")
	})
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DriveShareCmd{FileID: "f1", Email: "x@y.com"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNormalizeDrivePermissionRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		want    string
		wantErr bool
	}{
		{name: "default", role: "", want: drivePermRoleReader},
		{name: "trimmed", role: " writer ", want: drivePermRoleWriter},
		{name: "commenter", role: drivePermRoleCommenter, want: drivePermRoleCommenter},
		{name: "invalid", role: "owner", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDrivePermissionRole(tt.role)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDrivePermissionRole: %v", err)
			}
			if got != tt.want {
				t.Fatalf("role = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDriveShareNormalizeTarget(t *testing.T) {
	tests := []struct {
		name    string
		cmd     DriveShareCmd
		want    driveShareTarget
		wantErr string
	}{
		{name: "legacy anyone", cmd: DriveShareCmd{Anyone: true}, want: driveShareTarget{to: driveShareToAnyone}},
		{name: "legacy user", cmd: DriveShareCmd{Email: " x@example.com "}, want: driveShareTarget{to: driveShareToUser, email: "x@example.com"}},
		{name: "legacy domain", cmd: DriveShareCmd{Domain: " example.com "}, want: driveShareTarget{to: driveShareToDomain, domain: "example.com"}},
		{name: "explicit anyone", cmd: DriveShareCmd{To: " anyone "}, want: driveShareTarget{to: driveShareToAnyone}},
		{name: "explicit user", cmd: DriveShareCmd{To: driveShareToUser, Email: "x@example.com"}, want: driveShareTarget{to: driveShareToUser, email: "x@example.com"}},
		{name: "explicit domain", cmd: DriveShareCmd{To: driveShareToDomain, Domain: "example.com", Discoverable: true}, want: driveShareTarget{to: driveShareToDomain, domain: "example.com"}},
		{name: "missing target", cmd: DriveShareCmd{}, wantErr: "must specify --to"},
		{name: "ambiguous target", cmd: DriveShareCmd{Anyone: true, Email: "x@example.com"}, wantErr: "ambiguous share target"},
		{name: "anyone with email", cmd: DriveShareCmd{To: driveShareToAnyone, Email: "x@example.com"}, wantErr: "--to=anyone cannot be combined"},
		{name: "user without email", cmd: DriveShareCmd{To: driveShareToUser}, wantErr: "missing --email"},
		{name: "user invalid email", cmd: DriveShareCmd{To: driveShareToUser, Email: "nope"}, wantErr: "invalid --email"},
		{name: "user with domain", cmd: DriveShareCmd{To: driveShareToUser, Email: "x@example.com", Domain: "example.com"}, wantErr: "--to=user cannot be combined"},
		{name: "user discoverable", cmd: DriveShareCmd{To: driveShareToUser, Email: "x@example.com", Discoverable: true}, wantErr: "--discoverable is only valid"},
		{name: "domain without domain", cmd: DriveShareCmd{To: driveShareToDomain}, wantErr: "missing --domain"},
		{name: "domain invalid spaces", cmd: DriveShareCmd{To: driveShareToDomain, Domain: "not a domain"}, wantErr: "invalid --domain"},
		{name: "domain invalid url", cmd: DriveShareCmd{To: driveShareToDomain, Domain: "https://example.com"}, wantErr: "invalid --domain"},
		{name: "domain with email", cmd: DriveShareCmd{To: driveShareToDomain, Domain: "example.com", Email: "x@example.com"}, wantErr: "--to=domain cannot be combined"},
		{name: "invalid target", cmd: DriveShareCmd{To: "group"}, wantErr: "invalid --to"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cmd.normalizeTarget()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTarget: %v", err)
			}
			if got != tt.want {
				t.Fatalf("target = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDriveShare_InvalidTargetsFailBeforeDryRun(t *testing.T) {
	factory := func(context.Context, string) (*drive.Service, error) {
		t.Fatalf("expected validation to fail before creating drive service")
		return nil, errors.New("unexpected drive service call")
	}

	testCases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid email",
			args: []string{"--account", "a@b.com", "--dry-run", "drive", "share", "f1", "--to=user", "--email", "nope"},
			want: "invalid --email",
		},
		{
			name: "display name email",
			args: []string{"--account", "a@b.com", "--dry-run", "drive", "share", "f1", "--to=user", "--email", "Tester <x@example.com>"},
			want: "invalid --email",
		},
		{
			name: "invalid domain",
			args: []string{"--account", "a@b.com", "--dry-run", "drive", "share", "f1", "--to=domain", "--domain", "not a domain"},
			want: "invalid --domain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithDriveTestServiceFactory(t, tc.args, factory)
			var exitErr *ExitError
			if !errors.As(result.err, &exitErr) || exitErr.Code != 2 || !strings.Contains(result.err.Error(), tc.want) {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}

func TestDriveShare_CommenterRole(t *testing.T) {
	var sawCommenterRole bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/permissions"):
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode permission request: %v", err)
			}
			if req["role"] == drivePermRoleCommenter {
				sawCommenterRole = true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "perm1",
				"type":         "user",
				"role":         req["role"],
				"emailAddress": req["emailAddress"],
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/files/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "f1",
				"name":        "File",
				"webViewLink": "https://drive.example/f1",
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
		t.Fatalf("NewDriveService: %v", err)
	}

	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	flags := &RootFlags{Account: "a@b.com"}

	err = (&DriveShareCmd{
		FileID: "f1",
		To:     driveShareToUser,
		Email:  "x@y.com",
		Role:   drivePermRoleCommenter,
	}).Run(ctx, flags)
	if err != nil {
		t.Fatalf("DriveShareCmd.Run: %v", err)
	}
	if !sawCommenterRole {
		t.Fatalf("expected commenter role in permission create request")
	}
}

func TestDriveShare_Notify(t *testing.T) {
	var sawNotify bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/permissions"):
			sawNotify = r.URL.Query().Get("sendNotificationEmail") == "true"
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode permission request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "perm1",
				"type":         "user",
				"role":         req["role"],
				"emailAddress": req["emailAddress"],
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/files/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "f1",
				"name":        "File",
				"webViewLink": "https://drive.example/f1",
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
		t.Fatalf("NewDriveService: %v", err)
	}

	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	err = (&DriveShareCmd{
		FileID: "f1",
		To:     driveShareToUser,
		Email:  "x@y.com",
		Role:   drivePermRoleReader,
		Notify: true,
	}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("DriveShareCmd.Run: %v", err)
	}
	if !sawNotify {
		t.Fatalf("expected sendNotificationEmail=true")
	}
}

func TestDriveShare_DryRunSkipsPermissionCreate(t *testing.T) {
	var out bytes.Buffer
	ctx := withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), func(context.Context, string) (*drive.Service, error) {
		t.Fatal("Drive service should not be created during dry-run")
		return nil, errors.New("unexpected Drive service call")
	})
	err := (&DriveShareCmd{
		FileID: "f1",
		To:     driveShareToUser,
		Email:  "x@y.com",
		Role:   drivePermRoleReader,
		Notify: true,
	}).Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("DriveShareCmd.Run: %v", err)
	}
	if !strings.Contains(out.String(), `"sendNotificationEmail": true`) {
		t.Fatalf("unexpected dry-run output: %s", out.String())
	}
}

func TestDriveDownload_TextOutput(t *testing.T) {
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("data")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/drive/v3/files/") && !strings.HasPrefix(r.URL.Path, "/files/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "file1",
			"name":     "File",
			"mimeType": "text/plain",
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

	var outBuf bytes.Buffer
	ctx := withDriveTestOperations(newCmdRuntimeOutputContext(t, &outBuf, io.Discard), svc, download, nil)
	flags := &RootFlags{Account: "a@b.com"}

	dest := filepath.Join(t.TempDir(), "out.txt")
	cmd := &DriveDownloadCmd{FileID: "file1", Output: OutputPathFlag{Path: dest}}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(outBuf.String(), "path") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected file: %v", err)
	}
}

func TestDownloadDriveFile_ErrorPaths(t *testing.T) {
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return nil, errors.New("download boom")
	}
	export := func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return nil, errors.New("export boom")
	}

	ctx := withDriveTestOperations(context.Background(), &drive.Service{}, download, export)
	if _, _, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "x", MimeType: "text/plain"}, "out", "", false); err == nil {
		t.Fatalf("expected download error")
	}
	if _, _, err := downloadDriveFile(ctx, &drive.Service{}, &drive.File{Id: "x", MimeType: driveMimeGoogleDoc}, "out", "", false); err == nil {
		t.Fatalf("expected export error")
	}
}

func TestGoogleConvertMimeType(t *testing.T) {
	cases := []struct {
		path     string
		wantMime string
		wantOK   bool
	}{
		{"report.docx", driveMimeGoogleDoc, true},
		{"report.DOCX", driveMimeGoogleDoc, true},
		{"old.doc", driveMimeGoogleDoc, true},
		{"budget.xlsx", driveMimeGoogleSheet, true},
		{"budget.xls", driveMimeGoogleSheet, true},
		{"data.csv", driveMimeGoogleSheet, true},
		{"deck.pptx", driveMimeGoogleSlides, true},
		{"deck.ppt", driveMimeGoogleSlides, true},
		{"notes.txt", driveMimeGoogleDoc, true},
		{"notes.md", driveMimeGoogleDoc, true},
		{"page.html", driveMimeGoogleDoc, true},
		{"photo.png", "", false},
		{"archive.zip", "", false},
		{"binary.exe", "", false},
	}
	for _, tc := range cases {
		mime, ok := googleConvertMimeType(tc.path)
		if ok != tc.wantOK || mime != tc.wantMime {
			t.Errorf("googleConvertMimeType(%q) = (%q, %v), want (%q, %v)", tc.path, mime, ok, tc.wantMime, tc.wantOK)
		}
	}
}

func TestGoogleConvertTargetMimeType(t *testing.T) {
	cases := []struct {
		target   string
		wantMime string
		wantOK   bool
	}{
		{"doc", driveMimeGoogleDoc, true},
		{"sheet", driveMimeGoogleSheet, true},
		{"slides", driveMimeGoogleSlides, true},
		{"DOC", driveMimeGoogleDoc, true},
		{"unknown", "", false},
	}
	for _, tc := range cases {
		mime, ok := googleConvertTargetMimeType(tc.target)
		if ok != tc.wantOK || mime != tc.wantMime {
			t.Errorf("googleConvertTargetMimeType(%q) = (%q, %v), want (%q, %v)", tc.target, mime, ok, tc.wantMime, tc.wantOK)
		}
	}
}

func TestDriveUploadConvertMimeType(t *testing.T) {
	mimeType, convert, err := driveUploadConvertMimeType("report.docx", true, "")
	if err != nil {
		t.Fatalf("auto convert: %v", err)
	}
	if !convert || mimeType != driveMimeGoogleDoc {
		t.Fatalf("auto convert = (%q, %v), want (%q, true)", mimeType, convert, driveMimeGoogleDoc)
	}

	mimeType, convert, err = driveUploadConvertMimeType("notes.md", true, "")
	if err != nil {
		t.Fatalf("auto convert md: %v", err)
	}
	if !convert || mimeType != driveMimeGoogleDoc {
		t.Fatalf("auto convert md = (%q, %v), want (%q, true)", mimeType, convert, driveMimeGoogleDoc)
	}

	mimeType, convert, err = driveUploadConvertMimeType("photo.png", false, "sheet")
	if err != nil {
		t.Fatalf("explicit convert: %v", err)
	}
	if !convert || mimeType != driveMimeGoogleSheet {
		t.Fatalf("explicit convert = (%q, %v), want (%q, true)", mimeType, convert, driveMimeGoogleSheet)
	}

	mimeType, convert, err = driveUploadConvertMimeType("photo.png", false, "")
	if err != nil {
		t.Fatalf("no convert: %v", err)
	}
	if convert || mimeType != "" {
		t.Fatalf("no convert = (%q, %v), want empty/false", mimeType, convert)
	}

	if _, _, err = driveUploadConvertMimeType("photo.png", false, "not-a-target"); err == nil {
		t.Fatalf("expected error for invalid --convert-to target")
	} else if ExitCode(err) != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", ExitCode(err), err)
	}
}

func TestStripOfficeExt(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"report.docx", "report"},
		{"report.doc", "report"},
		{"budget.xlsx", "budget"},
		{"budget.xls", "budget"},
		{"deck.pptx", "deck"},
		{"deck.ppt", "deck"},
		{"notes.md", "notes"},
		{"notes.txt", "notes.txt"},
		{"photo.png", "photo.png"},
		{"no-ext", "no-ext"},
	}
	for _, tc := range cases {
		got := stripOfficeExt(tc.name)
		if got != tc.want {
			t.Errorf("stripOfficeExt(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDriveUpload_ConvertUnsupported(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	tmp := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(tmp, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	newServiceCalled := false
	ctx := withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*drive.Service, error) {
		newServiceCalled = true
		return &drive.Service{}, nil
	})

	cmd := &DriveUploadCmd{LocalPath: tmp, Convert: true}
	if err := cmd.Run(ctx, flags); err == nil {
		t.Fatalf("expected error for unsupported --convert type")
	} else if !strings.Contains(err.Error(), "--convert: unsupported") {
		t.Fatalf("unexpected error: %v", err)
	} else if ExitCode(err) != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", ExitCode(err), err)
	}
	if newServiceCalled {
		t.Fatalf("Drive service should not be created when --convert validation fails")
	}
}

func TestDriveWebLink_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
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
	if _, err := driveWebLink(context.Background(), svc, "file1"); err == nil {
		t.Fatalf("expected error")
	}
}
