package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
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

func newSheetsRawTestContext(t *testing.T, srv *httptest.Server, stdout, stderr io.Writer) context.Context {
	t.Helper()
	svc := newSheetsServiceFromServer(t, srv)
	return withSheetsTestService(newCmdRuntimeOutputContext(t, stdout, stderr), svc)
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

func TestSheetsRaw_HappyPath_NoGridDataByDefault(t *testing.T) {
	hit := &sheetsRawHit{}
	srv := newSheetsRawTestServer(t, 0, fullSheetResponse("s1"), hit)
	defer srv.Close()

	var output bytes.Buffer
	ctx := newSheetsRawTestContext(t, srv, &output, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := output.String()

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

func TestSheetsRaw_SheetSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"Data", "'Data'"},
		{"A1", "'A1'"},
		{"O'Brien! Data", "'O''Brien! Data'"},
		{" Data ", "' Data '"},
	} {
		for _, grid := range []bool{false, true} {
			t.Run(tc.name+"/grid="+strconv.FormatBool(grid), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if got := r.URL.Query().Get("ranges"); got != tc.want {
						t.Errorf("ranges = %q, want %q", got, tc.want)
					}
					if got := r.URL.Query().Get("includeGridData") == "true"; got != grid {
						t.Errorf("includeGridData = %v, want %v", got, grid)
					}
					_ = json.NewEncoder(w).Encode(fullSheetResponse("s1"))
				}))
				defer srv.Close()
				var output bytes.Buffer
				ctx := newSheetsRawTestContext(t, srv, &output, io.Discard)
				args := []string{"s1", "--sheet", tc.name}
				if grid {
					args = append(args, "--include-grid-data")
				}
				if err := runKong(t, &SheetsRawCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if err := json.Unmarshal(output.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got["spreadsheetId"] != "s1" || got["properties"] == nil {
					t.Fatalf("raw response metadata lost: %v", got)
				}
			})
		}
	}
}

func TestSheetsRaw_IncludeGridDataFlag(t *testing.T) {
	hit := &sheetsRawHit{}
	srv := newSheetsRawTestServer(t, 0, fullSheetResponse("s1"), hit)
	defer srv.Close()

	var stderr bytes.Buffer
	ctx := newSheetsRawTestContext(t, srv, io.Discard, &stderr)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &SheetsRawCmd{}, []string{"s1", "--include-grid-data"}, ctx, flags); err != nil {
		t.Fatalf("run: %v", err)
	}

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

	ctx := newSheetsRawTestContext(t, srv, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}
	err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags)
	if err == nil {
		t.Fatalf("expected error on 500")
	}
}

func TestSheetsRaw_NotFound(t *testing.T) {
	srv := newSheetsRawTestServer(t, http.StatusNotFound, nil, nil)
	defer srv.Close()

	ctx := newSheetsRawTestContext(t, srv, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}
	err := runKong(t, &SheetsRawCmd{}, []string{"s1"}, ctx, flags)
	if err == nil {
		t.Fatalf("expected error on 404")
	}
}

func TestSheetsRaw_EmptyID(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&SheetsRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
