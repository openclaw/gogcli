package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
)

type sheetsBatchUpdateCapture struct {
	Body    map[string]any
	Request sheets.BatchUpdateSpreadsheetRequest
	Last    *sheets.Request
}

func (c *sheetsBatchUpdateCapture) reset() {
	c.Body = nil
	c.Request = sheets.BatchUpdateSpreadsheetRequest{}
	c.Last = nil
}

func (c *sheetsBatchUpdateCapture) firstRequest(t *testing.T) *sheets.Request {
	t.Helper()
	if len(c.Request.Requests) != 1 {
		t.Fatalf("expected one batchUpdate request, got %#v", c.Request.Requests)
	}
	return c.Request.Requests[0]
}

func newSheetsBatchUpdateTestService(
	t *testing.T,
	spreadsheet map[string]any,
	capture *sheetsBatchUpdateCapture,
) *sheets.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(spreadsheet)
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&capture.Body); err != nil {
				t.Errorf("decode batchUpdate: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			body, err := json.Marshal(capture.Body)
			if err != nil {
				t.Errorf("marshal batchUpdate: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.Unmarshal(body, &capture.Request); err != nil {
				t.Errorf("decode typed batchUpdate: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(capture.Request.Requests) == 1 {
				capture.Last = capture.Request.Requests[0]
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})
	svc, closeServer := newGoogleTestService(t, handler, sheets.NewService)
	t.Cleanup(closeServer)
	return svc
}

func sheetsEmptyAnnotationsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{{"data": []map[string]any{{
				"rowData": []map[string]any{{"values": []map[string]any{{"formattedValue": "Name"}}}},
			}}}},
		})
	})
}

func sheetsAnnotationsHandler(rows [][]map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		if !strings.HasPrefix(path, "/spreadsheets/s1") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("includeGridData") != "true" {
			http.Error(w, "expected includeGridData=true", http.StatusBadRequest)
			return
		}

		startRow, startCol := 0, 0
		if strings.Contains(r.URL.Query().Get("ranges"), "B2") {
			startRow, startCol = 1, 1
		}
		rowData := make([]map[string]any, 0, len(rows))
		for _, values := range rows {
			rowData = append(rowData, map[string]any{"values": values})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{{
				"properties": map[string]any{"title": "Sheet1"},
				"data": []map[string]any{{
					"startRow": startRow, "startColumn": startCol, "rowData": rowData,
				}},
			}},
		})
	})
}

func assertSheetsNoAnnotations(
	t *testing.T,
	cmd any,
	args []string,
	newContext func(*testing.T, http.Handler, bool) (context.Context, *bytes.Buffer, *bytes.Buffer),
	want string,
) {
	t.Helper()
	ctx, _, stderr := newContext(t, sheetsEmptyAnnotationsHandler(), false)
	if err := runKong(t, cmd, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("read annotations: %v", err)
	}
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("expected %q on stderr: %q", want, stderr.String())
	}
}

func assertSheetsOffsetAnnotations(
	t *testing.T,
	cmd any,
	handler http.Handler,
	newContext func(*testing.T, http.Handler, bool) (context.Context, *bytes.Buffer, *bytes.Buffer),
	resultKey string,
) {
	t.Helper()
	ctx, output, _ := newContext(t, handler, true)
	if err := runKong(t, cmd, []string{"s1", "Sheet1!B2:C3"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("read annotations: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, output.String())
	}
	first := result[resultKey].([]any)[0].(map[string]any)
	if first["a1"] != "Sheet1!B2" || first["row"] != float64(2) || first["col"] != float64(2) {
		t.Errorf("unexpected offset annotation: %#v", first)
	}
}

func assertSheetsAnnotationsJSON(
	t *testing.T,
	cmd any,
	handler http.Handler,
	newContext func(*testing.T, http.Handler, bool) (context.Context, *bytes.Buffer, *bytes.Buffer),
	resultKey, field, wantField, wantValue string,
) {
	t.Helper()
	ctx, output, _ := newContext(t, handler, true)
	if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B3"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("read annotations: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v (output: %q)", err, output.String())
	}
	items, ok := result[resultKey].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("expected 3 %s, got %#v", resultKey, result[resultKey])
	}
	first := items[0].(map[string]any)
	if first["sheet"] != "Sheet1" || first["a1"] != "Sheet1!A1" || first["row"] != float64(1) || first["col"] != float64(1) {
		t.Errorf("unexpected annotation position: %#v", first)
	}
	if first[field] != wantField || first["value"] != wantValue {
		t.Errorf("unexpected annotation content: %#v", first)
	}
}

func newSheetsServiceFromServer(t *testing.T, srv *httptest.Server) *sheets.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", sheets.NewService)
}

func withSheetsTestService(ctx context.Context, svc *sheets.Service) context.Context {
	return withSheetsTestServiceFactory(ctx, func(context.Context, string) (*sheets.Service, error) {
		return svc, nil
	})
}

func withSheetsTestServiceFactory(ctx context.Context, factory app.SheetsServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Sheets = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithSheetsTestService(t *testing.T, args []string, svc *sheets.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Sheets: func(context.Context, string) (*sheets.Service, error) { return svc, nil },
	}})
}

func executeWithSheetsAndDriveTestServices(t *testing.T, args []string, sheetsSvc *sheets.Service, driveSvc *drive.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Drive:  stubDriveService(driveSvc),
		Sheets: func(context.Context, string) (*sheets.Service, error) { return sheetsSvc, nil },
	}})
}
