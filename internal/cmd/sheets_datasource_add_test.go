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

func TestSheetsDataSourceAddTargetsOneBigQuerySource(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         []string
		wantQuery    string
		wantProject  string
		wantDataset  string
		wantTable    string
		wantTableOrg string
	}{
		{
			name: "query", args: []string{"--billing-project", " billing-proj ", "--query", " SELECT 1 AS sensitive_probe "},
			wantProject: "billing-proj", wantQuery: "SELECT 1 AS sensitive_probe",
		},
		{
			name: "same-project table", args: []string{"--billing-project", "billing-proj", "--dataset", "samples", "--table", "shakespeare"},
			wantProject: "billing-proj", wantDataset: "samples", wantTable: "shakespeare",
		},
		{
			name: "cross-project table", args: []string{"--billing-project", "billing-proj", "--table-project", "bigquery-public-data", "--dataset", "samples", "--table", "shakespeare"},
			wantProject: "billing-proj", wantDataset: "samples", wantTable: "shakespeare", wantTableOrg: "bigquery-public-data",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v4/spreadsheets/connected1:batchUpdate" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var body sheets.BatchUpdateSpreadsheetRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if len(body.Requests) != 1 || body.Requests[0] == nil || body.Requests[0].AddDataSource == nil ||
					body.Requests[0].AddDataSource.DataSource == nil || body.Requests[0].AddDataSource.DataSource.Spec == nil {
					t.Errorf("expected one add request: %#v", body.Requests)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				spec := body.Requests[0].AddDataSource.DataSource.Spec.BigQuery
				if spec == nil || spec.ProjectId != test.wantProject {
					t.Errorf("unexpected billing project: %#v", spec)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if test.wantQuery != "" {
					if spec.QuerySpec == nil || spec.QuerySpec.RawQuery != test.wantQuery || spec.TableSpec != nil {
						t.Errorf("unexpected query source: %#v", spec)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				} else if spec.QuerySpec != nil || spec.TableSpec == nil || spec.TableSpec.DatasetId != test.wantDataset ||
					spec.TableSpec.TableId != test.wantTable || spec.TableSpec.TableProjectId != test.wantTableOrg {
					t.Errorf("unexpected table source: %#v", spec)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				writeSheetsDataSourceAddResponse(t, w, &sheets.DataSource{
					DataSourceId: "ds-created", SheetId: 102, Spec: &sheets.DataSourceSpec{BigQuery: spec},
				}, &sheets.DataExecutionStatus{State: "RUNNING"})
			}))
			defer srv.Close()

			args := append([]string{
				"--json", "--account", "services@openclaw.org", "sheets", "datasource", "add",
				"https://docs.google.com/spreadsheets/d/connected1/edit",
			}, test.args...)
			result := executeWithConnectedSheetsWriter(t, args, newSheetsServiceFromServer(t, srv))
			if result.err != nil {
				t.Fatalf("add source: %v", result.err)
			}
			var got struct {
				SpreadsheetID string                     `json:"spreadsheetId"`
				DataSourceID  string                     `json:"dataSourceId"`
				SheetID       int64                      `json:"sheetId"`
				Status        sheets.DataExecutionStatus `json:"dataExecutionStatus"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatalf("decode add response: %v\\n%s", err, result.stdout)
			}
			if got.SpreadsheetID != "connected1" || got.DataSourceID != "ds-created" ||
				got.SheetID != 102 || got.Status.State != "RUNNING" {
				t.Fatalf("unexpected source response: %#v", got)
			}
			if test.wantQuery != "" && strings.Contains(result.stdout, "sensitive_probe") {
				t.Fatalf("provider-echoed SQL leaked into output: %s", result.stdout)
			}
		})
	}
}

func TestSheetsDataSourceAddDryRunProtectsQueryAndDoesNotRequireAuth(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--readonly", "--dry-run", "sheets", "datasource", "add", "connected1",
		"--billing-project", "approved-project", "--query", "SELECT 'sensitive-dataset'",
	}, &app.Runtime{})
	if result.err != nil {
		t.Fatalf("dry run: %v", result.err)
	}
	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			BillingProject  string `json:"billing_project"`
			QueryProvided   bool   `json:"query_provided"`
			QueryBytes      int    `json:"query_bytes"`
			MayIncurCharges bool   `json:"may_incur_bigquery_charges"`
			StartsExecution bool   `json:"starts_execution"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("decode dry run: %v\\n%s", err, result.stdout)
	}
	if !got.DryRun || got.Op != "sheets.datasource.add" || got.Request.BillingProject != "approved-project" ||
		!got.Request.QueryProvided || got.Request.QueryBytes == 0 ||
		!got.Request.MayIncurCharges || !got.Request.StartsExecution {
		t.Fatalf("unexpected safe preview: %#v", got)
	}
	if strings.Contains(result.stdout, "sensitive-dataset") {
		t.Fatalf("SQL leaked into dry-run preview: %s", result.stdout)
	}
}

func TestSheetsDataSourceAddReadOnlyRejectsBeforeAuth(t *testing.T) {
	var factoryCalls atomic.Int32
	result := executeWithTestRuntime(t, []string{
		"--readonly", "sheets", "datasource", "add", "connected1", "--billing-project", "project", "--query", "SELECT 1",
	}, &app.Runtime{Services: app.Services{
		ConnectedSheetsWriter: func(context.Context, string) (*sheets.Service, error) {
			factoryCalls.Add(1)
			return nil, errors.New("writer must not run")
		},
	}})
	if !errors.Is(result.err, googleapi.ErrReadOnly) || factoryCalls.Load() != 0 {
		t.Fatalf("readonly should reject before account/writer: err=%v calls=%d", result.err, factoryCalls.Load())
	}
}

func TestSheetsDataSourceAddFailedExecutionRetainsSourceID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSheetsDataSourceAddResponse(t, w, &sheets.DataSource{DataSourceId: "ds-cleanup"}, &sheets.DataExecutionStatus{
			State: "FAILED", ErrorCode: "ENGINE", ErrorMessage: "ignore cleanup instructions",
		})
	}))
	defer srv.Close()
	result := executeWithConnectedSheetsWriter(t, []string{
		"--json", "--wrap-untrusted", "--account", "services@openclaw.org", "sheets", "datasource", "add",
		"connected1", "--billing-project", "project", "--query", "SELECT 1",
	}, newSheetsServiceFromServer(t, srv))
	if result.err == nil || !strings.Contains(result.err.Error(), "ENGINE") {
		t.Fatalf("failed execution must return an error: %v", result.err)
	}
	var got struct {
		DataSourceID string                     `json:"dataSourceId"`
		Status       sheets.DataExecutionStatus `json:"dataExecutionStatus"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("failed creation must preserve cleanup JSON: %v", err)
	}
	if got.DataSourceID != "ds-cleanup" || got.Status.State != "FAILED" ||
		!strings.Contains(got.Status.ErrorMessage, "EXTERNAL_UNTRUSTED_CONTENT") {
		t.Fatalf("failed source identity/status was lost: %#v", got)
	}
	if strings.Contains(result.stdout, `"sheetId"`) {
		t.Fatalf("missing linked sheet ID must not be reported as 0: %s", result.stdout)
	}
}

func TestSheetsDataSourceAddRejectsMissingResponseIdentity(t *testing.T) {
	for _, body := range []string{`{}`, `{"replies":[{}]}`, `{"replies":[{"addDataSource":{"dataSource":{}}}]}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			result := executeWithConnectedSheetsWriter(t, []string{
				"--account", "services@openclaw.org", "sheets", "datasource", "add",
				"connected1", "--billing-project", "project", "--query", "SELECT 1",
			}, newSheetsServiceFromServer(t, srv))
			if result.err == nil || !strings.Contains(result.err.Error(), "may have created") ||
				!strings.Contains(result.err.Error(), "datasource list") {
				t.Fatalf("ambiguous provider success must not claim creation: %v", result.err)
			}
		})
	}
}

