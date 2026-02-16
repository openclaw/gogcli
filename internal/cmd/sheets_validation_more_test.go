package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/ui"
)

func TestSheetsGet_ValidationAndNoData(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&SheetsGetCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing spreadsheetId error")
	}
	if err := (&SheetsGetCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing range error")
	}

	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		if strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B2",
				"values": []any{},
			})
			return
		}
		http.NotFound(w, r)
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

	cmd := &SheetsGetCmd{SpreadsheetID: "s1", Range: "Sheet1!A1:B2", MajorDimension: "ROWS", ValueRenderOption: "FORMATTED_VALUE"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("get: %v", err)
	}
}

func TestSheetsUpdateAppend_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&SheetsUpdateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing spreadsheetId error")
	}
	if err := (&SheetsUpdateCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing range error")
	}
	if err := (&SheetsUpdateCmd{SpreadsheetID: "s1", Range: "A1", ValuesJSON: "nope"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update invalid json error")
	}
	if err := (&SheetsUpdateCmd{SpreadsheetID: "s1", Range: "A1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected update missing values error")
	}

	if err := (&SheetsAppendCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected append missing spreadsheetId error")
	}
	if err := (&SheetsAppendCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected append missing range error")
	}
	if err := (&SheetsAppendCmd{SpreadsheetID: "s1", Range: "A1", ValuesJSON: "nope"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected append invalid json error")
	}
	if err := (&SheetsAppendCmd{SpreadsheetID: "s1", Range: "A1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected append missing values error")
	}
}

func TestSheetsUpdateCopyValidationMissingRange(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		if strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedRange": "",
				"updatedCells": 1,
			})
			return
		}
		http.NotFound(w, r)
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

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &SheetsUpdateCmd{ValueInput: ""}
	if err := runKong(t, cmd, []string{"s1", "Sheet1!A1", "--values-json", `[["a"]]`, "--copy-validation-from", "Sheet1!A2:A2"}, ctx, flags); err == nil {
		t.Fatalf("expected missing updated range error")
	}
}

func TestSheetsAppendCopyValidationMissingRange(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		if strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
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

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &SheetsAppendCmd{Insert: "INSERT_ROWS", ValueInput: ""}
	if err := runKong(t, cmd, []string{"s1", "Sheet1!A1", "--values-json", `[["a"]]`, "--copy-validation-from", "Sheet1!A2:A2"}, ctx, flags); err == nil {
		t.Fatalf("expected missing updated range error")
	}
}

func TestSheetsClearMetadataCreate_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&SheetsClearCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected clear missing spreadsheetId error")
	}
	if err := (&SheetsClearCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected clear missing range error")
	}
	if err := (&SheetsMetadataCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected metadata missing spreadsheetId error")
	}
	if err := (&SheetsLinksCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected links missing spreadsheetId error")
	}
	if err := (&SheetsLinksCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected links missing range error")
	}
	if err := (&SheetsCreateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected create missing title error")
	}
}

func TestSheetsFormat_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&SheetsFormatCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format missing spreadsheetId error")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format missing range error")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "Sheet1!A1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format missing format-json error")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "Sheet1!A1", FormatJSON: "{\"textFormat\":{\"bold\":true}}"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format missing format-fields error")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "Sheet1!A1", FormatJSON: "nope", FormatFields: "userEnteredFormat.textFormat.bold"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format invalid json error")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "Sheet1!A1", FormatJSON: "{\"boarders\":{\"top\":{\"style\":\"SOLID\"}}}", FormatFields: "borders.top.style"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format unknown field error for boarders json typo")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "Sheet1!A1", FormatJSON: "{\"borders\":{\"top\":{\"style\":\"SOLID\"}}}", FormatFields: "boarders.top.style"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format typo error for boarders field mask")
	}
	if err := (&SheetsFormatCmd{SpreadsheetID: "s1", Range: "A1:B2", FormatJSON: "{\"textFormat\":{\"bold\":true}}", FormatFields: "userEnteredFormat.textFormat.bold"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected format missing sheet name error")
	}
}

func TestParseSheetRangeAndGridRange(t *testing.T) {
	if _, err := parseSheetRange("A1:B2", "format"); err == nil {
		t.Fatalf("expected missing sheet name error")
	}

	r, err := parseSheetRange("Sheet1!B2:C3", "format")
	if err != nil {
		t.Fatalf("parseSheetRange: %v", err)
	}

	grid, err := gridRangeFromMap(r, map[string]int64{"Sheet1": 9}, "format")
	if err != nil {
		t.Fatalf("gridRangeFromMap: %v", err)
	}
	if grid.SheetId != 9 {
		t.Fatalf("unexpected sheet id: %d", grid.SheetId)
	}
	if !hasStringValue(grid.ForceSendFields, "SheetId") {
		t.Fatalf("expected SheetId in ForceSendFields, got %#v", grid.ForceSendFields)
	}

	zero := toGridRange(a1Range{
		SheetName: "Sheet1",
		StartRow:  1,
		EndRow:    2,
		StartCol:  1,
		EndCol:    2,
	}, 0)
	if zero.SheetId != 0 {
		t.Fatalf("expected sheet id 0, got %d", zero.SheetId)
	}
	if !hasStringValue(zero.ForceSendFields, "SheetId") {
		t.Fatalf("expected SheetId force-send for sheet id 0, got %#v", zero.ForceSendFields)
	}
}
