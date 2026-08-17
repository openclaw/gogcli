package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestSheetsDataSourceListAndDescribe(t *testing.T) {
	srv, queries := newConnectedSheetsFixtureServer(t)
	defer srv.Close()
	svc := newSheetsServiceFromServer(t, srv)

	listResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "list", "connected1",
	}, svc)
	if listResult.err != nil {
		t.Fatalf("list data sources: %v", listResult.err)
	}
	var list struct {
		SpreadsheetID string `json:"spreadsheetId"`
		DataSources   []struct {
			DataSourceID string `json:"dataSourceId"`
			SheetID      int64  `json:"sheetId"`
			SheetTitle   string `json:"sheetTitle"`
			Provider     string `json:"provider"`
			Source       string `json:"source"`
			State        string `json:"state"`
			ErrorCode    string `json:"errorCode"`
		} `json:"dataSources"`
	}
	if err := json.Unmarshal([]byte(listResult.stdout), &list); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, listResult.stdout)
	}
	if list.SpreadsheetID != "connected1" || len(list.DataSources) != 2 {
		t.Fatalf("unexpected list: %#v", list)
	}
	if list.DataSources[0].DataSourceID != "ds-query" || list.DataSources[0].State != "FAILED" || list.DataSources[0].ErrorCode != "ENGINE" {
		t.Fatalf("query data source summary = %#v", list.DataSources[0])
	}
	if list.DataSources[1].Provider != "BIGQUERY" || list.DataSources[1].Source != "bigquery-public-data.samples.shakespeare" {
		t.Fatalf("table data source summary = %#v", list.DataSources[1])
	}
	// Live responses omit Spreadsheet.dataSources[].sheetId, so both entries must
	// resolve their linked sheet by data source id rather than latching onto the
	// unrelated tab that happens to have sheet id 0.
	if list.DataSources[0].SheetID != 102 || list.DataSources[0].SheetTitle != "Query Preview" {
		t.Fatalf("query data source sheet identity = %#v", list.DataSources[0])
	}
	if list.DataSources[1].SheetID != 101 || list.DataSources[1].SheetTitle != "Shakespeare Preview" {
		t.Fatalf("table data source sheet identity = %#v", list.DataSources[1])
	}
	if strings.Contains(listResult.stdout, "SELECT corpus") {
		t.Fatalf("list output should not expose raw query text: %s", listResult.stdout)
	}

	describeResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "describe", "connected1", "ds-query",
	}, svc)
	if describeResult.err != nil {
		t.Fatalf("describe data source: %v", describeResult.err)
	}
	if !strings.Contains(describeResult.stdout, `"rawQuery": "SELECT corpus`) ||
		!strings.Contains(describeResult.stdout, `"sheetType": "DATA_SOURCE"`) ||
		!strings.Contains(describeResult.stdout, `"dataSourceSchedules"`) {
		t.Fatalf("describe output missing full source/sheet/schedule detail: %s", describeResult.stdout)
	}
	if len(*queries) < 2 || !strings.Contains((*queries)[0], "dataSources") || !strings.Contains((*queries)[0], "dataSourceSheetProperties") {
		t.Fatalf("unexpected field mask queries: %#v", *queries)
	}
}

func TestSheetsDataSourceTableListDescribeAndRead(t *testing.T) {
	srv, queries := newConnectedSheetsFixtureServer(t)
	defer srv.Close()
	svc := newSheetsServiceFromServer(t, srv)

	listResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "table", "list", "connected1", "--data-source-id", "ds-table",
	}, svc)
	if listResult.err != nil {
		t.Fatalf("list data-source tables: %v", listResult.err)
	}
	if !strings.Contains(listResult.stdout, `"anchor": "Extracts!B3"`) ||
		!strings.Contains(listResult.stdout, `"rowLimit": 5`) ||
		!strings.Contains(listResult.stdout, `"state": "SUCCEEDED"`) {
		t.Fatalf("unexpected table list: %s", listResult.stdout)
	}

	describeResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "table", "describe", "connected1", "Extracts!B3",
	}, svc)
	if describeResult.err != nil {
		t.Fatalf("describe data-source table: %v", describeResult.err)
	}
	if !strings.Contains(describeResult.stdout, `"columnSelectionType": "SELECTED"`) ||
		!strings.Contains(describeResult.stdout, `"dataSourceId": "ds-table"`) {
		t.Fatalf("unexpected table description: %s", describeResult.stdout)
	}

	readResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "table", "read", "connected1", "Extracts!B3", "--max-rows", "3",
	}, svc)
	if readResult.err != nil {
		t.Fatalf("read data-source table: %v", readResult.err)
	}
	var read struct {
		Anchor       string          `json:"anchor"`
		Range        string          `json:"range"`
		DataSourceID string          `json:"dataSourceId"`
		Truncated    bool            `json:"truncated"`
		Values       [][]interface{} `json:"values"`
	}
	if err := json.Unmarshal([]byte(readResult.stdout), &read); err != nil {
		t.Fatalf("decode read JSON: %v\n%s", err, readResult.stdout)
	}
	if read.Anchor != "Extracts!B3" || read.Range != "Extracts!B3:C6" || read.DataSourceID != "ds-table" || !read.Truncated || len(read.Values) != 4 {
		t.Fatalf("unexpected table read: %#v", read)
	}

	joinedQueries := strings.Join(*queries, "\n")
	if !strings.Contains(joinedQueries, "includeGridData=true") || !strings.Contains(joinedQueries, "dataSourceTable") {
		t.Fatalf("table discovery did not request anchor definitions: %s", joinedQueries)
	}
	// A SELECTED table lists its own columns, so it must not pay for the
	// unranged column lookup.
	if got := countUnrangedColumnLookups(*queries); got != 0 {
		t.Fatalf("unranged column lookups = %d, want 0: %#v", got, *queries)
	}
}

