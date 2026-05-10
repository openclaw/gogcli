package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"
)

func TestRequireAdminAccount_ConsumerBlocked(t *testing.T) {
	account, err := requireAdminAccount(&RootFlags{Account: "user@gmail.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if account != "" {
		t.Fatalf("expected empty account, got %q", account)
	}
	if !strings.Contains(err.Error(), "Google Workspace account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapAdminDirectoryError_MapsPermissions(t *testing.T) {
	err := wrapAdminDirectoryError(errors.New("insufficient authentication scopes"), "svc@example.com")
	if err == nil || !strings.Contains(err.Error(), "admin.directory.group.member") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapAdminOrgUnitDirectoryError_MapsPermissions(t *testing.T) {
	err := wrapAdminOrgUnitDirectoryError(errors.New("insufficient authentication scopes"), "svc@example.com")
	if err == nil || !strings.Contains(err.Error(), "admin.directory.orgunit scope") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "admin.directory.group.member") {
		t.Fatalf("unexpected group scope in error: %v", err)
	}
}

func TestAdminUsersCreate_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	flags := &RootFlags{Account: "svc@example.com"}

	tests := []struct {
		name string
		cmd  AdminUsersCreateCmd
		want string
	}{
		{name: "missing email", cmd: AdminUsersCreateCmd{GivenName: "Ada", FamilyName: "Lovelace", Password: "pw"}, want: "email required"},
		{name: "missing given", cmd: AdminUsersCreateCmd{Email: "ada@example.com", FamilyName: "Lovelace", Password: "pw"}, want: "--given required"},
		{name: "missing family", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", Password: "pw"}, want: "--family required"},
		{name: "hash without password", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", FamilyName: "Lovelace", HashFunction: "sha1"}, want: "--password required when --hash-function is set"},
		{name: "bad hash", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", FamilyName: "Lovelace", Password: "pw", HashFunction: "bcrypt"}, want: "invalid --hash-function"},
		{name: "admin unsupported", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", FamilyName: "Lovelace", Password: "pw", Admin: true}, want: "--admin is not supported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAdminUsersCreate_JSONSendsWorkspaceUser(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	var gotInsert admin.User
	var gotPatch admin.User
	patchCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users"):
			if err := json.NewDecoder(r.Body).Decode(&gotInsert); err != nil {
				t.Fatalf("decode insert body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"primaryEmail": gotInsert.PrimaryEmail,
				"id":           "user-123",
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/users/ada@example.com"):
			patchCalls++
			if patchCalls == 1 {
				http.Error(w, `{"error":{"code":404,"message":"Resource Not Found: userKey"}}`, http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&gotPatch); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"primaryEmail": "ada@example.com",
				"id":           "user-123",
				"suspended":    gotPatch.Suspended,
				"archived":     gotPatch.Archived,
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)
	out := captureStdout(t, func() {
		err := (&AdminUsersCreateCmd{
			Email:         "ada@example.com",
			GivenName:     "Ada",
			FamilyName:    "Lovelace",
			Password:      "hashed-pw",
			ChangePwd:     true,
			OrgUnit:       "/Engineering",
			Suspended:     true,
			Archived:      true,
			RecoveryEmail: "ada.recovery@example.net",
			RecoveryPhone: "+15551234567",
			HashFunction:  "sha1",
		}).Run(ctx, &RootFlags{Account: "svc@example.com"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotInsert.PrimaryEmail != "ada@example.com" || gotInsert.Name == nil || gotInsert.Name.GivenName != "Ada" || gotInsert.Name.FamilyName != "Lovelace" {
		t.Fatalf("unexpected user identity: %#v", gotInsert)
	}
	if gotInsert.Password != "hashed-pw" || gotInsert.HashFunction != "SHA-1" {
		t.Fatalf("unexpected password fields: password=%q hash=%q", gotInsert.Password, gotInsert.HashFunction)
	}
	if !gotInsert.ChangePasswordAtNextLogin {
		t.Fatalf("expected change password in insert request: %#v", gotInsert)
	}
	if !gotPatch.Suspended || !gotPatch.Archived {
		t.Fatalf("expected post-create state patch: %#v", gotPatch)
	}
	if patchCalls != 2 {
		t.Fatalf("expected retry after transient patch 404, got %d calls", patchCalls)
	}
	if gotInsert.OrgUnitPath != "/Engineering" || gotInsert.RecoveryEmail != "ada.recovery@example.net" || gotInsert.RecoveryPhone != "+15551234567" {
		t.Fatalf("unexpected profile fields: %#v", gotInsert)
	}

	var parsed struct {
		Email     string `json:"email"`
		ID        string `json:"id"`
		Suspended bool   `json:"suspended"`
		Archived  bool   `json:"archived"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Email != "ada@example.com" || parsed.ID != "user-123" || !parsed.Suspended || !parsed.Archived {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}

func TestAdminUsersCreate_GeneratesPasswordWhenOmitted(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	var got admin.User
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users")) {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"primaryEmail": got.PrimaryEmail,
			"id":           "user-456",
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)
	out := captureStdout(t, func() {
		err := (&AdminUsersCreateCmd{
			Email:      "grace@example.com",
			GivenName:  "Grace",
			FamilyName: "Hopper",
		}).Run(ctx, &RootFlags{Account: "svc@example.com"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if got.Password == "" || len(got.Password) < 8 {
		t.Fatalf("expected generated password, got %q", got.Password)
	}
	if !got.ChangePasswordAtNextLogin {
		t.Fatalf("generated password should force password change")
	}

	var parsed struct {
		Email             string `json:"email"`
		ID                string `json:"id"`
		GeneratedPassword string `json:"generatedPassword"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Email != "grace@example.com" || parsed.ID != "user-456" || parsed.GeneratedPassword != got.Password {
		t.Fatalf("unexpected response: %#v request password %q", parsed, got.Password)
	}
}

func TestAdminUsersDelete_JSONRequiresForceAndDeletes(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/")) {
			http.NotFound(w, r)
			return
		}
		deletedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)
	out := captureStdout(t, func() {
		err := (&AdminUsersDeleteCmd{UserEmail: "temp@example.com"}).Run(ctx, &RootFlags{
			Account: "svc@example.com",
			Force:   true,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(deletedPath, "/users/temp@example.com") {
		t.Fatalf("unexpected delete path: %q", deletedPath)
	}
	var parsed struct {
		Email   string `json:"email"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Email != "temp@example.com" || !parsed.Deleted {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}

func TestAdminOrgunitsCreateUpdateDelete_JSON(t *testing.T) {
	origNew := newAdminOrgUnitDirectoryService
	t.Cleanup(func() { newAdminOrgUnitDirectoryService = origNew })

	var inserted admin.OrgUnit
	var patched admin.OrgUnit
	var patchBody string
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/orgunits"):
			if err := json.NewDecoder(r.Body).Decode(&inserted); err != nil {
				t.Fatalf("decode insert body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":              inserted.Name,
				"orgUnitPath":       "/Engineering",
				"orgUnitId":         "ou-123",
				"parentOrgUnitPath": inserted.ParentOrgUnitPath,
				"description":       inserted.Description,
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/orgunits/Engineering"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read patch body: %v", err)
			}
			patchBody = string(body)
			if err := json.NewDecoder(bytes.NewReader(body)).Decode(&patched); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        patched.Name,
				"orgUnitPath": "/Engineering",
				"orgUnitId":   "ou-123",
				"description": patched.Description,
			})
			return
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/orgunits/Engineering"):
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminOrgUnitDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)
	createOut := captureStdout(t, func() {
		err := (&AdminOrgunitsCreateCmd{
			Name:        "Engineering",
			Parent:      "/",
			Description: "Builders",
		}).Run(ctx, &RootFlags{Account: "svc@example.com"})
		if err != nil {
			t.Fatalf("create Run: %v", err)
		}
	})
	if inserted.Name != "Engineering" || inserted.ParentOrgUnitPath != "/" || inserted.Description != "Builders" {
		t.Fatalf("unexpected insert body: %#v", inserted)
	}
	var created admin.OrgUnit
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created.OrgUnitPath != "/Engineering" || created.OrgUnitId != "ou-123" {
		t.Fatalf("unexpected create response: %#v", created)
	}

	rename := "Eng"
	description := ""
	updateOut := captureStdout(t, func() {
		err := (&AdminOrgunitsUpdateCmd{
			Path:        "/Engineering",
			Name:        &rename,
			Description: &description,
		}).Run(ctx, &RootFlags{Account: "svc@example.com"})
		if err != nil {
			t.Fatalf("update Run: %v", err)
		}
	})
	if patched.Name != "Eng" || patched.Description != "" || !strings.Contains(patchBody, `"description":""`) {
		t.Fatalf("unexpected patch body: %#v", patched)
	}
	var updated admin.OrgUnit
	if err := json.Unmarshal([]byte(updateOut), &updated); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if updated.Name != "Eng" || updated.OrgUnitPath != "/Engineering" {
		t.Fatalf("unexpected update response: %#v", updated)
	}

	deleteOut := captureStdout(t, func() {
		err := (&AdminOrgunitsDeleteCmd{Path: "/Engineering"}).Run(ctx, &RootFlags{
			Account: "svc@example.com",
			Force:   true,
		})
		if err != nil {
			t.Fatalf("delete Run: %v", err)
		}
	})
	if !deleted {
		t.Fatalf("expected delete request")
	}
	var deleteResult struct {
		Path    string `json:"path"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(deleteOut), &deleteResult); err != nil {
		t.Fatalf("unmarshal delete: %v", err)
	}
	if deleteResult.Path != "Engineering" || !deleteResult.Deleted {
		t.Fatalf("unexpected delete output: %#v", deleteResult)
	}
}

func TestAdminOrgunitsList_JSON(t *testing.T) {
	origNew := newAdminOrgUnitDirectoryService
	t.Cleanup(func() { newAdminOrgUnitDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/orgunits")) {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("orgUnitPath"); got != "/" {
			t.Fatalf("orgUnitPath query = %q, want /", got)
		}
		if got := r.URL.Query().Get("type"); got != "all" {
			t.Fatalf("type query = %q, want all", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "Engineering", "orgUnitPath": "/Engineering", "orgUnitId": "ou-123"},
			},
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminOrgUnitDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)
	out := captureStdout(t, func() {
		if err := (&AdminOrgunitsListCmd{Parent: "/", Type: "all"}).Run(ctx, &RootFlags{Account: "svc@example.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "Engineering") || !strings.Contains(out, "/Engineering") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAdminUsersList_JSON_AllowsNilName(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]any{
				{
					"primaryEmail": "ada@example.com",
					"suspended":    false,
					"isAdmin":      true,
				},
			},
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)

	out := captureStdout(t, func() {
		if err := (&AdminUsersListCmd{Domain: "example.com"}).Run(ctx, &RootFlags{Account: "svc@example.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Users []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
			Admin bool   `json:"admin"`
		} `json:"users"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Users) != 1 || parsed.Users[0].Email != "ada@example.com" || parsed.Users[0].Name != "" || !parsed.Users[0].Admin {
		t.Fatalf("unexpected users: %#v", parsed.Users)
	}
}

func TestAdminGroupsMembersAdd_JSON(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	var gotRole string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/members")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRole, _ = body["role"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "dev@example.com",
			"role":  gotRole,
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)

	out := captureStdout(t, func() {
		if err := (&AdminGroupsMembersAddCmd{
			GroupEmail:  "eng@example.com",
			MemberEmail: "dev@example.com",
			Role:        "owner",
		}).Run(ctx, &RootFlags{Account: "svc@example.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotRole != adminRoleOwner {
		t.Fatalf("unexpected role sent: %q", gotRole)
	}
	var parsed struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Email != "dev@example.com" || parsed.Role != adminRoleOwner {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}
