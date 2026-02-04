package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drivelabels/v2"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
)

// ========== LabelsListCmd Tests ==========

func TestLabelsListCmd_Text(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{
				{"name": "labels/label1", "labelType": "ADMIN", "properties": map[string]any{"title": "Confidential"}, "lifecycle": map[string]any{"state": "PUBLISHED"}},
				{"name": "labels/label2", "labelType": "SHARED", "properties": map[string]any{"title": "Public"}, "lifecycle": map[string]any{"state": "DRAFT"}},
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Confidential") {
		t.Fatalf("expected output to contain 'Confidential', got: %s", out)
	}
	if !strings.Contains(out, "Public") {
		t.Fatalf("expected output to contain 'Public', got: %s", out)
	}
	if !strings.Contains(out, "ADMIN") {
		t.Fatalf("expected output to contain 'ADMIN', got: %s", out)
	}
	if !strings.Contains(out, "PUBLISHED") {
		t.Fatalf("expected output to contain 'PUBLISHED', got: %s", out)
	}
}

func TestLabelsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{
				{"name": "labels/label1", "labelType": "ADMIN", "properties": map[string]any{"title": "Confidential"}},
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsListCmd{}

	ctx := outfmt.WithMode(testContext(t), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, out)
	}

	labels, ok := result["labels"].([]any)
	if !ok {
		t.Fatalf("expected labels array in JSON output, got: %v", result)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
}

func TestLabelsListCmd_EmptyResults(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsListCmd{}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "No labels") {
		t.Fatalf("expected 'No labels' message in stderr, got: %s", stderr)
	}
}

func TestLabelsListCmd_Pagination(t *testing.T) {
	var gotPageSize int64
	var gotPageToken string

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}

		// Extract query params
		if ps := r.URL.Query().Get("pageSize"); ps != "" {
			var v int64
			if err := json.Unmarshal([]byte(ps), &v); err == nil {
				gotPageSize = v
			}
		}
		gotPageToken = r.URL.Query().Get("pageToken")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{
				{"name": "labels/label1", "labelType": "ADMIN", "properties": map[string]any{"title": "Test"}},
			},
			"nextPageToken": "next-token-123",
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsListCmd{Max: 10, Page: "prev-token"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageSize != 10 {
		t.Fatalf("expected pageSize 10, got %d", gotPageSize)
	}
	if gotPageToken != "prev-token" {
		t.Fatalf("expected pageToken 'prev-token', got %q", gotPageToken)
	}
}

func TestLabelsListCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    500,
				"message": "Internal server error",
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list labels") {
		t.Fatalf("expected error to contain 'list labels', got: %v", err)
	}
}

// ========== LabelsGetCmd Tests ==========

func TestLabelsGetCmd_Text(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels/label1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "labels/label1",
			"labelType":  "ADMIN",
			"properties": map[string]any{"title": "Confidential"},
			"lifecycle":  map[string]any{"state": "PUBLISHED"},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsGetCmd{LabelID: "label1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "labels/label1") {
		t.Fatalf("expected output to contain 'labels/label1', got: %s", out)
	}
	if !strings.Contains(out, "Confidential") {
		t.Fatalf("expected output to contain 'Confidential', got: %s", out)
	}
	if !strings.Contains(out, "ADMIN") {
		t.Fatalf("expected output to contain 'ADMIN', got: %s", out)
	}
	if !strings.Contains(out, "PUBLISHED") {
		t.Fatalf("expected output to contain 'PUBLISHED', got: %s", out)
	}
}

func TestLabelsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels/label1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "labels/label1",
			"labelType":  "ADMIN",
			"properties": map[string]any{"title": "Confidential"},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsGetCmd{LabelID: "label1"}

	ctx := outfmt.WithMode(testContextWithStdout(t), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, out)
	}

	if result["name"] != "labels/label1" {
		t.Fatalf("expected name 'labels/label1', got: %v", result["name"])
	}
}

