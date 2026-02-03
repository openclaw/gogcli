package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
