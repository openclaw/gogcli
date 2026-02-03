package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestOrgunitsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "Sales", "orgUnitPath": "/Sales", "orgUnitId": "ou-1"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Sales") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestOrgunitsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "Engineering", "orgUnitPath": "/Engineering", "orgUnitId": "ou-2", "description": "Engineering team"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "organizationUnits") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
	if !strings.Contains(out, "Engineering") {
		t.Fatalf("expected Engineering in output, got: %s", out)
	}
}

func TestOrgunitsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{}

	// Should not error on empty results
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestOrgunitsListCmd_WithParent(t *testing.T) {
	var capturedPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		capturedPath = r.URL.Query().Get("orgUnitPath")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "West", "orgUnitPath": "/Sales/West", "orgUnitId": "ou-3"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{Parent: "/Sales"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if capturedPath != "/Sales" {
		t.Errorf("expected parent path /Sales, got %s", capturedPath)
	}
	if !strings.Contains(out, "West") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestOrgunitsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":              "Sales",
			"orgUnitPath":       "/Sales",
			"orgUnitId":         "ou-1",
			"parentOrgUnitPath": "/",
			"parentOrgUnitId":   "root-ou",
			"description":       "Sales department",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsGetCmd{Path: "/Sales"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Name:") || !strings.Contains(out, "Sales") {
		t.Fatalf("expected Name: Sales in output, got: %s", out)
	}
	if !strings.Contains(out, "Path:") || !strings.Contains(out, "/Sales") {
		t.Fatalf("expected Path: /Sales in output, got: %s", out)
	}
	if !strings.Contains(out, "Description:") || !strings.Contains(out, "Sales department") {
		t.Fatalf("expected Description in output, got: %s", out)
	}
}

func TestOrgunitsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "Engineering",
			"orgUnitPath": "/Engineering",
			"orgUnitId":   "ou-2",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsGetCmd{Path: "/Engineering"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"orgUnitPath"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestOrgunitsGetCmd_MinimalFields(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Response with minimal fields - no parent, no description
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "Simple",
			"orgUnitPath": "/Simple",
			"orgUnitId":   "ou-simple",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsGetCmd{Path: "/Simple"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Simple") {
		t.Fatalf("unexpected output: %s", out)
	}
	// Should NOT contain description if empty
	if strings.Contains(out, "Description:") {
		t.Errorf("should not show empty description, got: %s", out)
	}
}

func TestOrgunitsGetCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsGetCmd{Path: "/NonExistent"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for non-existent org unit")
	}
	if !strings.Contains(err.Error(), "get org unit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrgunitsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "NewUnit",
			"orgUnitPath": "/NewUnit",
			"orgUnitId":   "ou-new",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "NewUnit"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created org unit") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "NewUnit") {
		t.Fatalf("expected NewUnit in output, got: %s", out)
	}
}

func TestOrgunitsCreateCmd_WithParent(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":              "SubUnit",
			"orgUnitPath":       "/Sales/SubUnit",
			"orgUnitId":         "ou-sub",
			"parentOrgUnitPath": "/Sales",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "SubUnit", Parent: "/Sales"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if capturedBody["parentOrgUnitPath"] != "/Sales" {
		t.Errorf("expected parent /Sales, got %v", capturedBody["parentOrgUnitPath"])
	}
	if !strings.Contains(out, "SubUnit") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestOrgunitsCreateCmd_WithDescription(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "Marketing",
			"orgUnitPath": "/Marketing",
			"orgUnitId":   "ou-mkt",
			"description": "Marketing department",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "Marketing", Description: "Marketing department"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if capturedBody["description"] != "Marketing department" {
		t.Errorf("expected description to be passed, got %v", capturedBody["description"])
	}
	if !strings.Contains(out, "Marketing") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestOrgunitsCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "JSONUnit",
			"orgUnitPath": "/JSONUnit",
			"orgUnitId":   "ou-json",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "JSONUnit"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"orgUnitPath"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestOrgunitsCreateCmd_DefaultParent(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "RootChild",
			"orgUnitPath": "/RootChild",
			"orgUnitId":   "ou-root-child",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "RootChild", Parent: ""} // Empty parent

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should default to root "/"
	if capturedBody["parentOrgUnitPath"] != "/" {
		t.Errorf("expected default parent /, got %v", capturedBody["parentOrgUnitPath"])
	}
}

func TestOrgunitsCreateCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "already exists", http.StatusConflict)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsCreateCmd{Name: "Existing"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for conflict")
	}
	if !strings.Contains(err.Error(), "create org unit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrgunitsUpdateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "UpdatedName",
			"orgUnitPath": "/Sales",
			"orgUnitId":   "ou-1",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "UpdatedName"
	cmd := &OrgunitsUpdateCmd{Path: "/Sales", Name: &newName}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated org unit") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "UpdatedName") {
		t.Fatalf("expected UpdatedName in output, got: %s", out)
	}
}