func TestLabelsGetCmd_NormalizesLabelName(t *testing.T) {
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "labels/my-label",
			"labelType":  "ADMIN",
			"properties": map[string]any{"title": "Test"},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	// Pass label without "labels/" prefix - should be normalized
	cmd := &LabelsGetCmd{LabelID: "my-label"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// The path should contain "labels/my-label"
	if !strings.Contains(gotPath, "labels/my-label") {
		t.Fatalf("expected path to contain 'labels/my-label', got: %s", gotPath)
	}
}

func TestLabelsGetCmd_EmptyLabelID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsGetCmd{LabelID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty label-id, got nil")
	}
	if !strings.Contains(err.Error(), "label-id is required") {
		t.Fatalf("expected error to contain 'label-id is required', got: %v", err)
	}
}

func TestLabelsGetCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "Label not found",
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsGetCmd{LabelID: "nonexistent"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get label") {
		t.Fatalf("expected error to contain 'get label', got: %v", err)
	}
}

// ========== LabelsCreateCmd Tests ==========

func TestLabelsCreateCmd_Text(t *testing.T) {
	var gotTitle string
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			LabelType  string `json:"labelType"`
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotType = payload.LabelType
		gotTitle = payload.Properties.Title
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "labels/label1", "labelType": payload.LabelType, "properties": map[string]any{"title": payload.Properties.Title}})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsCreateCmd{Name: "Confidential", Type: "ADMIN"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotTitle != "Confidential" || gotType != "ADMIN" {
		t.Fatalf("unexpected payload: title=%q type=%q", gotTitle, gotType)
	}
	if !strings.Contains(out, "Created label") {
		t.Fatalf("expected output to contain 'Created label', got: %s", out)
	}
}

func TestLabelsCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "labels/new-label",
			"labelType":  "SHARED",
			"properties": map[string]any{"title": "My New Label"},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsCreateCmd{Name: "My New Label", Type: "SHARED"}

	ctx := outfmt.WithMode(testContextWithStdout(t), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, out)
	}

	if result["name"] != "labels/new-label" {
		t.Fatalf("expected name 'labels/new-label', got: %v", result["name"])
	}
}

func TestLabelsCreateCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsCreateCmd{Name: "   ", Type: "ADMIN"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected error to contain '--name is required', got: %v", err)
	}
}

func TestLabelsCreateCmd_InvalidType(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsCreateCmd{Name: "Test Label", Type: "INVALID"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Fatalf("expected error to contain 'invalid --type', got: %v", err)
	}
}