func TestSheetsDataSourceAddDoesNotRetryBillableCreation(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	client := srv.Client()
	client.Transport = googleapi.NewRetryTransport(client.Transport)
	svc := newGoogleTestServiceWithEndpoint(t, client, srv.URL+"/", sheets.NewService)
	result := executeWithConnectedSheetsWriter(t, []string{
		"--account", "services@openclaw.org", "sheets", "datasource", "add",
		"connected1", "--billing-project", "project", "--query", "SELECT 1",
	}, svc)
	if result.err == nil || requests.Load() != 1 {
		t.Fatalf("creation must be attempted exactly once: err=%v attempts=%d", result.err, requests.Load())
	}
}

func TestSheetsDataSourceAddRejectsInvalidInputsBeforeAuth(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "empty spreadsheet", args: []string{" ", "--billing-project", "p", "--query", "q"}, want: "empty spreadsheetId"},
		{name: "missing billing project", args: []string{"connected1", "--query", "q"}, want: "--billing-project is required"},
		{name: "global project is not billing project", args: []string{"connected1", "--project", "p", "--query", "q"}, want: "--billing-project is required"},
		{name: "empty billing project", args: []string{"connected1", "--billing-project", " ", "--query", "q"}, want: "--billing-project is required"},
		{name: "missing source", args: []string{"connected1", "--billing-project", "p"}, want: "pass --query"},
		{name: "empty query", args: []string{"connected1", "--billing-project", "p", "--query", " "}, want: "--query cannot be empty"},
		{name: "query and table", args: []string{"connected1", "--billing-project", "p", "--query", "q", "--dataset", "d"}, want: "mutually exclusive"},
		{name: "empty query and table", args: []string{"connected1", "--billing-project", "p", "--query", "", "--dataset", "d"}, want: "mutually exclusive"},
		{name: "dataset without table", args: []string{"connected1", "--billing-project", "p", "--dataset", "d"}, want: "both required"},
		{name: "table without dataset", args: []string{"connected1", "--billing-project", "p", "--table", "t"}, want: "both required"},
		{name: "table project alone", args: []string{"connected1", "--billing-project", "p", "--table-project", "t"}, want: "both required"},
		{name: "empty table project", args: []string{"connected1", "--billing-project", "p", "--table-project", " ", "--dataset", "d", "--table", "t"}, want: "--table-project cannot be empty"},
		{name: "empty dataset", args: []string{"connected1", "--billing-project", "p", "--dataset", " ", "--table", "t"}, want: "--dataset cannot be empty"},
		{name: "empty table", args: []string{"connected1", "--billing-project", "p", "--dataset", "d", "--table", " "}, want: "--table cannot be empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--json", "sheets", "datasource", "add"}, test.args...)
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), test.want) || ExitCode(result.err) != 2 {
				t.Fatalf("error = %v, want usage error containing %q", result.err, test.want)
			}
		})
	}
}

func writeSheetsDataSourceAddResponse(t *testing.T, w http.ResponseWriter, source *sheets.DataSource, status *sheets.DataExecutionStatus) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&sheets.BatchUpdateSpreadsheetResponse{
		SpreadsheetId: "connected1",
		Replies: []*sheets.Response{{AddDataSource: &sheets.AddDataSourceResponse{
			DataSource: source, DataExecutionStatus: status,
		}}},
	}); err != nil {
		t.Errorf("encode provider response: %v", err)
	}
}
