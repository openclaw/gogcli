package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/driveactivity/v2"

	"github.com/steipete/gogcli/internal/outfmt"
)

// -----------------------------------------------------------------------------
// activityAction tests
// -----------------------------------------------------------------------------

func TestActivityAction_Nil(t *testing.T) {
	if got := activityAction(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestActivityAction_Create(t *testing.T) {
	detail := &driveactivity.ActionDetail{Create: &driveactivity.Create{}}
	if got := activityAction(detail); got != "create" {
		t.Fatalf("expected create, got %q", got)
	}
}

func TestActivityAction_Edit(t *testing.T) {
	detail := &driveactivity.ActionDetail{Edit: &driveactivity.Edit{}}
	if got := activityAction(detail); got != "edit" {
		t.Fatalf("expected edit, got %q", got)
	}
}

func TestActivityAction_Move(t *testing.T) {
	detail := &driveactivity.ActionDetail{Move: &driveactivity.Move{}}
	if got := activityAction(detail); got != "move" {
		t.Fatalf("expected move, got %q", got)
	}
}

func TestActivityAction_Rename(t *testing.T) {
	detail := &driveactivity.ActionDetail{Rename: &driveactivity.Rename{}}
	if got := activityAction(detail); got != "rename" {
		t.Fatalf("expected rename, got %q", got)
	}
}

func TestActivityAction_Delete(t *testing.T) {
	detail := &driveactivity.ActionDetail{Delete: &driveactivity.Delete{}}
	if got := activityAction(detail); got != "delete" {
		t.Fatalf("expected delete, got %q", got)
	}
}

func TestActivityAction_Restore(t *testing.T) {
	detail := &driveactivity.ActionDetail{Restore: &driveactivity.Restore{}}
	if got := activityAction(detail); got != "restore" {
		t.Fatalf("expected restore, got %q", got)
	}
}

func TestActivityAction_Permission(t *testing.T) {
	detail := &driveactivity.ActionDetail{PermissionChange: &driveactivity.PermissionChange{}}
	if got := activityAction(detail); got != "permission" {
		t.Fatalf("expected permission, got %q", got)
	}
}

func TestActivityAction_Comment(t *testing.T) {
	detail := &driveactivity.ActionDetail{Comment: &driveactivity.Comment{}}
	if got := activityAction(detail); got != "comment" {
		t.Fatalf("expected comment, got %q", got)
	}
}

func TestActivityAction_Dlp(t *testing.T) {
	detail := &driveactivity.ActionDetail{DlpChange: &driveactivity.DataLeakPreventionChange{}}
	if got := activityAction(detail); got != "dlp" {
		t.Fatalf("expected dlp, got %q", got)
	}
}

func TestActivityAction_Settings(t *testing.T) {
	detail := &driveactivity.ActionDetail{SettingsChange: &driveactivity.SettingsChange{}}
	if got := activityAction(detail); got != "settings" {
		t.Fatalf("expected settings, got %q", got)
	}
}

func TestActivityAction_Reference(t *testing.T) {
	detail := &driveactivity.ActionDetail{Reference: &driveactivity.ApplicationReference{}}
	if got := activityAction(detail); got != "reference" {
		t.Fatalf("expected reference, got %q", got)
	}
}

func TestActivityAction_Other(t *testing.T) {
	// Empty detail with no action type set
	detail := &driveactivity.ActionDetail{}
	if got := activityAction(detail); got != "other" {
		t.Fatalf("expected other, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// activityActor tests
// -----------------------------------------------------------------------------

func TestActivityActor_Empty(t *testing.T) {
	if got := activityActor(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := activityActor([]*driveactivity.Actor{}); got != "" {
		t.Fatalf("expected empty for empty slice, got %q", got)
	}
	if got := activityActor([]*driveactivity.Actor{nil}); got != "" {
		t.Fatalf("expected empty for nil actor, got %q", got)
	}
}

func TestActivityActor_KnownUserWithName(t *testing.T) {
	actors := []*driveactivity.Actor{
		{User: &driveactivity.User{KnownUser: &driveactivity.KnownUser{PersonName: "John Doe"}}},
	}
	if got := activityActor(actors); got != "John Doe" {
		t.Fatalf("expected John Doe, got %q", got)
	}
}

func TestActivityActor_KnownUserCurrentUser(t *testing.T) {
	actors := []*driveactivity.Actor{
		{User: &driveactivity.User{KnownUser: &driveactivity.KnownUser{IsCurrentUser: true}}},
	}
	if got := activityActor(actors); got != "me" {
		t.Fatalf("expected me, got %q", got)
	}
}

func TestActivityActor_Administrator(t *testing.T) {
	actors := []*driveactivity.Actor{
		{Administrator: &driveactivity.Administrator{}},
	}
	if got := activityActor(actors); got != "admin" {
		t.Fatalf("expected admin, got %q", got)
	}
}

func TestActivityActor_System(t *testing.T) {
	actors := []*driveactivity.Actor{
		{System: &driveactivity.SystemEvent{}},
	}
	if got := activityActor(actors); got != "system" {
		t.Fatalf("expected system, got %q", got)
	}
}

func TestActivityActor_Anonymous(t *testing.T) {
	actors := []*driveactivity.Actor{
		{Anonymous: &driveactivity.AnonymousUser{}},
	}
	if got := activityActor(actors); got != "anonymous" {
		t.Fatalf("expected anonymous, got %q", got)
	}
}

func TestActivityActor_Unknown(t *testing.T) {
	// Actor with no known type
	actors := []*driveactivity.Actor{
		{},
	}
	if got := activityActor(actors); got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestActivityActor_UserWithNoKnownUser(t *testing.T) {
	// User set but KnownUser is nil - falls through to unknown
	actors := []*driveactivity.Actor{
		{User: &driveactivity.User{}},
	}
	if got := activityActor(actors); got != "unknown" {
		t.Fatalf("expected unknown for User with no KnownUser, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// DriveActivityCmd tests
// -----------------------------------------------------------------------------

func TestDriveActivityCmd_ValidationEmptyFileID(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for empty file-id")
	}
	if !strings.Contains(err.Error(), "file-id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveActivityCmd_ValidationNoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &DriveActivityCmd{FileID: "file1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing account")
	}
}

func TestDriveActivityCmd_TableOutput(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				{
					"timestamp": "2026-01-02T10:30:00Z",
					"primaryActionDetail": map[string]any{
						"create": map[string]any{},
					},
					"actors": []map[string]any{
						{"user": map[string]any{"knownUser": map[string]any{"personName": "Alice Smith"}}},
					},
				},
				{
					"timestamp": "2026-01-03T14:00:00Z",
					"primaryActionDetail": map[string]any{
						"edit": map[string]any{},
					},
					"actors": []map[string]any{
						{"administrator": map[string]any{}},
					},
				},
			},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "TIME") || !strings.Contains(out, "ACTOR") || !strings.Contains(out, "ACTION") {
		t.Fatalf("missing table headers: %s", out)
	}
	if !strings.Contains(out, "create") {
		t.Fatalf("missing create action: %s", out)
	}
	if !strings.Contains(out, "edit") {
		t.Fatalf("missing edit action: %s", out)
	}
	if !strings.Contains(out, "Alice Smith") {
		t.Fatalf("missing actor name: %s", out)
	}
	if !strings.Contains(out, "admin") {
		t.Fatalf("missing admin actor: %s", out)
	}
}

func TestDriveActivityCmd_JSONOutput(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				{
					"timestamp": "2026-01-02T10:30:00Z",
					"primaryActionDetail": map[string]any{
						"delete": map[string]any{},
					},
					"actors": []map[string]any{
						{"system": map[string]any{}},
					},
				},
			},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "activities") {
		t.Fatalf("expected JSON activities output: %s", out)
	}
	if !strings.Contains(out, "delete") {
		t.Fatalf("expected delete in JSON: %s", out)
	}
}

func TestDriveActivityCmd_NoActivities(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	// No activities should print to stderr
	err := cmd.Run(testContext(t), flags)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDriveActivityCmd_TimeRange(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				{
					// No direct timestamp, use timeRange instead
					"timeRange": map[string]any{
						"startTime": "2026-01-01T00:00:00Z",
						"endTime":   "2026-01-02T00:00:00Z",
					},
					"primaryActionDetail": map[string]any{
						"move": map[string]any{},
					},
					"actors": []map[string]any{
						{"anonymous": map[string]any{}},
					},
				},
			},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "2026-01-02") {
		t.Fatalf("expected timeRange endTime in output: %s", out)
	}
	if !strings.Contains(out, "move") {
		t.Fatalf("expected move action: %s", out)
	}
	if !strings.Contains(out, "anonymous") {
		t.Fatalf("expected anonymous actor: %s", out)
	}
}

func TestDriveActivityCmd_NilActivity(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Return activities with one nil entry (API doesn't usually do this, but testing code path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				nil,
				{
					"timestamp": "2026-01-02T10:30:00Z",
					"primaryActionDetail": map[string]any{
						"rename": map[string]any{},
					},
					"actors": []map[string]any{
						{"user": map[string]any{"knownUser": map[string]any{"isCurrentUser": true}}},
					},
				},
			},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "rename") {
		t.Fatalf("expected rename action: %s", out)
	}
	if !strings.Contains(out, "me") {
		t.Fatalf("expected 'me' as current user: %s", out)
	}
}

func TestDriveActivityCmd_Pagination(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				{
					"timestamp": "2026-01-02T10:30:00Z",
					"primaryActionDetail": map[string]any{
						"permissionChange": map[string]any{},
					},
					"actors": []map[string]any{
						{"user": map[string]any{"knownUser": map[string]any{"personName": "Bob"}}},
					},
				},
			},
			"nextPageToken": "next-page-token-123",
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1", Max: 10}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "permission") {
		t.Fatalf("expected permission action: %s", out)
	}
}

func TestDriveActivityCmd_AllActionTypes(t *testing.T) {
	// Test all action types in a single response
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/activity:query") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"activities": []map[string]any{
				{"timestamp": "2026-01-01T00:00:00Z", "primaryActionDetail": map[string]any{"restore": map[string]any{}}, "actors": []map[string]any{{}}},
				{"timestamp": "2026-01-02T00:00:00Z", "primaryActionDetail": map[string]any{"comment": map[string]any{}}, "actors": []map[string]any{{}}},
				{"timestamp": "2026-01-03T00:00:00Z", "primaryActionDetail": map[string]any{"dlpChange": map[string]any{}}, "actors": []map[string]any{{}}},
				{"timestamp": "2026-01-04T00:00:00Z", "primaryActionDetail": map[string]any{"settingsChange": map[string]any{}}, "actors": []map[string]any{{}}},
				{"timestamp": "2026-01-05T00:00:00Z", "primaryActionDetail": map[string]any{"reference": map[string]any{}}, "actors": []map[string]any{{}}},
				{"timestamp": "2026-01-06T00:00:00Z", "primaryActionDetail": map[string]any{}, "actors": []map[string]any{{}}},
			},
		})
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, expected := range []string{"restore", "comment", "dlp", "settings", "reference", "other"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("missing %s action: %s", expected, out)
		}
	}
}

func TestDriveActivityCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "forbidden"}}`, http.StatusForbidden)
	})
	stubDriveActivity(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &DriveActivityCmd{FileID: "file1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "query activity") {
		t.Fatalf("unexpected error: %v", err)
	}
}