// countUnrangedColumnLookups counts spreadsheets.get calls that carry neither
// grid data nor value-range parameters, which is the shape of the unranged
// column lookup used for SYNC_ALL extracts.
func countUnrangedColumnLookups(queries []string) int {
	count := 0
	for _, query := range queries {
		if strings.Contains(query, "includeGridData") || strings.Contains(query, "majorDimension") {
			continue
		}
		if strings.Contains(query, "dataSourceSheetProperties") {
			count++
		}
	}

	return count
}

func TestSheetsDataSourceTableReadSyncAllExtract(t *testing.T) {
	srv, queries := newConnectedSheetsFixtureServer(t)
	defer srv.Close()
	svc := newSheetsServiceFromServer(t, srv)

	// A SYNC_ALL extract carries no inline column list, and the ranged
	// spreadsheets.get that locates the anchor omits the DATA_SOURCE sheet that
	// does, so the column count has to be recovered from an unranged read.
	readResult := executeWithSheetsTestService(t, []string{
		"--json", "--account", "services@openclaw.org",
		"sheets", "datasource", "table", "read", "connected1", "Synced Extract!A1", "--max-rows", "3",
	}, svc)
	if readResult.err != nil {
		t.Fatalf("read SYNC_ALL data-source table: %v", readResult.err)
	}
	var read struct {
		Anchor       string `json:"anchor"`
		Range        string `json:"range"`
		DataSourceID string `json:"dataSourceId"`
		Truncated    bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(readResult.stdout), &read); err != nil {
		t.Fatalf("decode read JSON: %v\n%s", err, readResult.stdout)
	}
	// ds-query exposes two columns on its DATA_SOURCE sheet, so three data rows
	// plus the header span A1:B4.
	if read.Anchor != "'Synced Extract'!A1" || read.Range != "'Synced Extract'!A1:B4" {
		t.Fatalf("unexpected SYNC_ALL read bounds: %#v", read)
	}
	if read.DataSourceID != "ds-query" || !read.Truncated {
		t.Fatalf("unexpected SYNC_ALL read identity: %#v", read)
	}
	// Exactly one extra lookup: the ranged anchor fetch cannot supply the columns,
	// and repeating it per read would multiply the request cost.
	if got := countUnrangedColumnLookups(*queries); got != 1 {
		t.Fatalf("unranged column lookups = %d, want 1: %#v", got, *queries)
	}
}

