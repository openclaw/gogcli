package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestSheetsFilterSetBuildsSetBasicFilterRequest(t *testing.T) {
	var gotPath string
	var gotRequestBody string
	var gotRequest sheets.BatchUpdateSpreadsheetRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{{
					"properties": map[string]any{
						"sheetId": 0,
						"title":   "Sheet1",
						"gridProperties": map[string]any{
							"rowCount":    1000,
							"columnCount": 26,
						},
					},
				}},
				"namedRanges": []map[string]any{{
					"namedRangeId": "nr1",
					"name":         "NamedFilterRange",
					"range": map[string]any{
						"sheetId":          0,
						"startRowIndex":    2,
						"endRowIndex":      8,
						"startColumnIndex": 1,
						"endColumnIndex":   4,
					},
				}},
			})
		case path == "/spreadsheets/s1:batchUpdate" && r.Method == http.MethodPost:
			gotPath = path
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batchUpdate: %v", err)
			}
			gotRequestBody = string(body)
			if err := json.Unmarshal(body, &gotRequest); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"spreadsheetId":"s1","replies":[{}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	var out bytes.Buffer
	ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &SheetsFilterSetCmd{}, []string{"s1", "NamedFilterRange"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("filter set: %v", err)
	}

	if gotPath != "/spreadsheets/s1:batchUpdate" {
		t.Fatalf("batchUpdate path = %q", gotPath)
	}
	if len(gotRequest.Requests) != 1 || gotRequest.Requests[0].SetBasicFilter == nil {
		t.Fatalf("missing setBasicFilter request: %#v", gotRequest.Requests)
	}
	filter := gotRequest.Requests[0].SetBasicFilter.Filter
	if filter == nil || filter.Range == nil {
		t.Fatalf("missing basic filter range: %#v", gotRequest.Requests[0].SetBasicFilter)
	}
	if filter.Range.SheetId != 0 ||
		filter.Range.StartRowIndex != 2 || filter.Range.EndRowIndex != 8 ||
		filter.Range.StartColumnIndex != 1 || filter.Range.EndColumnIndex != 4 {
		t.Fatalf("range = %#v", filter.Range)
	}
	if !strings.Contains(gotRequestBody, `"setBasicFilter"`) || !strings.Contains(gotRequestBody, `"sheetId":0`) {
		t.Fatalf("request should include setBasicFilter and force-sent sheetId: %s", gotRequestBody)
	}
	if !strings.Contains(out.String(), `"spreadsheetId": "s1"`) || !strings.Contains(out.String(), `"NamedFilterRange"`) {
		t.Fatalf("unexpected JSON output: %s", out.String())
	}
}

func TestSheetsFilterSetDryRunSkipsService(t *testing.T) {
	var out bytes.Buffer
	ctx := withSheetsTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("Sheets service should not be called during dry-run")
		return nil, errors.New("unexpected sheets service call")
	})

	err := runKong(t, &SheetsFilterSetCmd{}, []string{"s1", "Sheet1!A1:C5"}, ctx, &RootFlags{DryRun: true, NoInput: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("dry-run filter set: %v", err)
	}

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			SpreadsheetID string `json:"spreadsheet_id"`
			Range         string `json:"range"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run: %v\nout=%s", err, out.String())
	}
	if !payload.DryRun || payload.Op != "sheets.filter.set" ||
		payload.Request.SpreadsheetID != "s1" || payload.Request.Range != "Sheet1!A1:C5" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
}

func TestSheetsFilterSetRequiresConfirmationToReplaceExistingFilter(t *testing.T) {
	var batchUpdates int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case path == "/spreadsheets/s1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"spreadsheetId":"s1","sheets":[{"properties":{"sheetId":0,"title":"Sheet1","gridProperties":{"rowCount":1000,"columnCount":26}},"basicFilter":{"range":{"sheetId":0,"startRowIndex":0,"endRowIndex":10,"startColumnIndex":0,"endColumnIndex":3}}}]}`))
		case path == "/spreadsheets/s1:batchUpdate" && r.Method == http.MethodPost:
			batchUpdates++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"spreadsheetId":"s1","replies":[{}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &SheetsFilterSetCmd{}, []string{"s1", "Sheet1!A1:D20"}, ctx, &RootFlags{Account: "a@b.com", NoInput: true})
	if err == nil || !strings.Contains(err.Error(), "replace existing basic filter") {
		t.Fatalf("expected existing-filter refusal, got %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
	if batchUpdates != 0 {
		t.Fatalf("batch updates = %d, want 0", batchUpdates)
	}

	var out bytes.Buffer
	ctx = withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &SheetsFilterSetCmd{}, []string{"s1", "Sheet1!A1:D20"}, ctx, &RootFlags{Account: "a@b.com", Force: true, NoInput: true}); err != nil {
		t.Fatalf("forced filter replacement: %v", err)
	}
	if batchUpdates != 1 {
		t.Fatalf("batch updates = %d, want 1", batchUpdates)
	}
	if !strings.Contains(out.String(), `"replaced": true`) {
		t.Fatalf("replacement output = %s", out.String())
	}
}
