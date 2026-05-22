package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestSheetsBatchUpdateCmd_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotReq sheets.BatchUpdateValuesRequest
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		gotPath = path
		switch {
		case path == "/spreadsheets/s1/values:batchUpdate" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode batchUpdate values: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":       "s1",
				"totalUpdatedRows":    2,
				"totalUpdatedColumns": 2,
				"totalUpdatedCells":   4,
				"totalUpdatedSheets":  1,
				"responses": []map[string]any{
					{
						"updatedRange":   "Sheet1!A1:B1",
						"updatedRows":    1,
						"updatedColumns": 2,
						"updatedCells":   2,
					},
					{
						"updatedRange":   "Sheet1!A2:B2",
						"updatedRows":    1,
						"updatedColumns": 2,
						"updatedCells":   2,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		err := runKong(t, &SheetsBatchUpdateCmd{}, []string{
			"s1",
			"--input", "RAW",
			"--include-values-in-response",
			"--response-render", "UNFORMATTED_VALUE",
			"--data-json", `[
				{"range":"Sheet1\\!A1:B1","values":[["a","b"]]},
				{"range":"Sheet1!A2:B2","values":[["c","d"]]}
			]`,
		}, newCmdJSONContext(t), &RootFlags{Account: "a@b.com"})
		if err != nil {
			t.Fatalf("batch update: %v", err)
		}
	})

	if gotPath != "/spreadsheets/s1/values:batchUpdate" {
		t.Fatalf("unexpected request path: %q", gotPath)
	}
	if gotReq.ValueInputOption != "RAW" {
		t.Fatalf("ValueInputOption = %q, want RAW", gotReq.ValueInputOption)
	}
	if !gotReq.IncludeValuesInResponse {
		t.Fatal("expected IncludeValuesInResponse")
	}
	if gotReq.ResponseValueRenderOption != "UNFORMATTED_VALUE" {
		t.Fatalf("ResponseValueRenderOption = %q", gotReq.ResponseValueRenderOption)
	}
	if len(gotReq.Data) != 2 {
		t.Fatalf("expected 2 value ranges, got %d", len(gotReq.Data))
	}
	if gotReq.Data[0].Range != "Sheet1!A1:B1" {
		t.Fatalf("range was not cleaned: %q", gotReq.Data[0].Range)
	}
	if got := gotReq.Data[1].Values[0][1]; got != "d" {
		t.Fatalf("unexpected value: %#v", got)
	}

	var payload struct {
		SpreadsheetID       string `json:"spreadsheetId"`
		TotalUpdatedRows    int64  `json:"totalUpdatedRows"`
		TotalUpdatedColumns int64  `json:"totalUpdatedColumns"`
		TotalUpdatedCells   int64  `json:"totalUpdatedCells"`
		TotalUpdatedSheets  int64  `json:"totalUpdatedSheets"`
		Responses           []struct {
			UpdatedRange string `json:"updatedRange"`
		} `json:"responses"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v\nout=%s", err, out)
	}
	if payload.SpreadsheetID != "s1" || payload.TotalUpdatedCells != 4 || len(payload.Responses) != 2 {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestSheetsBatchUpdateCmd_DryRunSkipsService(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	newSheetsService = func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("newSheetsService should not be called during dry-run")
		return nil, errors.New("unexpected sheets service call")
	}

	out := captureStdout(t, func() {
		err := runKong(t, &SheetsBatchUpdateCmd{}, []string{
			"s1",
			"--data-json", `[{"range":"Sheet1!A1","values":[["a"]]}]`,
		}, newCmdJSONContext(t), &RootFlags{DryRun: true, NoInput: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("dry-run batch update: %v", err)
		}
	})

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			SpreadsheetID string `json:"spreadsheet_id"`
			Data          []struct {
				Range  string          `json:"range"`
				Values [][]interface{} `json:"values"`
			} `json:"data"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode dry-run: %v\nout=%s", err, out)
	}
	if !payload.DryRun || payload.Op != "sheets.batch-update" || payload.Request.SpreadsheetID != "s1" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if len(payload.Request.Data) != 1 || payload.Request.Data[0].Range != "Sheet1!A1" {
		t.Fatalf("unexpected dry-run data: %#v", payload.Request.Data)
	}
}

func TestParseSheetsBatchUpdateDataRejectsInvalidPayloads(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "empty array", in: `[]`, want: "at least one value range"},
		{name: "null range", in: `[null]`, want: "range 0 is null"},
		{name: "empty range", in: `[{"range":"","values":[["a"]]}]`, want: "empty range"},
		{name: "missing values", in: `[{"range":"Sheet1!A1"}]`, want: "empty values"},
		{name: "empty values", in: `[{"range":"Sheet1!A1","values":[]}]`, want: "empty values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSheetsBatchUpdateData(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
