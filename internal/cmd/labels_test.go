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
)

func TestLabelsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{
				{"name": "labels/label1", "labelType": "ADMIN", "properties": map[string]any{"title": "Confidential"}, "lifecycle": map[string]any{"state": "PUBLISHED"}},
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
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLabelsCreateCmd(t *testing.T) {
	var gotTitle string
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v2/labels") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			LabelType string `json:"labelType"`
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
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLabelsUpdateCmd(t *testing.T) {
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
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubDriveLabels(t *testing.T, handler http.Handler) *httptest.Server {
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
	return srv
}
