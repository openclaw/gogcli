package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDriveAuditSharingFindsPublicAndExternal(t *testing.T) {
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{{
					"id":       "file1",
					"name":     "Shared Doc",
					"mimeType": "text/plain",
					"owners":   []map[string]any{{"emailAddress": "owner@example.com"}},
				}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "anyone", "type": "anyone", "role": "reader"},
					{"id": "user1", "type": "user", "role": "writer", "emailAddress": "a@external.test"},
					{"id": "user2", "type": "user", "role": "reader", "emailAddress": "b@example.com"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeSvc()

	var stdout bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), svc)
	if err := (&DriveAuditSharingCmd{Parent: "root", Depth: 1, Max: 10}).Run(ctx, &RootFlags{Account: "owner@example.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var parsed struct {
		FindingCount int `json:"findingCount"`
		Findings     []struct {
			PermissionID string   `json:"permissionId"`
			Reasons      []string `json:"reasons"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v\n%s", err, stdout.String())
	}
	if parsed.FindingCount != 2 {
		t.Fatalf("finding count = %d, want 2: %#v", parsed.FindingCount, parsed.Findings)
	}
	if parsed.Findings[0].PermissionID != "anyone" || parsed.Findings[1].PermissionID != "user1" {
		t.Fatalf("unexpected findings: %#v", parsed.Findings)
	}
}

func TestListDrivePermissionsForAuditRejectsRepeatedPageToken(t *testing.T) {
	t.Parallel()

	var listCalls atomic.Int32
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/file1/permissions") {
			http.NotFound(w, r)
			return
		}
		call := listCalls.Add(1)
		if call > 2 {
			http.Error(w, "unexpected extra page request", http.StatusBadRequest)
			return
		}
		requireSupportsAllDrives(t, r)
		if call == 1 {
			requireQuery(t, r, "pageToken", "")
		} else {
			requireQuery(t, r, "pageToken", "stuck")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"permissions": []map[string]any{
				{"id": "anyone", "type": "anyone", "role": "reader"},
			},
			"nextPageToken": "stuck",
		})
	}))
	defer closeSvc()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	permissions, err := listDrivePermissionsForAudit(ctx, svc, "file1")
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, listCalls.Load())
	}
	if permissions != nil {
		t.Fatalf("permissions = %#v, want nil on incomplete listing", permissions)
	}
	if got := listCalls.Load(); got != 2 {
		t.Fatalf("list calls = %d, want 2", got)
	}
	t.Logf("err = %v after %d list calls", err, listCalls.Load())
}

func TestListDrivePermissionsForAuditPagination(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		failSecondPage bool
	}{
		{name: "success preserves page order"},
		{name: "later error discards partial permissions", failSecondPage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var listCalls atomic.Int32
			svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/files/file1/permissions") {
					http.NotFound(w, r)
					return
				}
				requireSupportsAllDrives(t, r)
				requireQuery(t, r, "pageSize", "100")
				requireQuery(t, r, "fields", "nextPageToken,permissions(id,type,role,emailAddress,domain,displayName,allowFileDiscovery,deleted,expirationTime,permissionDetails(permissionType,role,inherited,inheritedFrom))")
				call := listCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch call {
				case 1:
					requireQuery(t, r, "pageToken", "")
					_, _ = io.WriteString(w, `{"permissions":[{"id":"first","type":"anyone","role":"reader"}],"nextPageToken":"page-2"}`)
				case 2:
					requireQuery(t, r, "pageToken", "page-2")
					if tc.failSecondPage {
						http.Error(w, `{"error":{"code":403,"message":"page two denied"}}`, http.StatusForbidden)
						return
					}
					_, _ = io.WriteString(w, `{"permissions":[{"id":"second","type":"user","role":"writer"}]}`)
				default:
					http.Error(w, "unexpected extra page request", http.StatusBadRequest)
				}
			}))
			defer closeSvc()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			permissions, err := listDrivePermissionsForAudit(ctx, svc, "file1")
			if tc.failSecondPage {
				if err == nil || !strings.Contains(err.Error(), "page two denied") || permissions != nil {
					t.Fatalf("permissions = %#v, err = %v; want nil permissions and provider error", permissions, err)
				}
			} else if err != nil || len(permissions) != 2 || permissions[0].Id != "first" || permissions[1].Id != "second" {
				t.Fatalf("permissions = %#v, err = %v; want both pages in order", permissions, err)
			}
			if got := listCalls.Load(); got != 2 {
				t.Fatalf("list calls = %d, want 2", got)
			}
		})
	}
}

func TestExecuteDrivePermissionScanFailurePreventsOutputAndWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "sharing audit", args: []string{"drive", "audit", "sharing", "--internal-domain", "example.com", "--fail-found"}},
		{name: "user audit", args: []string{"drive", "audit", "user", "a@external.test", "--fail-found"}},
		{name: "remove public", args: []string{"drive", "bulk", "remove-public", "--force"}},
		{name: "update role", args: []string{"drive", "bulk", "update-role", "--force", "--from", "writer", "--to", "reader", "--type", "user", "--target", "a@external.test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var filesCalls, firstPermissionCalls, secondPermissionCalls, mutations atomic.Int32
			svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations.Add(1)
					http.Error(w, "unexpected permission mutation", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/files"):
					filesCalls.Add(1)
					if !strings.Contains(r.URL.Query().Get("q"), "'root' in parents") {
						t.Errorf("unexpected file query: %s", r.URL.RawQuery)
					}
					_, _ = io.WriteString(w, `{"files":[{"id":"file1","name":"01-ready","mimeType":"text/plain"},{"id":"file2","name":"02-loop","mimeType":"text/plain"}]}`)
				case strings.HasSuffix(r.URL.Path, "/files/file1/permissions"):
					firstPermissionCalls.Add(1)
					_, _ = io.WriteString(w, `{"permissions":[{"id":"public1","type":"anyone","role":"reader"},{"id":"writer1","type":"user","role":"writer","emailAddress":"a@external.test"}]}`)
				case strings.HasSuffix(r.URL.Path, "/files/file2/permissions"):
					if firstPermissionCalls.Load() != 1 {
						t.Error("later-file failure must follow an earlier actionable permission page")
					}
					call := secondPermissionCalls.Add(1)
					if call > 2 {
						http.Error(w, "unexpected extra page request", http.StatusBadRequest)
						return
					}
					requireSupportsAllDrives(t, r)
					if call == 1 {
						requireQuery(t, r, "pageToken", "")
					} else {
						requireQuery(t, r, "pageToken", "stuck")
					}
					_, _ = io.WriteString(w, `{"permissions":[{"id":"loop","type":"anyone","role":"reader"}],"nextPageToken":"stuck"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer closeSvc()

			args := append([]string{"--json", "--account", "owner@example.com", "--no-input"}, tc.args...)
			args = append(args, "--parent", "root", "--depth", "1", "--max", "10")
			result := executeWithDriveTestService(t, args, svc)
			wantError := `list permissions for file2: pagination loop: repeated page token "stuck"`
			if result.err == nil || !strings.Contains(result.err.Error(), wantError) || ExitCode(result.err) != 1 {
				t.Fatalf("err = %v, exit = %d, stderr = %q; want file-specific scan error", result.err, ExitCode(result.err), result.stderr)
			}
			if result.stdout != "" || !strings.Contains(result.stderr, wantError) {
				t.Fatalf("stdout = %q, stderr = %q; want only the scan error", result.stdout, result.stderr)
			}
			if filesCalls.Load() != 1 || firstPermissionCalls.Load() != 1 || secondPermissionCalls.Load() != 2 || mutations.Load() != 0 {
				t.Fatalf("files = %d, first permissions = %d, second permissions = %d, mutations = %d", filesCalls.Load(), firstPermissionCalls.Load(), secondPermissionCalls.Load(), mutations.Load())
			}
			t.Logf("%v; no success output or permission writes", result.err)
		})
	}
}
