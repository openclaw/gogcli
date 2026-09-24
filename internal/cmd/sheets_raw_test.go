package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/outfmt"
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
	previous := sheetsRawBaseURL
	sheetsRawBaseURL = srv.URL + "/v4"
	t.Cleanup(func() { sheetsRawBaseURL = previous })
	ctx := newCmdRuntimeOutputContext(t, stdout, stderr)
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.SheetsHTTP = func(context.Context, string) (*http.Client, error) {
		return srv.Client(), nil
	}
	return app.WithRuntime(ctx, runtime)
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

func TestSheetsRaw_PreservesResponseValues(t *testing.T) {
	const payload = `{"spreadsheetId":"s1","properties":{"title":"Raw title"},"sheets":[{"properties":{"sheetId":0,"index":0,"hidden":false,"rightToLeft":false,"title":""},"data":[{"startRow":0,"startColumn":0,"rowData":[]}],"futureField":{"flag":false,"value":null}}],"namedRanges":[],"futureRoot":{"integer":9007199254740993,"value":null,"text":""}}`
	for _, pretty := range []bool{false, true} {
		t.Run(strconv.FormatBool(pretty), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, payload)
			}))
			defer server.Close()
			var output bytes.Buffer
			ctx := newSheetsRawTestContext(t, server, &output, io.Discard)
			ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{Select: []string{"spreadsheetId"}})
			if err := (&SheetsRawCmd{SpreadsheetID: "s1", Pretty: pretty}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			decode := func(raw string) any {
				t.Helper()
				var value any
				decoder := json.NewDecoder(strings.NewReader(raw))
				decoder.UseNumber()
				if err := decoder.Decode(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			if !reflect.DeepEqual(decode(output.String()), decode(payload)) {
				t.Fatalf("raw response changed:\n%s", output.String())
			}
			if !strings.HasSuffix(output.String(), "\n") || (!pretty && strings.Count(output.String(), "\n") != 1) {
				t.Fatalf("unexpected raw formatting: %q", output.String())
			}
		})
	}
}

func TestSheetsRaw_WrappingPreservesStructuredValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"spreadsheetId":"s1","properties":{"title":"Raw title"},"sheets":[{"properties":{"sheetId":0,"hidden":false},"data":[{"rowData":[{"values":[{"note":"Raw note"}]}]}]}],"future":{"integer":9007199254740993,"nullable":null}}`)
	}))
	defer server.Close()
	var output bytes.Buffer
	ctx := newSheetsRawTestContext(t, server, &output, io.Discard)
	ctx = outfmt.WithUntrustedWrapper(ctx, outfmt.UntrustedWrapOptions{Enabled: true})
	if err := (&SheetsRawCmd{SpreadsheetID: "s1"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		SpreadsheetID string                 `json:"spreadsheetId"`
		Properties    struct{ Title string } `json:"properties"`
		Sheets        []struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"sheets"`
		Future map[string]json.RawMessage `json:"future"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SpreadsheetID != "s1" || !strings.Contains(got.Properties.Title, "EXTERNAL_UNTRUSTED_CONTENT") || !strings.Contains(got.Properties.Title, "Raw title") {
		t.Fatalf("unexpected wrapped identity/title: %s", output.String())
	}
	if len(got.Sheets) != 1 || string(got.Sheets[0].Properties["sheetId"]) != "0" || string(got.Sheets[0].Properties["hidden"]) != "false" || string(got.Future["integer"]) != "9007199254740993" || string(got.Future["nullable"]) != "null" {
		t.Fatalf("structured values changed: %s", output.String())
	}
}

func TestSheetsRaw_InvalidResponseHasNoOutput(t *testing.T) {
	for _, body := range []string{"null", "[]", "true", "not json", "{} {}"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			var output bytes.Buffer
			ctx := newSheetsRawTestContext(t, server, &output, io.Discard)
			if err := (&SheetsRawCmd{SpreadsheetID: "s1"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil || output.Len() != 0 {
				t.Fatalf("error = %v, output = %q", err, output.String())
			}
		})
	}
}

func TestSheetsRaw_PreservesHTTPFailureCodes(t *testing.T) {
	for code, want := range map[int]int{403: 6, 404: 5, 429: 7, 503: 8} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			server := newSheetsRawTestServer(t, code, nil, nil)
			defer server.Close()
			var output bytes.Buffer
			ctx := newSheetsRawTestContext(t, server, &output, io.Discard)
			err := (&SheetsRawCmd{SpreadsheetID: "s1"}).Run(ctx, &RootFlags{Account: "a@b.com"})
			if got := ExitCode(stableExitCode(err)); got != want || output.Len() != 0 {
				t.Fatalf("exit = %d, want %d; error = %v, output = %q", got, want, err, output.String())
			}
		})
	}
}

func TestSheetsRaw_DeveloperMetadataWarning(t *testing.T) {
	for _, metadata := range []string{"null", "[]", `[{"metadataId":0,"metadataKey":"synthetic"}]`} {
		t.Run(metadata, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"spreadsheetId":"s1","developerMetadata":`+metadata+`}`)
			}))
			defer server.Close()
			var output, warnings bytes.Buffer
			ctx := newSheetsRawTestContext(t, server, &output, &warnings)
			if err := (&SheetsRawCmd{SpreadsheetID: "s1"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			if want := strings.HasPrefix(metadata, "[{"); strings.Contains(warnings.String(), "developerMetadata") != want {
				t.Fatalf("warning = %q, expected warning = %v", warnings.String(), want)
			}
		})
	}
}

func TestSheetsRaw_RefusesRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/redirected" {
			t.Error("followed authenticated redirect")
			_, _ = io.WriteString(w, `{"spreadsheetId":"s1"}`)
			return
		}
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	var output bytes.Buffer
	ctx := newSheetsRawTestContext(t, server, &output, io.Discard)
	if err := (&SheetsRawCmd{SpreadsheetID: "s1"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil || calls.Load() != 1 || output.Len() != 0 {
		t.Fatalf("error = %v, requests = %d, output = %q", err, calls.Load(), output.String())
	}
}
