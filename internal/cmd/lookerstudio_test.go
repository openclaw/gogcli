package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLookerStudioPermissionsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/asset1/permissions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"permissions": []map[string]any{
				{"id": "perm1", "type": "user", "role": "reader", "emailAddress": "viewer@example.com"},
			},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &LookerStudioPermissionsListCmd{AssetID: "asset1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "viewer@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLookerStudioPermissionsAddCmd(t *testing.T) {
	var gotRole string
	var gotEmail string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/files/asset1/permissions") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Role  string `json:"role"`
			Type  string `json:"type"`
			Email string `json:"emailAddress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotRole = payload.Role
		gotEmail = payload.Email
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1", "role": payload.Role, "emailAddress": payload.Email})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &LookerStudioPermissionsAddCmd{AssetID: "asset1", Email: "editor@example.com", Role: "EDITOR"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotRole != "writer" || gotEmail != "editor@example.com" {
		t.Fatalf("unexpected permission payload: role=%q email=%q", gotRole, gotEmail)
	}
	if !strings.Contains(out, "Added permission") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLookerStudioPermissionsRemoveCmd(t *testing.T) {
	var deleted bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/files/asset1/permissions/perm1") {
			http.NotFound(w, r)
			return
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &LookerStudioPermissionsRemoveCmd{AssetID: "asset1", PermissionID: "perm1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatalf("expected delete request")
	}
	if !strings.Contains(out, "Removed permission") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLookerStudioPermissionsAddCmd_BadRole(t *testing.T) {
	cmd := &LookerStudioPermissionsAddCmd{AssetID: "asset1", Email: "user@example.com", Role: "OWNER"}
	if err := cmd.Run(context.Background(), &RootFlags{Account: "user@example.com"}); err == nil {
		t.Fatalf("expected error")
	}
}
