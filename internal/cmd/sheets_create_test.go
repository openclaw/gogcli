package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestSheetsCreateCmd_ParentMoveSuccess(t *testing.T) {
	origSheets := newSheetsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSheetsService = origSheets
		newDriveService = origDrive
	})

	sheetsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v4/spreadsheets") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spreadsheetId":  "id2",
			"spreadsheetUrl": "https://example.test/sheets/id2",
			"properties":     map[string]any{"title": "Budget"},
		})
	}))
	defer sheetsSrv.Close()

	var sawGet bool
	var sawPatch bool
	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/files/id2") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			sawGet = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "id2",
				"parents": []string{"root"},
			})
		case http.MethodPatch:
			sawPatch = true
			if got := r.URL.Query().Get("addParents"); got != "folder123" {
				t.Fatalf("addParents=%q", got)
			}
			if got := r.URL.Query().Get("removeParents"); got != "root" {
				t.Fatalf("removeParents=%q", got)
			}
			if got := r.URL.Query().Get("supportsAllDrives"); got != "true" {
				t.Fatalf("supportsAllDrives=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "id2",
				"parents": []string{"folder123"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	t.Setenv("GOG_ACCOUNT", "a@b.com")

	sheetsSvc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(sheetsSrv.Client()),
		option.WithEndpoint(sheetsSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("sheets.NewService: %v", err)
	}
	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return sheetsSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	var payload map[string]any
	stderr := captureStderr(t, func() {
		stdout := captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "create", "Budget", "--parent", "folder123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("json.Unmarshal: %v\nstdout=%q", err, stdout)
		}
	})

	if !sawGet || !sawPatch {
		t.Fatalf("expected drive get+patch, sawGet=%v sawPatch=%v", sawGet, sawPatch)
	}
	if got := payload["parent"]; got != "folder123" {
		t.Fatalf("parent=%v", got)
	}
	if got := payload["movedToParent"]; got != true {
		t.Fatalf("movedToParent=%v", got)
	}
	if _, ok := payload["moveError"]; ok {
		t.Fatalf("unexpected moveError=%v", payload["moveError"])
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("unexpected stderr=%q", stderr)
	}
}

func TestSheetsCreateCmd_ParentMoveFailureReportedInJSON(t *testing.T) {
	origSheets := newSheetsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSheetsService = origSheets
		newDriveService = origDrive
	})

	sheetsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v4/spreadsheets") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spreadsheetId":  "id2",
			"spreadsheetUrl": "https://example.test/sheets/id2",
			"properties":     map[string]any{"title": "Budget"},
		})
	}))
	defer sheetsSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/files/id2") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "id2",
				"parents": []string{"root"},
			})
		case http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    403,
					"message": "forbidden",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	t.Setenv("GOG_ACCOUNT", "a@b.com")

	sheetsSvc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(sheetsSrv.Client()),
		option.WithEndpoint(sheetsSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("sheets.NewService: %v", err)
	}
	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return sheetsSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	var payload map[string]any
	stderr := captureStderr(t, func() {
		stdout := captureStdout(t, func() {
			if err := Execute([]string{"--json", "sheets", "create", "Budget", "--parent", "folder123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("json.Unmarshal: %v\nstdout=%q", err, stdout)
		}
	})

	if got := payload["parent"]; got != "folder123" {
		t.Fatalf("parent=%v", got)
	}
	if got := payload["movedToParent"]; got != false {
		t.Fatalf("movedToParent=%v", got)
	}
	moveError, _ := payload["moveError"].(string)
	if !strings.Contains(moveError, "forbidden") {
		t.Fatalf("moveError=%q", moveError)
	}
	if !strings.Contains(stderr, "failed to move spreadsheet to folder") {
		t.Fatalf("stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "Spreadsheet created in Drive root") {
		t.Fatalf("stderr=%q", stderr)
	}
}
