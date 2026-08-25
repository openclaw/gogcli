package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestSheetsDataSourceDeleteRequiresExplicitConfirmation(t *testing.T) {
	var factoryCalls atomic.Int32
	result := executeWithTestRuntime(t, []string{
		"--no-input", "sheets", "datasource", "delete", "connected1", "ds-target",
	}, &app.Runtime{Services: app.Services{
		ConnectedSheetsWriter: func(context.Context, string) (*sheets.Service, error) {
			factoryCalls.Add(1)
			return nil, errors.New("writer must not run")
		},
	}})
	if result.err == nil || ExitCode(result.err) != 2 || factoryCalls.Load() != 0 {
		t.Fatalf("noninteractive deletion must refuse before authentication: err=%v calls=%d", result.err, factoryCalls.Load())
	}
	for _, want := range []string{"--force", "ds-target", "connected1", "DATA_SOURCE sheet", "unlink dependent", "extracts", "charts", "pivot tables"} {
		if !strings.Contains(result.err.Error(), want) {
			t.Errorf("confirmation omitted %q: %v", want, result.err)
		}
	}
}

func TestSheetsDataSourceDeleteTargetsOneSource(t *testing.T) {
	for _, responseBody := range []string{`{"replies":[{}]}`, `{}`} {
		t.Run(responseBody, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v4/spreadsheets/connected1:batchUpdate" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var body sheets.BatchUpdateSpreadsheetRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Requests) != 1 ||
					body.Requests[0] == nil || body.Requests[0].DeleteDataSource == nil ||
					body.Requests[0].DeleteDataSource.DataSourceId != "ds-target" {
					t.Errorf("unexpected destructive request: %#v (%v)", body.Requests, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(responseBody))
			}))
			defer srv.Close()
			result := executeWithConnectedSheetsWriter(t, []string{
				"--json", "--force", "--account", "services@openclaw.org",
				"sheets", "datasource", "delete", "https://docs.google.com/spreadsheets/d/connected1/edit", " ds-target ",
			}, newSheetsServiceFromServer(t, srv))
			if result.err != nil {
				t.Fatalf("delete source: %v", result.err)
			}
			var got struct {
				SpreadsheetID string `json:"spreadsheetId"`
				DataSourceID  string `json:"dataSourceId"`
				Deleted       bool   `json:"deleted"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatalf("decode deletion result: %v", err)
			}
			if got.SpreadsheetID != "connected1" || got.DataSourceID != "ds-target" ||
				!got.Deleted || requests.Load() != 1 {
				t.Fatalf("unexpected deletion result/attempts: %#v requests=%d", got, requests.Load())
			}
		})
	}
}

func TestSheetsDataSourceDeleteDryRunAvoidsConfirmationAndAuthentication(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--readonly", "--dry-run", "--no-input",
		"sheets", "datasource", "delete", "connected1", "ds-target",
	}, &app.Runtime{})
	if result.err != nil {
		t.Fatalf("offline deletion preview: %v", result.err)
	}
	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			DataSourceID            string                               `json:"data_source_id"`
			DeletesLinkedSheet      bool                                 `json:"deletes_linked_sheet"`
			UnlinksDependentObjects bool                                 `json:"unlinks_dependent_objects"`
			BatchUpdate             sheets.BatchUpdateSpreadsheetRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("decode deletion preview: %v", err)
	}
	if !got.DryRun || got.Op != "sheets.datasource.delete" || got.Request.DataSourceID != "ds-target" ||
		!got.Request.DeletesLinkedSheet || !got.Request.UnlinksDependentObjects ||
		len(got.Request.BatchUpdate.Requests) != 1 ||
		got.Request.BatchUpdate.Requests[0].DeleteDataSource.DataSourceId != "ds-target" {
		t.Fatalf("unexpected destructive preview: %#v", got)
	}
}

func TestSheetsDataSourceDeleteForceDoesNotBypassReadOnly(t *testing.T) {
	var factoryCalls atomic.Int32
	result := executeWithTestRuntime(t, []string{
		"--force", "--readonly", "sheets", "datasource", "delete", "connected1", "ds-target",
	}, &app.Runtime{Services: app.Services{
		ConnectedSheetsWriter: func(context.Context, string) (*sheets.Service, error) {
			factoryCalls.Add(1)
			return nil, errors.New("writer must not run")
		},
	}})
	if !errors.Is(result.err, googleapi.ErrReadOnly) || factoryCalls.Load() != 0 {
		t.Fatalf("force bypassed readonly guard: err=%v calls=%d", result.err, factoryCalls.Load())
	}
}

func TestSheetsDataSourceDeleteDoesNotRetryDestructiveRequest(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				http.Error(w, "temporary provider failure", code)
			}))
			defer srv.Close()
			client := srv.Client()
			client.Transport = googleapi.NewRetryTransport(client.Transport)
			svc := newGoogleTestServiceWithEndpoint(t, client, srv.URL+"/", sheets.NewService)
			result := executeWithConnectedSheetsWriter(t, []string{
				"--force", "--account", "services@openclaw.org", "sheets", "datasource", "delete", "connected1", "ds-target",
			}, svc)
			if result.err == nil || requests.Load() != 1 || strings.Contains(result.stdout, "deleted") {
				t.Fatalf("destructive request must never replay or report success: err=%v requests=%d output=%q",
					result.err, requests.Load(), result.stdout)
			}
		})
	}
}

func TestSheetsDataSourceDeleteExplainsMissingBigQueryScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"The request scopes are not sufficient for performing this operation. Please include bigquery.readonly scope.","status":"PERMISSION_DENIED"}}`))
	}))
	defer srv.Close()
	result := executeWithConnectedSheetsWriter(t, []string{
		"--force", "--account", "services@openclaw.org",
		"sheets", "datasource", "delete", "connected1", "ds-target",
	}, newSheetsServiceFromServer(t, srv))
	if result.err == nil || !strings.Contains(result.err.Error(), connectedSheetsBigQueryScope) ||
		!strings.Contains(result.err.Error(), "--drive-scope") || strings.Contains(result.stdout, "deleted") {
		t.Fatalf("missing scope must preserve recovery and never report success: err=%v output=%q", result.err, result.stdout)
	}
}

func TestSheetsDataSourceDeleteRejectsInvalidIDsBeforeConfirmation(t *testing.T) {
	for _, test := range []struct {
		name          string
		spreadsheetID string
		dataSourceID  string
		want          string
	}{
		{name: "spreadsheet", spreadsheetID: " ", dataSourceID: "ds-target", want: "empty spreadsheetId"},
		{name: "source", spreadsheetID: "connected1", dataSourceID: " ", want: "empty dataSourceId"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := executeWithTestRuntime(t, []string{
				"--no-input", "sheets", "datasource", "delete", test.spreadsheetID, test.dataSourceID,
			}, &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), test.want) || ExitCode(result.err) != 2 {
				t.Fatalf("invalid IDs must reject before confirmation/auth: %v", result.err)
			}
		})
	}
}
