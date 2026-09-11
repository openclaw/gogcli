package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestSheetsInsertCmd_FirstPositionSendsZeroes(t *testing.T) {
	for _, sheetID := range []int64{0, 7} {
		for _, dimension := range []string{"rows", "cols"} {
			t.Run(fmt.Sprintf("%s/sheet%d", dimension, sheetID), func(t *testing.T) {
				capture := &sheetsBatchUpdateCapture{}
				svc := newSheetsBatchUpdateTestService(t, map[string]any{
					"sheets": []map[string]any{{"properties": map[string]any{"sheetId": sheetID, "title": "Data"}}},
				}, capture)
				ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
				if err := runKong(t, &SheetsInsertCmd{}, []string{"s1", "Data", dimension, "1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
					t.Fatal(err)
				}
				requests := capture.Body["requests"].([]any)
				insert := requests[0].(map[string]any)["insertDimension"].(map[string]any)
				rng := insert["range"].(map[string]any)
				for key, want := range map[string]float64{"sheetId": float64(sheetID), "startIndex": 0, "endIndex": 1} {
					if got, ok := rng[key]; !ok || got != want {
						t.Errorf("expected %s=%v in serialized range, got %#v", key, want, rng)
					}
				}
			})
		}
	}
}

func TestSheetsInsertCmd(t *testing.T) {
	var gotInsert *sheets.InsertDimensionRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 7, "title": "Data"}},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].InsertDimension == nil {
				t.Fatalf("expected insertDimension request, got %#v", req.Requests)
			}
			gotInsert = req.Requests[0].InsertDimension
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	t.Run("insert rows before", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "rows", "2", "--count", "3",
		}, ctx, flags); err != nil {
			t.Fatalf("insert rows: %v", err)
		}

		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		if gotInsert.Range.SheetId != 7 {
			t.Fatalf("unexpected sheet id: %d", gotInsert.Range.SheetId)
		}
		if gotInsert.Range.Dimension != "ROWS" {
			t.Fatalf("unexpected dimension: %s", gotInsert.Range.Dimension)
		}
		// startRow=2 → startIndex=1, endIndex=1+3=4
		if gotInsert.Range.StartIndex != 1 {
			t.Fatalf("unexpected startIndex: %d, want 1", gotInsert.Range.StartIndex)
		}
		if gotInsert.Range.EndIndex != 4 {
			t.Fatalf("unexpected endIndex: %d, want 4", gotInsert.Range.EndIndex)
		}
		if gotInsert.InheritFromBefore {
			t.Fatal("expected inheritFromBefore=false")
		}
	})

	t.Run("insert rows after", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "rows", "2", "--count", "1", "--after",
		}, ctx, flags); err != nil {
			t.Fatalf("insert rows: %v", err)
		}

		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		// startRow=2 --after → startIndex=2, endIndex=3
		if gotInsert.Range.StartIndex != 2 {
			t.Fatalf("unexpected startIndex: %d, want 2", gotInsert.Range.StartIndex)
		}
		if gotInsert.Range.EndIndex != 3 {
			t.Fatalf("unexpected endIndex: %d, want 3", gotInsert.Range.EndIndex)
		}
		if !gotInsert.InheritFromBefore {
			t.Fatal("expected inheritFromBefore=true")
		}
	})

	t.Run("insert after inheriting from following dimension", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "rows", "2", "--count", "1", "--after", "--inherit-from-before=false",
		}, ctx, flags); err != nil {
			t.Fatalf("insert rows: %v", err)
		}
		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		// --after would default inheritFromBefore=true; the explicit flag overrides it
		// so the API inherits from the following adjacent row/column instead.
		if gotInsert.InheritFromBefore {
			t.Fatal("expected inheritFromBefore=false when --inherit-from-before=false overrides --after")
		}
	})

	t.Run("insert before with explicit inherit", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "rows", "2", "--count", "1", "--inherit-from-before",
		}, ctx, flags); err != nil {
			t.Fatalf("insert rows: %v", err)
		}
		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		// before-insert defaults inheritFromBefore=false; the explicit flag overrides it.
		if !gotInsert.InheritFromBefore {
			t.Fatal("expected inheritFromBefore=true when --inherit-from-before is set")
		}
	})

	t.Run("reject inherit from before at first row", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		err := runKong(t, cmd, []string{
			"s1", "Data", "rows", "1", "--inherit-from-before",
		}, ctx, flags)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "cannot inherit from the previous row") {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotInsert != nil {
			t.Fatal("did not expect API request")
		}
	})

	t.Run("insert cols before", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "cols", "3", "--count", "2",
		}, ctx, flags); err != nil {
			t.Fatalf("insert cols: %v", err)
		}

		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		if gotInsert.Range.Dimension != "COLUMNS" {
			t.Fatalf("unexpected dimension: %s", gotInsert.Range.Dimension)
		}
		// startCol=3 → startIndex=2, endIndex=2+2=4
		if gotInsert.Range.StartIndex != 2 {
			t.Fatalf("unexpected startIndex: %d, want 2", gotInsert.Range.StartIndex)
		}
		if gotInsert.Range.EndIndex != 4 {
			t.Fatalf("unexpected endIndex: %d, want 4", gotInsert.Range.EndIndex)
		}
	})

	t.Run("insert cols after", func(t *testing.T) {
		gotInsert = nil
		cmd := &SheetsInsertCmd{}
		if err := runKong(t, cmd, []string{
			"s1", "Data", "cols", "3", "--count", "1", "--after",
		}, ctx, flags); err != nil {
			t.Fatalf("insert cols: %v", err)
		}

		if gotInsert == nil {
			t.Fatal("expected insertDimension request")
		}
		// startCol=3 --after → startIndex=3, endIndex=4
		if gotInsert.Range.StartIndex != 3 {
			t.Fatalf("unexpected startIndex: %d, want 3", gotInsert.Range.StartIndex)
		}
		if gotInsert.Range.EndIndex != 4 {
			t.Fatalf("unexpected endIndex: %d, want 4", gotInsert.Range.EndIndex)
		}
		if !gotInsert.InheritFromBefore {
			t.Fatal("expected inheritFromBefore=true")
		}
	})

	t.Run("invalid dimension", func(t *testing.T) {
		cmd := &SheetsInsertCmd{}
		err := runKong(t, cmd, []string{
			"s1", "Data", "sheets", "1",
		}, ctx, flags)
		if err == nil {
			t.Fatal("expected error for invalid dimension")
		}
		if !strings.Contains(err.Error(), "dimension must be rows or cols") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
