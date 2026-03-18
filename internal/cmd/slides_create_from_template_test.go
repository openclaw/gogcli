package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestSlidesCreateFromTemplate_Basic(t *testing.T) {
	var capturedDriveRequest *http.Request
	var capturedSlidesRequests []*slides.Request

	driveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDriveRequest = r

		// Handle copy request - the path includes /v3/files/{id}/copy
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/files/template123/copy") {
			response := &drive.File{
				Id:          "copied123",
				Name:        "New Presentation",
				MimeType:    "application/vnd.google-apps.presentation",
				WebViewLink: "https://docs.google.com/presentation/d/copied123/edit",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer driveServer.Close()

	slidesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/presentations/copied123:batchUpdate" {
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			capturedSlidesRequests = req.Requests

			// Build response with replacement statistics
			replies := make([]*slides.Response, len(req.Requests))
			for i := range req.Requests {
				replies[i] = &slides.Response{
					ReplaceAllText: &slides.ReplaceAllTextResponse{
						OccurrencesChanged: 2,
					},
				}
			}

			response := &slides.BatchUpdatePresentationResponse{
				PresentationId: "copied123",
				Replies:        replies,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer slidesServer.Close()

	// Create Drive service
	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(driveServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	// Create Slides service
	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(slidesServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	oldNewDrive := newDriveService
	oldNewSlides := newSlidesService
	defer func() {
		newDriveService = oldNewDrive
		newSlidesService = oldNewSlides
	}()

	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID: "template123",
		Title:      "New Presentation",
		Replace:    []string{"name=John Doe", "company=ACME Corp"},
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify Drive API call
	if capturedDriveRequest == nil {
		t.Fatal("Drive API was not called")
	}

	// Verify Slides API calls
	if len(capturedSlidesRequests) != 2 {
		t.Fatalf("Expected 2 replacement requests, got %d", len(capturedSlidesRequests))
	}

	got := make(map[string]string, len(capturedSlidesRequests))
	for _, req := range capturedSlidesRequests {
		if req.ReplaceAllText == nil {
			t.Fatal("request is not ReplaceAllText")
		}
		got[req.ReplaceAllText.ContainsText.Text] = req.ReplaceAllText.ReplaceText
	}
	if got["{{name}}"] != "John Doe" {
		t.Errorf("expected {{name}} => John Doe, got %q", got["{{name}}"])
	}
	if got["{{company}}"] != "ACME Corp" {
		t.Errorf("expected {{company}} => ACME Corp, got %q", got["{{company}}"])
	}
}

func TestSlidesCreateFromTemplate_JSONFile(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "replacements.json")

	replacements := map[string]interface{}{
		"name":    "Jane Smith",
		"age":     30,
		"active":  true,
		"company": "TechCorp",
	}

	data, err := json.Marshal(replacements)
	if err != nil {
		t.Fatal(err)
	}

	if writeErr := os.WriteFile(jsonFile, data, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	var capturedSlidesRequests []*slides.Request

	driveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/files/template456/copy") {
			response := &drive.File{
				Id:          "copied456",
				Name:        "Test Presentation",
				MimeType:    "application/vnd.google-apps.presentation",
				WebViewLink: "https://docs.google.com/presentation/d/copied456/edit",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer driveServer.Close()

	slidesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/presentations/copied456:batchUpdate" {
			var req slides.BatchUpdatePresentationRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
				http.Error(w, decodeErr.Error(), http.StatusBadRequest)
				return
			}

			capturedSlidesRequests = req.Requests

			replies := make([]*slides.Response, len(req.Requests))
			for i := range req.Requests {
				replies[i] = &slides.Response{
					ReplaceAllText: &slides.ReplaceAllTextResponse{
						OccurrencesChanged: 1,
					},
				}
			}

			response := &slides.BatchUpdatePresentationResponse{
				PresentationId: "copied456",
				Replies:        replies,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer slidesServer.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(driveServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(slidesServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	oldNewDrive := newDriveService
	oldNewSlides := newSlidesService
	defer func() {
		newDriveService = oldNewDrive
		newSlidesService = oldNewSlides
	}()

	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID:   "template456",
		Title:        "Test Presentation",
		Replacements: jsonFile,
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have 4 replacements
	if len(capturedSlidesRequests) != 4 {
		t.Fatalf("Expected 4 replacement requests, got %d", len(capturedSlidesRequests))
	}

	// Verify type conversions
	foundAge := false
	foundActive := false
	for _, req := range capturedSlidesRequests {
		if req.ReplaceAllText != nil {
			text := req.ReplaceAllText.ContainsText.Text
			if text == "{{age}}" {
				foundAge = true
				if req.ReplaceAllText.ReplaceText != "30" {
					t.Errorf("Expected age '30', got %s", req.ReplaceAllText.ReplaceText)
				}
			}
			if text == "{{active}}" {
				foundActive = true
				if req.ReplaceAllText.ReplaceText != "true" {
					t.Errorf("Expected active 'true', got %s", req.ReplaceAllText.ReplaceText)
				}
			}
		}
	}

	if !foundAge {
		t.Error("Did not find age replacement")
	}
	if !foundActive {
		t.Error("Did not find active replacement")
	}
}

func TestSlidesCreateFromTemplate_ExactMode(t *testing.T) {
	var capturedSlidesRequests []*slides.Request

	driveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/files/template789/copy") {
			response := &drive.File{
				Id:          "copied789",
				Name:        "Exact Mode Test",
				MimeType:    "application/vnd.google-apps.presentation",
				WebViewLink: "https://docs.google.com/presentation/d/copied789/edit",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer driveServer.Close()

	slidesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/presentations/copied789:batchUpdate" {
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			capturedSlidesRequests = req.Requests

			replies := make([]*slides.Response, len(req.Requests))
			for i := range req.Requests {
				replies[i] = &slides.Response{
					ReplaceAllText: &slides.ReplaceAllTextResponse{
						OccurrencesChanged: 1,
					},
				}
			}

			response := &slides.BatchUpdatePresentationResponse{
				PresentationId: "copied789",
				Replies:        replies,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer slidesServer.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(driveServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(slidesServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	oldNewDrive := newDriveService
	oldNewSlides := newSlidesService
	defer func() {
		newDriveService = oldNewDrive
		newSlidesService = oldNewSlides
	}()

	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID: "template789",
		Title:      "Exact Mode Test",
		Replace:    []string{"OLD_TEXT=NEW_TEXT"},
		Exact:      true,
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(capturedSlidesRequests) != 1 {
		t.Fatalf("Expected 1 replacement request, got %d", len(capturedSlidesRequests))
	}

	// In exact mode, should search for "OLD_TEXT" not "{{OLD_TEXT}}"
	if capturedSlidesRequests[0].ReplaceAllText.ContainsText.Text != "OLD_TEXT" {
		t.Errorf("Expected 'OLD_TEXT', got %s", capturedSlidesRequests[0].ReplaceAllText.ContainsText.Text)
	}
}

func TestSlidesCreateFromTemplate_EmptyReplacements(t *testing.T) {
	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID: "template123",
		Title:      "Test",
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err == nil {
		t.Fatal("Expected error for empty replacements, got nil")
	}
	if ExitCode(err) != 2 {
		t.Errorf("Expected usage error (exit code 2), got: %v", err)
	}
}

func TestSlidesCreateFromTemplate_InvalidReplaceFormat(t *testing.T) {
	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID: "template123",
		Title:      "Test",
		Replace:    []string{"invalid_no_equals_sign"},
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err == nil {
		t.Fatal("Expected error for invalid replace format, got nil")
	}
}

func TestSlidesCreateFromTemplate_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(jsonFile, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID:   "template123",
		Title:        "Test",
		Replacements: jsonFile,
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestSlidesCreateFromTemplate_CombineFileAndFlags(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "replacements.json")

	fileReplacements := map[string]string{
		"name":    "From File",
		"company": "File Corp",
	}

	data, err := json.Marshal(fileReplacements)
	if err != nil {
		t.Fatal(err)
	}

	if writeErr := os.WriteFile(jsonFile, data, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	var capturedSlidesRequests []*slides.Request

	driveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			response := &drive.File{
				Id:          "copied999",
				Name:        "Combined Test",
				MimeType:    "application/vnd.google-apps.presentation",
				WebViewLink: "https://docs.google.com/presentation/d/copied999/edit",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer driveServer.Close()

	slidesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req slides.BatchUpdatePresentationRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
				http.Error(w, decodeErr.Error(), http.StatusBadRequest)
				return
			}

			capturedSlidesRequests = req.Requests

			replies := make([]*slides.Response, len(req.Requests))
			for i := range req.Requests {
				replies[i] = &slides.Response{
					ReplaceAllText: &slides.ReplaceAllTextResponse{
						OccurrencesChanged: 1,
					},
				}
			}

			response := &slides.BatchUpdatePresentationResponse{
				PresentationId: "copied999",
				Replies:        replies,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer slidesServer.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(driveServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(slidesServer.URL))
	if err != nil {
		t.Fatal(err)
	}

	oldNewDrive := newDriveService
	oldNewSlides := newSlidesService
	defer func() {
		newDriveService = oldNewDrive
		newSlidesService = oldNewSlides
	}()

	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	// Flag overrides file
	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID:   "template999",
		Title:        "Combined Test",
		Replacements: jsonFile,
		Replace:      []string{"name=From Flag"},
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have 2 replacements (name and company)
	if len(capturedSlidesRequests) != 2 {
		t.Fatalf("Expected 2 replacement requests, got %d", len(capturedSlidesRequests))
	}

	// Verify that flag value overrides file value
	foundNameOverride := false
	for _, req := range capturedSlidesRequests {
		if req.ReplaceAllText != nil && req.ReplaceAllText.ContainsText.Text == "{{name}}" {
			foundNameOverride = true
			if req.ReplaceAllText.ReplaceText != "From Flag" {
				t.Errorf("Expected 'From Flag', got %s", req.ReplaceAllText.ReplaceText)
			}
		}
	}

	if !foundNameOverride {
		t.Error("Flag should override file value for 'name'")
	}
}

func TestSlidesCreateFromTemplate_DryRunSkipsAPICalls(t *testing.T) {
	origNewDrive := newDriveService
	origNewSlides := newSlidesService
	t.Cleanup(func() {
		newDriveService = origNewDrive
		newSlidesService = origNewSlides
	})

	driveCalls := 0
	slidesCalls := 0
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		driveCalls++
		t.Fatal("drive service should not be created during dry-run")
		return &drive.Service{}, nil
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) {
		slidesCalls++
		t.Fatal("slides service should not be created during dry-run")
		return &slides.Service{}, nil
	}

	cmd := &SlidesCreateFromTemplateCmd{
		TemplateID: "template123",
		Title:      "Dry Run Deck",
		Replace:    []string{"name=John Doe"},
		Parent:     "https://drive.google.com/drive/folders/parent123",
	}

	u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com", DryRun: true, NoInput: true})
	if ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
	if driveCalls != 0 || slidesCalls != 0 {
		t.Fatalf("expected no API calls, got drive=%d slides=%d", driveCalls, slidesCalls)
	}
}