func TestOrgunitsUpdateCmd_WithDescription(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "Sales",
			"orgUnitPath": "/Sales",
			"orgUnitId":   "ou-1",
			"description": "Updated description",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	desc := "Updated description"
	cmd := &OrgunitsUpdateCmd{Path: "/Sales", Description: &desc}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if capturedBody["description"] != "Updated description" {
		t.Errorf("expected description update, got %v", capturedBody["description"])
	}
}

func TestOrgunitsUpdateCmd_WithParent(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":              "West",
			"orgUnitPath":       "/Marketing/West",
			"orgUnitId":         "ou-west",
			"parentOrgUnitPath": "/Marketing",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newParent := "/Marketing"
	cmd := &OrgunitsUpdateCmd{Path: "/Sales/West", Parent: &newParent}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if capturedBody["parentOrgUnitPath"] != "/Marketing" {
		t.Errorf("expected parent update, got %v", capturedBody["parentOrgUnitPath"])
	}
}

func TestOrgunitsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "JSONUpdate",
			"orgUnitPath": "/JSONUpdate",
			"orgUnitId":   "ou-json",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "JSONUpdate"
	cmd := &OrgunitsUpdateCmd{Path: "/OldName", Name: &newName}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"orgUnitPath"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestOrgunitsUpdateCmd_NoUpdates(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called
		t.Error("API should not be called when no updates specified")
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsUpdateCmd{Path: "/Sales"} // No Name, Parent, or Description

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when no updates specified")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Errorf("expected usage error (exit code 2), got %v", err)
	}
}

func TestOrgunitsUpdateCmd_ClearDescription(t *testing.T) {
	var capturedBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "Sales",
			"orgUnitPath": "/Sales",
			"orgUnitId":   "ou-1",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	emptyDesc := ""
	cmd := &OrgunitsUpdateCmd{Path: "/Sales", Description: &emptyDesc}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The empty string should be explicitly sent
	if capturedBody["description"] != "" {
		t.Errorf("expected empty description to be sent, got %v", capturedBody["description"])
	}
}

func TestOrgunitsUpdateCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "NewName"
	cmd := &OrgunitsUpdateCmd{Path: "/NonExistent", Name: &newName}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for non-existent org unit")
	}
	if !strings.Contains(err.Error(), "update org unit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrgunitsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &OrgunitsDeleteCmd{Path: "/OldUnit"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted org unit") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "/OldUnit") {
		t.Fatalf("expected path in output, got: %s", out)
	}
}

func TestOrgunitsDeleteCmd_RequiresForce(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called without Force flag
		t.Error("API should not be called without Force flag")
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true} // No Force, non-interactive
	cmd := &OrgunitsDeleteCmd{Path: "/OldUnit"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error without --force")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("expected ExitError, got %v", err)
	}
}

func TestOrgunitsDeleteCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &OrgunitsDeleteCmd{Path: "/NonExistent"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for non-existent org unit")
	}
	if !strings.Contains(err.Error(), "delete org unit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrgunitsDeleteCmd_ByID(t *testing.T) {
	var capturedPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		// Capture the path part after /orgunits/
		parts := strings.Split(r.URL.Path, "/orgunits/")
		if len(parts) > 1 {
			capturedPath = parts[1]
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &OrgunitsDeleteCmd{Path: "id:ou-12345"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// The path should be passed through (org unit ID format)
	if !strings.Contains(capturedPath, "id:ou-12345") && !strings.Contains(capturedPath, "ou-12345") {
		t.Errorf("expected org unit ID to be passed, got path: %s", capturedPath)
	}
	if !strings.Contains(out, "Deleted org unit") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestOrgunitsListCmd_Error(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
	if !strings.Contains(err.Error(), "list org units") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrgunitsListCmd_WithType(t *testing.T) {
	var capturedType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		capturedType = r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "All1", "orgUnitPath": "/All1", "orgUnitId": "ou-all1"},
				{"name": "All2", "orgUnitPath": "/All1/All2", "orgUnitId": "ou-all2"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{Type: "all"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if capturedType != "all" {
		t.Errorf("expected type=all, got %s", capturedType)
	}
	if !strings.Contains(out, "All1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

// Test that requireAccount returns error when no account is provided
func TestOrgunitsCmd_RequiresAccount(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called
		t.Error("API should not be called without account")
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{} // No account specified
	cmd := &OrgunitsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when account not specified")
	}
}