func TestFindSheetsDataSourceSheet(t *testing.T) {
	decoy := &sheets.Sheet{Properties: &sheets.SheetProperties{SheetId: 0, Title: "Unrelated First Tab"}}
	linked := &sheets.Sheet{Properties: &sheets.SheetProperties{
		SheetId:                   777,
		Title:                     "Linked",
		DataSourceSheetProperties: &sheets.DataSourceSheetProperties{DataSourceId: "ds-1"},
	}}
	byIDOnly := &sheets.Sheet{Properties: &sheets.SheetProperties{SheetId: 777, Title: "By Sheet Id"}}

	for _, test := range []struct {
		name      string
		allSheets []*sheets.Sheet
		source    *sheets.DataSource
		want      *sheets.Sheet
	}{{
		// Live responses omit dataSources[].sheetId, so the zero value must not
		// win against a sheet that actually declares the data source.
		name:      "data source id wins over a sheet with id 0",
		allSheets: []*sheets.Sheet{decoy, linked},
		source:    &sheets.DataSource{DataSourceId: "ds-1"},
		want:      linked,
	}, {
		name:      "falls back to a supplied sheet id",
		allSheets: []*sheets.Sheet{decoy, byIDOnly},
		source:    &sheets.DataSource{DataSourceId: "ds-1", SheetId: 777},
		want:      byIDOnly,
	}, {
		name:      "no match without a usable sheet id",
		allSheets: []*sheets.Sheet{decoy, byIDOnly},
		source:    &sheets.DataSource{DataSourceId: "ds-1"},
		want:      nil,
	}} {
		t.Run(test.name, func(t *testing.T) {
			if got := findSheetsDataSourceSheet(test.allSheets, test.source); got != test.want {
				t.Fatalf("findSheetsDataSourceSheet = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSheetsDataSourceTableValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		anchor string
		want   string
	}{
		{name: "missing sheet", anchor: "A1", want: "include a sheet name"},
		{name: "range", anchor: "Extracts!A1:B2", want: "one cell"},
		{name: "invalid", anchor: "Extracts!nope", want: "invalid anchor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateSheetsDataSourceTableArgs("connected1", test.anchor)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if ExitCode(err) != 2 {
				t.Fatalf("ExitCode = %d, want 2", ExitCode(err))
			}
		})
	}
}

func TestWrapConnectedSheetsReadError(t *testing.T) {
	cause := errors.New("Request had insufficient authentication scopes")
	err := wrapConnectedSheetsReadError(cause, "services@openclaw.org")
	if err == nil || !strings.Contains(err.Error(), connectedSheetsBigQueryScope) || !strings.Contains(err.Error(), "--extra-scopes") {
		t.Fatalf("missing reauthorization guidance: %v", err)
	}
	plain := errors.New("permission denied for BigQuery table")
	if got := wrapConnectedSheetsReadError(plain, "services@openclaw.org"); !errors.Is(got, plain) {
		t.Fatalf("ordinary permission error should be preserved: %v", got)
	}
}

func TestSheetsMetadataIncludesConnectedSheets(t *testing.T) {
	srv, _ := newConnectedSheetsFixtureServer(t)
	defer srv.Close()
	svc := newSheetsServiceFromServer(t, srv)
	var out bytes.Buffer
	ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if err := (&SheetsMetadataCmd{SpreadsheetID: "connected1"}).Run(ctx, &RootFlags{Account: "services@openclaw.org"}); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if !strings.Contains(out.String(), `"dataSources"`) || !strings.Contains(out.String(), `"dataSourceSchedules"`) {
		t.Fatalf("metadata omitted Connected Sheets fields: %s", out.String())
	}
}

func newConnectedSheetsFixtureServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/sheets_connected_sheets.json")
	if err != nil {
		t.Fatalf("read Connected Sheets fixture: %v", err)
	}
	// Decode here rather than per request: an httptest handler runs on its own
	// goroutine, where t.Fatalf is not allowed.
	var parsed map[string]any
	if err := json.Unmarshal(fixture, &parsed); err != nil {
		t.Fatalf("decode Connected Sheets fixture: %v", err)
	}
	queries := make([]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/connected1/values/") && r.Method == http.MethodGet:
			// net/http already decoded r.URL.Path, so the trimmed remainder is
			// the requested A1 range.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":          strings.TrimPrefix(path, "/spreadsheets/connected1/values/"),
				"majorDimension": "ROWS",
				"values": [][]any{
					{"word", "word_count"},
					{"love", 2019},
					{"the", 33201},
					{"king", 1500},
				},
			})
		case strings.HasPrefix(path, "/spreadsheets/connected1") && r.Method == http.MethodGet:
			body, err := connectedSheetsFixtureForRanges(parsed, fixture, r.URL.Query()["ranges"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &queries
}

// connectedSheetsFixtureForRanges mirrors a live spreadsheets.get: when ranges
// are supplied the response only carries the sheets those ranges intersect, so a
// SYNC_ALL extract's column list on the separate DATA_SOURCE sheet disappears.
func connectedSheetsFixtureForRanges(parsed map[string]any, full []byte, ranges []string) ([]byte, error) {
	if len(ranges) == 0 {
		return full, nil
	}
	titles := make(map[string]bool, len(ranges))
	for _, item := range ranges {
		title := item
		if idx := strings.LastIndex(item, "!"); idx >= 0 {
			title = item[:idx]
		}
		titles[strings.Trim(title, "'")] = true
	}

	sheetsValue, _ := parsed["sheets"].([]any)
	kept := make([]any, 0, len(sheetsValue))
	for _, sheet := range sheetsValue {
		entry, _ := sheet.(map[string]any)
		properties, _ := entry["properties"].(map[string]any)
		title, _ := properties["title"].(string)
		if titles[title] {
			kept = append(kept, sheet)
		}
	}

	// Copy rather than mutate: the parsed fixture is shared across requests.
	filtered := make(map[string]any, len(parsed))
	for key, value := range parsed {
		filtered[key] = value
	}
	filtered["sheets"] = kept

	return json.Marshal(filtered)
}
