package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/ui"
)

type sheetsRawHit struct {
	includeGridData atomic.Bool
}

func newSheetsRawTestServer(t *testing.T, status int, body map[string]any, hit *sheetsRawHit) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		if !strings.HasPrefix(path, "/spreadsheets/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if hit != nil && r.URL.Query().Get("includeGridData") == "true" {
			hit.includeGridData.Store(true)
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockSheetsService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", sheets.NewService)
	stubGoogleTestService(t, &newSheetsService, svc)
}

func fullSheetResponse(id string) map[string]any {
	return map[string]any{
		"spreadsheetId":  id,
		"spreadsheetUrl": "http://example.com/" + id,
		"properties": map[string]any{
			"title":    "Full Sheet",
			"locale":   "en_US",
			"timeZone": "UTC",
		},
		"sheets": []map[string]any{
			{
				"properties": map[string]any{
					"sheetId": 1,
					"title":   "Sheet1",
					"gridProperties": map[string]any{
						"rowCount":    100,
						"columnCount": 26,
					},
				},
			},
		},
	}
}

func rawContextWithStderr(t *testing.T, stderr io.Writer) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func TestSheetsRaw_HappyPath_NoGridDataByDefault(t *testing.T) {
	hit := &sheetsRawHit{}
	srv := newSheetsRawTestServer(t, 0, fullSheetResponse("s1"), hit)
	defer srv.Close()
	installMockSheetsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if hit.includeGridData.Load() {
		t.Fatalf("--include-grid-data should not be set by default")
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["spreadsheetId"] != "s1" {
		t.Fatalf("expected spreadsheetId=s1, got: %v", got["spreadsheetId"])
	}
	if _, ok := got["sheets"]; !ok {
		t.Fatalf("expected sheets in raw output")
	}
}

func TestSheetsRaw_IncludeGridDataFlag(t *testing.T) {
	hit := &sheetsRawHit{}
	srv := newSheetsRawTestServer(t, 0, fullSheetResponse("s1"), hit)
	defer srv.Close()
	installMockSheetsService(t, srv)

	var stderr bytes.Buffer
	ctx := rawContextWithStderr(t, &stderr)
	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		if err := runKong(t, &SheetsRawCmd{}, []string{"s1", "--include-grid-data"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if !hit.includeGridData.Load() {
		t.Fatalf("expected includeGridData=true in request")
	}
	// Audit requires a stderr warning when grid data is included.
	if !strings.Contains(stderr.String(), "grid") {
		t.Fatalf("expected stderr warning mentioning 'grid', got: %q", stderr.String())
	}
}

func TestSheetsRaw_APIError(t *testing.T) {
	srv := newSheetsRawTestServer(t, http.StatusInternalServerError, nil, nil)
	defer srv.Close()
	installMockSheetsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestSheetsRaw_NotFound(t *testing.T) {
	srv := newSheetsRawTestServer(t, http.StatusNotFound, nil, nil)
	defer srv.Close()
	installMockSheetsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestSheetsRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&SheetsRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
