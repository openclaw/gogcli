package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

// =============================================================================
// Buildings Tests
// =============================================================================

func TestResourcesBuildingsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildings": []map[string]any{
				{"buildingId": "b1", "buildingName": "HQ", "floorNames": []string{"1", "2"}},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "HQ") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesBuildingsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildings": []map[string]any{
				{"buildingId": "b1", "buildingName": "HQ", "floorNames": []string{"1", "2"}},
				{"buildingId": "b2", "buildingName": "Annex", "description": "Secondary building"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "buildings") || !strings.Contains(out, "b1") {
		t.Fatalf("expected JSON buildings output, got: %s", out)
	}
}

func TestResourcesBuildingsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildings": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsListCmd{}

	// Should not error on empty list
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResourcesBuildingsListCmd_Pagination(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		// Check pagination parameters
		if r.URL.Query().Get("maxResults") != "10" {
			t.Errorf("expected maxResults=10, got %s", r.URL.Query().Get("maxResults"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildings": []map[string]any{
				{"buildingId": "b1", "buildingName": "HQ"},
			},
			"nextPageToken": "token123",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsListCmd{Max: 10}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResourcesBuildingsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b1",
			"buildingName": "Headquarters",
			"description":  "Main office building",
			"floorNames":   []string{"Ground", "1", "2"},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsGetCmd{BuildingID: "b1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ID:") || !strings.Contains(out, "b1") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Name:") || !strings.Contains(out, "Headquarters") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesBuildingsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b1",
			"buildingName": "Headquarters",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsGetCmd{BuildingID: "b1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "buildingId") || !strings.Contains(out, "b1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesBuildingsGetCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsGetCmd{BuildingID: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty building ID")
	}
	if !strings.Contains(err.Error(), "building ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesBuildingsCreateCmd(t *testing.T) {
	var gotPayload struct {
		BuildingName string   `json:"buildingName"`
		Description  string   `json:"description"`
		FloorNames   []string `json:"floorNames"`
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b-new",
			"buildingName": gotPayload.BuildingName,
			"description":  gotPayload.Description,
			"floorNames":   gotPayload.FloorNames,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsCreateCmd{
		Name:        "New Building",
		Description: "A new office building",
		Floors:      "Ground, 1, 2, 3",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPayload.BuildingName != "New Building" {
		t.Errorf("expected name 'New Building', got %q", gotPayload.BuildingName)
	}
	if gotPayload.Description != "A new office building" {
		t.Errorf("expected description 'A new office building', got %q", gotPayload.Description)
	}
	if len(gotPayload.FloorNames) != 4 {
		t.Errorf("expected 4 floors, got %d", len(gotPayload.FloorNames))
	}
	if !strings.Contains(out, "Created building") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesBuildingsCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/buildings") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b-new",
			"buildingName": "New Building",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsCreateCmd{Name: "New Building"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "buildingId") || !strings.Contains(out, "b-new") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesBuildingsCreateCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsCreateCmd{Name: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesBuildingsUpdateCmd(t *testing.T) {
	var gotPayload struct {
		BuildingName string `json:"buildingName"`
		Description  string `json:"description"`
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b1",
			"buildingName": gotPayload.BuildingName,
			"description":  gotPayload.Description,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated HQ"
	newDesc := "Updated description"
	cmd := &ResourcesBuildingsUpdateCmd{
		BuildingID:  "b1",
		Name:        &newName,
		Description: &newDesc,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPayload.BuildingName != "Updated HQ" {
		t.Errorf("expected name 'Updated HQ', got %q", gotPayload.BuildingName)
	}
	if gotPayload.Description != "Updated description" {
		t.Errorf("expected description 'Updated description', got %q", gotPayload.Description)
	}
	if !strings.Contains(out, "Updated building") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesBuildingsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"buildingId":   "b1",
			"buildingName": "Updated HQ",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated HQ"
	cmd := &ResourcesBuildingsUpdateCmd{BuildingID: "b1", Name: &newName}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "buildingId") || !strings.Contains(out, "b1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesBuildingsUpdateCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated"
	cmd := &ResourcesBuildingsUpdateCmd{BuildingID: "  ", Name: &newName}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty building ID")
	}
	if !strings.Contains(err.Error(), "building ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesBuildingsUpdateCmd_NoUpdates(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesBuildingsUpdateCmd{BuildingID: "b1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesBuildingsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesBuildingsDeleteCmd{BuildingID: "b1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted building") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesBuildingsDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/buildings/b1") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesBuildingsDeleteCmd{BuildingID: "b1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") || !strings.Contains(out, "true") {
		t.Fatalf("expected JSON output with deleted:true, got: %s", out)
	}
}

func TestResourcesBuildingsDeleteCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesBuildingsDeleteCmd{BuildingID: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty building ID")
	}
	if !strings.Contains(err.Error(), "building ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Calendars Tests
// =============================================================================

func TestResourcesCalendarsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"resourceId":       "r1",
					"resourceName":     "Conference Room A",
					"resourceEmail":    "room-a@example.com",
					"resourceCategory": "CONFERENCE_ROOM",
					"buildingId":       "b1",
					"floorName":        "1",
					"capacity":         10,
				},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Conference Room A") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"resourceId": "r1", "resourceName": "Conference Room A"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "items") || !strings.Contains(out, "r1") {
		t.Fatalf("expected JSON items output, got: %s", out)
	}
}

func TestResourcesCalendarsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsListCmd{}

	// Should not error on empty list
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResourcesCalendarsListCmd_FilterByBuilding(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"resourceId": "r1", "resourceName": "Room A", "buildingId": "b1"},
				{"resourceId": "r2", "resourceName": "Room B", "buildingId": "b2"},
				{"resourceId": "r3", "resourceName": "Room C", "buildingId": "b1"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsListCmd{Building: "b1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should show only rooms in b1
	if !strings.Contains(out, "Room A") || !strings.Contains(out, "Room C") {
		t.Fatalf("expected rooms from b1, got: %s", out)
	}
	if strings.Contains(out, "Room B") {
		t.Fatalf("should not contain Room B (b2), got: %s", out)
	}
}

func TestResourcesCalendarsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":             "r1",
			"resourceName":           "Conference Room A",
			"resourceEmail":          "room-a@example.com",
			"resourceCategory":       "CONFERENCE_ROOM",
			"resourceDescription":    "Main conference room",
			"userVisibleDescription": "Large room with projector",
			"buildingId":             "b1",
			"floorName":              "1",
			"capacity":               15,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsGetCmd{ResourceID: "r1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ID:") || !strings.Contains(out, "r1") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Name:") || !strings.Contains(out, "Conference Room A") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Email:") || !strings.Contains(out, "room-a@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":   "r1",
			"resourceName": "Conference Room A",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsGetCmd{ResourceID: "r1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "resourceId") || !strings.Contains(out, "r1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesCalendarsGetCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsGetCmd{ResourceID: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty resource ID")
	}
	if !strings.Contains(err.Error(), "resource ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesCalendarsCreateCmd(t *testing.T) {
	var gotName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			ResourceName     string `json:"resourceName"`
			ResourceCategory string `json:"resourceCategory"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotName = payload.ResourceName
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":       "r1",
			"resourceName":     payload.ResourceName,
			"resourceEmail":    "room@example.com",
			"resourceCategory": payload.ResourceCategory,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsCreateCmd{Name: "Training Room", Type: "CONFERENCE_ROOM"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "Training Room" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if !strings.Contains(out, "Created calendar resource") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsCreateCmd_WithAllFields(t *testing.T) {
	var gotPayload struct {
		ResourceName     string `json:"resourceName"`
		ResourceCategory string `json:"resourceCategory"`
		BuildingId       string `json:"buildingId"`
		FloorName        string `json:"floorName"`
		Capacity         int64  `json:"capacity"`
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":       "r-new",
			"resourceName":     gotPayload.ResourceName,
			"resourceEmail":    "new-room@example.com",
			"resourceCategory": gotPayload.ResourceCategory,
			"buildingId":       gotPayload.BuildingId,
			"floorName":        gotPayload.FloorName,
			"capacity":         gotPayload.Capacity,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsCreateCmd{
		Name:     "Large Meeting Room",
		Type:     "CONFERENCE_ROOM",
		Building: "b1",
		Floor:    "2",
		Capacity: 20,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPayload.ResourceName != "Large Meeting Room" {
		t.Errorf("expected name 'Large Meeting Room', got %q", gotPayload.ResourceName)
	}
	if gotPayload.ResourceCategory != "CONFERENCE_ROOM" {
		t.Errorf("expected category 'CONFERENCE_ROOM', got %q", gotPayload.ResourceCategory)
	}
	if gotPayload.BuildingId != "b1" {
		t.Errorf("expected buildingId 'b1', got %q", gotPayload.BuildingId)
	}
	if gotPayload.FloorName != "2" {
		t.Errorf("expected floor '2', got %q", gotPayload.FloorName)
	}
	if gotPayload.Capacity != 20 {
		t.Errorf("expected capacity 20, got %d", gotPayload.Capacity)
	}
	if !strings.Contains(out, "Created calendar resource") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/calendars") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":       "r-new",
			"resourceName":     "New Room",
			"resourceEmail":    "new-room@example.com",
			"resourceCategory": "OTHER",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsCreateCmd{Name: "New Room", Type: "OTHER"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "resourceId") || !strings.Contains(out, "r-new") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesCalendarsCreateCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsCreateCmd{Name: "  ", Type: "CONFERENCE_ROOM"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesCalendarsUpdateCmd(t *testing.T) {
	var gotPayload struct {
		ResourceName string `json:"resourceName"`
		Capacity     int64  `json:"capacity"`
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":   "r1",
			"resourceName": gotPayload.ResourceName,
			"capacity":     gotPayload.Capacity,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated Room"
	newCapacity := int64(25)
	cmd := &ResourcesCalendarsUpdateCmd{
		ResourceID: "r1",
		Name:       &newName,
		Capacity:   &newCapacity,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPayload.ResourceName != "Updated Room" {
		t.Errorf("expected name 'Updated Room', got %q", gotPayload.ResourceName)
	}
	if gotPayload.Capacity != 25 {
		t.Errorf("expected capacity 25, got %d", gotPayload.Capacity)
	}
	if !strings.Contains(out, "Updated calendar resource") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceId":   "r1",
			"resourceName": "Updated Room",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated Room"
	cmd := &ResourcesCalendarsUpdateCmd{ResourceID: "r1", Name: &newName}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "resourceId") || !strings.Contains(out, "r1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesCalendarsUpdateCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated"
	cmd := &ResourcesCalendarsUpdateCmd{ResourceID: "  ", Name: &newName}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty resource ID")
	}
	if !strings.Contains(err.Error(), "resource ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesCalendarsUpdateCmd_NoUpdates(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesCalendarsUpdateCmd{ResourceID: "r1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesCalendarsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesCalendarsDeleteCmd{ResourceID: "r1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted calendar resource") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesCalendarsDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/calendars/r1") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesCalendarsDeleteCmd{ResourceID: "r1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") || !strings.Contains(out, "true") {
		t.Fatalf("expected JSON output with deleted:true, got: %s", out)
	}
}

func TestResourcesCalendarsDeleteCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesCalendarsDeleteCmd{ResourceID: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty resource ID")
	}
	if !strings.Contains(err.Error(), "resource ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Features Tests
// =============================================================================

func TestResourcesFeaturesListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features": []map[string]any{
				{"name": "Projector"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Projector") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesFeaturesListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features": []map[string]any{
				{"name": "Projector"},
				{"name": "Whiteboard"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "features") || !strings.Contains(out, "Projector") {
		t.Fatalf("expected JSON features output, got: %s", out)
	}
}

func TestResourcesFeaturesListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features": []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesListCmd{}

	// Should not error on empty list
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResourcesFeaturesListCmd_Pagination(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		// Check pagination parameters
		if r.URL.Query().Get("maxResults") != "50" {
			t.Errorf("expected maxResults=50, got %s", r.URL.Query().Get("maxResults"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features": []map[string]any{
				{"name": "Projector"},
			},
			"nextPageToken": "token456",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesListCmd{Max: 50}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResourcesFeaturesCreateCmd(t *testing.T) {
	var gotName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotName = payload.Name
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": payload.Name,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesCreateCmd{Name: "Video Conferencing"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "Video Conferencing" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if !strings.Contains(out, "Created feature") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesFeaturesCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/resources/features") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "New Feature",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesCreateCmd{Name: "New Feature"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "name") || !strings.Contains(out, "New Feature") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResourcesFeaturesCreateCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResourcesFeaturesCreateCmd{Name: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResourcesFeaturesDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/features/Projector") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesFeaturesDeleteCmd{Name: "Projector"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted feature") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResourcesFeaturesDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/resources/features/Projector") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesFeaturesDeleteCmd{Name: "Projector"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "deleted") || !strings.Contains(out, "true") {
		t.Fatalf("expected JSON output with deleted:true, got: %s", out)
	}
}

func TestResourcesFeaturesDeleteCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ResourcesFeaturesDeleteCmd{Name: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty feature name")
	}
	if !strings.Contains(err.Error(), "feature name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