func TestLabelsCreateCmd_TypeCaseInsensitive(t *testing.T) {
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			LabelType string `json:"labelType"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotType = payload.LabelType
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "labels/label1"})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	// Test lowercase input
	cmd := &LabelsCreateCmd{Name: "Test", Type: "shared"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotType != "SHARED" {
		t.Fatalf("expected type to be normalized to 'SHARED', got: %s", gotType)
	}
}

func TestLabelsCreateCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    403,
				"message": "Insufficient permissions",
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsCreateCmd{Name: "Test", Type: "ADMIN"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create label") {
		t.Fatalf("expected error to contain 'create label', got: %v", err)
	}
}

// ========== LabelsDeleteCmd Tests ==========

func TestLabelsDeleteCmd_Text(t *testing.T) {
	var deleteCalled bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/v2/labels/label1") {
			http.NotFound(w, r)
			return
		}
		deleteCalled = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LabelsDeleteCmd{LabelID: "label1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("expected delete API to be called")
	}
	if !strings.Contains(out, "Deleted label") {
		t.Fatalf("expected output to contain 'Deleted label', got: %s", out)
	}
}

func TestLabelsDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/v2/labels/label1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LabelsDeleteCmd{LabelID: "label1"}

	ctx := outfmt.WithMode(testContextWithStdout(t), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, out)
	}

	if result["deleted"] != true {
		t.Fatalf("expected deleted=true, got: %v", result["deleted"])
	}
	if result["label"] != "labels/label1" {
		t.Fatalf("expected label='labels/label1', got: %v", result["label"])
	}
}

func TestLabelsDeleteCmd_EmptyLabelID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LabelsDeleteCmd{LabelID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty label-id, got nil")
	}
	if !strings.Contains(err.Error(), "label-id is required") {
		t.Fatalf("expected error to contain 'label-id is required', got: %v", err)
	}
}

func TestLabelsDeleteCmd_NormalizesLabelName(t *testing.T) {
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	// Pass label without "labels/" prefix - should be normalized
	cmd := &LabelsDeleteCmd{LabelID: "my-label-to-delete"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(gotPath, "labels/my-label-to-delete") {
		t.Fatalf("expected path to contain 'labels/my-label-to-delete', got: %s", gotPath)
	}
}

func TestLabelsDeleteCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "Label not found",
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LabelsDeleteCmd{LabelID: "nonexistent"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "delete label") {
		t.Fatalf("expected error to contain 'delete label', got: %v", err)
	}
}

// ========== LabelsUpdateCmd Tests ==========

func TestLabelsUpdateCmd_Text(t *testing.T) {
	var gotTitle string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels/label1:delta") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Requests []struct {
				UpdateLabel struct {
					Properties struct {
						Title string `json:"title"`
					} `json:"properties"`
					UpdateMask string `json:"updateMask"`
				} `json:"updateLabel"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(payload.Requests) > 0 {
			gotTitle = payload.Requests[0].UpdateLabel.Properties.Title
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"updated": true})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	name := "Renamed"
	cmd := &LabelsUpdateCmd{LabelID: "label1", Name: &name}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotTitle != "Renamed" {
		t.Fatalf("unexpected title: %q", gotTitle)
	}
	if !strings.Contains(out, "Updated label") {
		t.Fatalf("expected output to contain 'Updated label', got: %s", out)
	}
}

func TestLabelsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels/label1:delta") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{{"updateLabel": map[string]any{}}},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	name := "New Title"
	cmd := &LabelsUpdateCmd{LabelID: "label1", Name: &name}

	ctx := outfmt.WithMode(testContextWithStdout(t), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, out)
	}

	if _, ok := result["responses"]; !ok {
		t.Fatalf("expected 'responses' in JSON output, got: %v", result)
	}
}

func TestLabelsUpdateCmd_EmptyLabelID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	name := "New Title"
	cmd := &LabelsUpdateCmd{LabelID: "   ", Name: &name}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty label-id, got nil")
	}
	if !strings.Contains(err.Error(), "label-id is required") {
		t.Fatalf("expected error to contain 'label-id is required', got: %v", err)
	}
}

func TestLabelsUpdateCmd_NoUpdates(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LabelsUpdateCmd{LabelID: "label1", Name: nil}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates, got nil")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("expected error to contain 'no updates specified', got: %v", err)
	}
}

func TestLabelsUpdateCmd_EmptyNameValue(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	emptyName := "   "
	cmd := &LabelsUpdateCmd{LabelID: "label1", Name: &emptyName}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name value, got nil")
	}
	if !strings.Contains(err.Error(), "--name cannot be empty") {
		t.Fatalf("expected error to contain '--name cannot be empty', got: %v", err)
	}
}

func TestLabelsUpdateCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    500,
				"message": "Update failed",
			},
		})
	})
	stubDriveLabels(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	name := "New Title"
	cmd := &LabelsUpdateCmd{LabelID: "label1", Name: &name}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "update label") {
		t.Fatalf("expected error to contain 'update label', got: %v", err)
	}
}

// ========== Helper Functions ==========

func stubDriveLabels(t *testing.T, handler http.Handler) {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newDriveLabelsService
	svc, err := drivelabels.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new drive labels service: %v", err)
	}
	newDriveLabelsService = func(context.Context, string) (*drivelabels.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newDriveLabelsService = orig
		srv.Close()
	})
}
