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
	queries := make([]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/connected1/values/") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":          "Extracts!B3:C6",
				"majorDimension": "ROWS",
				"values": [][]any{
					{"word", "word_count"},
					{"love", 2019},
					{"the", 33201},
					{"king", 1500},
				},
			})
		case strings.HasPrefix(path, "/spreadsheets/connected1") && r.Method == http.MethodGet:
			_, _ = w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &queries
}
