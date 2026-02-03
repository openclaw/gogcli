package todrive

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

func TestWriterCreateAndWrite(t *testing.T) {
	var gotValues [][]interface{}

	sheetsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v4/spreadsheets":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "sheet1",
				"spreadsheetUrl": "https://sheet/1",
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/v4/spreadsheets/sheet1/values/Sheet1!A1"):
			var vr sheets.ValueRange
			_ = json.NewDecoder(r.Body).Decode(&vr)
			gotValues = vr.Values
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"updatedRange": "Sheet1!A1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer sheetsSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer driveSrv.Close()

	origSheets := newSheetsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSheetsService = origSheets
		newDriveService = origDrive
	})

	newSheetsService = func(ctx context.Context, _ string) (*sheets.Service, error) {
		return sheets.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithHTTPClient(sheetsSrv.Client()),
			option.WithEndpoint(sheetsSrv.URL+"/"),
		)
	}
	newDriveService = func(ctx context.Context, _ string) (*drive.Service, error) {
		return drive.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithHTTPClient(driveSrv.Client()),
			option.WithEndpoint(driveSrv.URL+"/"),
		)
	}

	writer, err := New(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := writer.Write(context.Background(), []string{"A", "B"}, [][]string{{"1", "2"}}, Options{SheetName: "Report"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.SpreadsheetID != "sheet1" {
		t.Fatalf("unexpected id: %s", res.SpreadsheetID)
	}
	if len(gotValues) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(gotValues))
	}
}
