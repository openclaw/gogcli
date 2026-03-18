package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestResolveGridRangeWithCatalog_DoesNotMutateNamedRangeForceSendFields(t *testing.T) {
	origFS := make([]string, 1, 2)
	origFS[0] = "ExistingField"
	nr := &sheets.NamedRange{
		Name: "MyNamedRange",
		Range: &sheets.GridRange{
			SheetId:         0,
			StartRowIndex:   1,
			EndRowIndex:     2,
			ForceSendFields: origFS,
		},
	}
	catalog := &spreadsheetRangeCatalog{
		NamedRanges: []*sheets.NamedRange{nr},
	}

	gr, err := resolveGridRangeWithCatalog("MyNamedRange", catalog, "format")
	if err != nil {
		t.Fatalf("resolveGridRangeWithCatalog: %v", err)
	}
	if gr == nil {
		t.Fatalf("expected non-nil grid range")
	}

	// Returned range includes SheetId force-send.
	if !containsStringValue(gr.ForceSendFields, "SheetId") {
		t.Fatalf("expected SheetId in returned ForceSendFields: %#v", gr.ForceSendFields)
	}

	// Original named range remains untouched.
	if len(nr.Range.ForceSendFields) != 1 || nr.Range.ForceSendFields[0] != "ExistingField" {
		t.Fatalf("expected original ForceSendFields unchanged, got %#v", nr.Range.ForceSendFields)
	}
}

func TestResolveGridRangeWithCatalog_DedupsSheetIDForceSendField(t *testing.T) {
	nr := &sheets.NamedRange{
		Name: "MyNamedRange",
		Range: &sheets.GridRange{
			SheetId:         7,
			StartRowIndex:   1,
			EndRowIndex:     2,
			ForceSendFields: []string{"SheetId"},
		},
	}
	catalog := &spreadsheetRangeCatalog{
		NamedRanges: []*sheets.NamedRange{nr},
	}

	gr, err := resolveGridRangeWithCatalog("MyNamedRange", catalog, "format")
	if err != nil {
		t.Fatalf("resolveGridRangeWithCatalog: %v", err)
	}
	if gr == nil {
		t.Fatalf("expected non-nil grid range")
	}
	if countStringValue(gr.ForceSendFields, "SheetId") != 1 {
		t.Fatalf("expected SheetId once, got %#v", gr.ForceSendFields)
	}
}

func TestFetchSpreadsheetRangeCatalog_PreservesSpacedSheetTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spreadsheetId": "s1",
			"sheets": []map[string]any{
				{"properties": map[string]any{"sheetId": 42, "title": "  Sheet With Spaces  "}},
			},
		})
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

	catalog, err := fetchSpreadsheetRangeCatalog(context.Background(), svc, "s1")
	if err != nil {
		t.Fatalf("fetchSpreadsheetRangeCatalog: %v", err)
	}
	if catalog == nil {
		t.Fatalf("expected catalog")
	}

	title := "  Sheet With Spaces  "
	if catalog.SheetIDsByTitle[title] != 42 {
		t.Fatalf("expected exact title key %q mapped to 42, got map=%#v", title, catalog.SheetIDsByTitle)
	}
	if _, ok := catalog.SheetIDsByTitle["Sheet With Spaces"]; ok {
		t.Fatalf("did not expect trimmed title key, map=%#v", catalog.SheetIDsByTitle)
	}

	// And ensure quoted A1 names with spaces still resolve against the map.
	r, err := parseSheetRange("'  Sheet With Spaces  '!A1:B2", "format")
	if err != nil {
		t.Fatalf("parseSheetRange: %v", err)
	}
	gr, err := gridRangeFromMap(r, catalog.SheetIDsByTitle, "format")
	if err != nil {
		t.Fatalf("gridRangeFromMap: %v", err)
	}
	if gr.SheetId != 42 {
		t.Fatalf("unexpected sheet id: %d", gr.SheetId)
	}
}

func containsStringValue(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}

func countStringValue(in []string, want string) int {
	n := 0
	for _, s := range in {
		if s == want {
			n++
		}
	}
	return n
}
