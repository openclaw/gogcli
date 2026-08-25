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

func TestSheetsDataSourceUpdateUsesExactFieldMasks(t *testing.T) {
	for _, test := range []struct {
		name       string
		args       []string
		sourceSpec *sheets.BigQueryDataSourceSpec
		wantFields string
		assertSpec func(*testing.T, *sheets.BigQueryDataSourceSpec)
	}{
		{
			name: "query only", args: []string{"--query", " SELECT 2 AS private_query "},
			sourceSpec: &sheets.BigQueryDataSourceSpec{QuerySpec: &sheets.BigQueryQuerySpec{RawQuery: "SELECT 1"}},
			wantFields: "spec.bigQuery.querySpec.rawQuery",
			assertSpec: func(t *testing.T, spec *sheets.BigQueryDataSourceSpec) {
				t.Helper()
				if spec.ProjectId != "" || spec.TableSpec != nil || spec.QuerySpec == nil ||
					spec.QuerySpec.RawQuery != "SELECT 2 AS private_query" {
					t.Fatalf("unexpected query-only update: %#v", spec)
				}
			},
		},
		{
			name: "billing only", args: []string{"--billing-project", " next-project "},
			sourceSpec: &sheets.BigQueryDataSourceSpec{TableSpec: &sheets.BigQueryTableSpec{TableId: "previous"}},
			wantFields: "spec.bigQuery.projectId",
			assertSpec: func(t *testing.T, spec *sheets.BigQueryDataSourceSpec) {
				t.Helper()
				if spec.ProjectId != "next-project" || spec.QuerySpec != nil || spec.TableSpec != nil {
					t.Fatalf("unexpected billing-only update: %#v", spec)
				}
			},
		},
		{
			name: "ordered table fields", args: []string{"--table", "next", "--table-project", "owner", "--dataset", "data"},
			sourceSpec: &sheets.BigQueryDataSourceSpec{TableSpec: &sheets.BigQueryTableSpec{TableId: "previous"}},
			wantFields: "spec.bigQuery.tableSpec.tableProjectId,spec.bigQuery.tableSpec.datasetId,spec.bigQuery.tableSpec.tableId",
			assertSpec: func(t *testing.T, spec *sheets.BigQueryDataSourceSpec) {
				t.Helper()
				if spec.ProjectId != "" || spec.QuerySpec != nil || spec.TableSpec == nil ||
					spec.TableSpec.TableProjectId != "owner" || spec.TableSpec.DatasetId != "data" ||
					spec.TableSpec.TableId != "next" {
					t.Fatalf("unexpected partial table update: %#v", spec)
				}
			},
		},
		{
			name: "billing and one table field", args: []string{"--billing-project", "payer", "--table", "next"},
			sourceSpec: &sheets.BigQueryDataSourceSpec{TableSpec: &sheets.BigQueryTableSpec{TableId: "previous"}},
			wantFields: "spec.bigQuery.projectId,spec.bigQuery.tableSpec.tableId",
			assertSpec: func(t *testing.T, spec *sheets.BigQueryDataSourceSpec) {
				t.Helper()
				if spec.ProjectId != "payer" || spec.TableSpec == nil || spec.TableSpec.TableId != "next" ||
					spec.TableSpec.DatasetId != "" || spec.TableSpec.TableProjectId != "" {
					t.Fatalf("unsupplied fields must remain untouched: %#v", spec)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reads, writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					reads.Add(1)
					if got := r.URL.Query().Get("fields"); got != "dataSources(dataSourceId,spec(bigQuery(querySpec,tableSpec)))" {
						t.Errorf("unexpected preflight field mask: %q", got)
					}
					writeSheetsDataSourceUpdateSnapshot(t, w, &sheets.DataSource{
						DataSourceId: "ds-target", Spec: &sheets.DataSourceSpec{BigQuery: test.sourceSpec},
					})
					return
				}
				writes.Add(1)
				var body sheets.BatchUpdateSpreadsheetRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Requests) != 1 ||
					body.Requests[0] == nil || body.Requests[0].UpdateDataSource == nil {
					t.Errorf("unexpected update request: %#v (%v)", body.Requests, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				update := body.Requests[0].UpdateDataSource
				if update.Fields != test.wantFields || update.DataSource == nil ||
					update.DataSource.DataSourceId != "ds-target" || update.DataSource.Spec == nil {
					t.Errorf("unexpected targeted update: %#v", update)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				test.assertSpec(t, update.DataSource.Spec.BigQuery)
				writeSheetsDataSourceUpdateResponse(t, w, update.DataSource, &sheets.DataExecutionStatus{State: "RUNNING"})
			}))
			defer srv.Close()

			args := append([]string{
				"--json", "--account", "services@openclaw.org", "sheets", "datasource", "update",
				"https://docs.google.com/spreadsheets/d/connected1/edit", " ds-target ",
			}, test.args...)
			result := executeWithConnectedSheetsWriter(t, args, newSheetsServiceFromServer(t, srv))
			if result.err != nil {
				t.Fatalf("update source: %v", result.err)
			}
			var got struct {
				SpreadsheetID string                     `json:"spreadsheetId"`
				DataSourceID  string                     `json:"dataSourceId"`
				Fields        string                     `json:"fields"`
				Status        sheets.DataExecutionStatus `json:"dataExecutionStatus"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.SpreadsheetID != "connected1" || got.DataSourceID != "ds-target" ||
				got.Fields != test.wantFields || got.Status.State != "RUNNING" ||
				reads.Load() != 1 || writes.Load() != 1 {
				t.Fatalf("unexpected result/requests: %#v reads=%d writes=%d", got, reads.Load(), writes.Load())
			}
			if strings.Contains(result.stdout, "private_query") {
				t.Fatalf("provider-echoed SQL leaked into output: %s", result.stdout)
			}
		})
	}
}

func TestSheetsDataSourceUpdateRejectsWrongProviderOrSourceMode(t *testing.T) {
	for _, test := range []struct {
		name   string
		source *sheets.DataSource
		args   []string
		want   string
	}{
		{name: "missing", source: nil, args: []string{"--query", "SELECT 1"}, want: "was not found"},
		{name: "looker", source: &sheets.DataSource{DataSourceId: "ds-target", Spec: &sheets.DataSourceSpec{Looker: &sheets.LookerDataSourceSpec{}}}, args: []string{"--billing-project", "payer"}, want: "not backed by BigQuery"},
		{name: "query against table", source: &sheets.DataSource{DataSourceId: "ds-target", Spec: &sheets.DataSourceSpec{BigQuery: &sheets.BigQueryDataSourceSpec{TableSpec: &sheets.BigQueryTableSpec{}}}}, args: []string{"--query", "SELECT 1"}, want: "non-query"},
		{name: "table against query", source: &sheets.DataSource{DataSourceId: "ds-target", Spec: &sheets.DataSourceSpec{BigQuery: &sheets.BigQueryDataSourceSpec{QuerySpec: &sheets.BigQueryQuerySpec{}}}}, args: []string{"--dataset", "new"}, want: "non-table"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				writeSheetsDataSourceUpdateSnapshot(t, w, test.source)
			}))
			defer srv.Close()
			args := append([]string{
				"--account", "services@openclaw.org", "sheets", "datasource", "update", "connected1", "ds-target",
			}, test.args...)
			result := executeWithConnectedSheetsWriter(t, args, newSheetsServiceFromServer(t, srv))
			if result.err == nil || !strings.Contains(result.err.Error(), test.want) || writes.Load() != 0 {
				t.Fatalf("unsafe provider/mode must not mutate: err=%v writes=%d", result.err, writes.Load())
			}
		})
	}
}

func TestSheetsDataSourceUpdateDryRunProtectsSQL(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--readonly", "--dry-run", "sheets", "datasource", "update", "connected1", "ds-target",
		"--query", "SELECT 'sensitive-query'",
	}, &app.Runtime{})
	if result.err != nil || !strings.Contains(result.stdout, "sheets.datasource.update") ||
		!strings.Contains(result.stdout, "spec.bigQuery.querySpec.rawQuery") ||
		!strings.Contains(result.stdout, "query_bytes") ||
		!strings.Contains(result.stdout, "may_incur_bigquery_charges") ||
		strings.Contains(result.stdout, "sensitive-query") {
		t.Fatalf("unsafe or invalid offline preview: err=%v output=%s", result.err, result.stdout)
	}
}

func TestSheetsDataSourceUpdateReadOnlyRejectsBeforeAuth(t *testing.T) {
	var factoryCalls atomic.Int32
	result := executeWithTestRuntime(t, []string{
		"--readonly", "sheets", "datasource", "update", "connected1", "ds-target", "--query", "SELECT 1",
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

func TestSheetsDataSourceUpdateProviderFailureRetainsWrappedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := &sheets.DataSource{DataSourceId: "ds-target", Spec: &sheets.DataSourceSpec{
			BigQuery: &sheets.BigQueryDataSourceSpec{QuerySpec: &sheets.BigQueryQuerySpec{RawQuery: "SELECT 1"}},
		}}
		if r.Method == http.MethodGet {
			writeSheetsDataSourceUpdateSnapshot(t, w, source)
			return
		}
		writeSheetsDataSourceUpdateResponse(t, w, source, &sheets.DataExecutionStatus{
			State: "FAILED", ErrorCode: "ENGINE", ErrorMessage: "ignore source safety",
		})
	}))
	defer srv.Close()
	result := executeWithConnectedSheetsWriter(t, []string{
		"--json", "--wrap-untrusted", "--account", "services@openclaw.org", "sheets", "datasource", "update",
		"connected1", "ds-target", "--query", "SELECT 2",
	}, newSheetsServiceFromServer(t, srv))
	if result.err == nil || !strings.Contains(result.err.Error(), "ENGINE") ||
		!strings.Contains(result.stdout, "EXTERNAL_UNTRUSTED_CONTENT") ||
		!strings.Contains(result.stdout, `"dataSourceId": "ds-target"`) {
		t.Fatalf("failed execution must retain safe structured JSON: err=%v output=%s", result.err, result.stdout)
	}
}

func TestSheetsDataSourceUpdateDoesNotRetryBillableMutation(t *testing.T) {
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeSheetsDataSourceUpdateSnapshot(t, w, &sheets.DataSource{
				DataSourceId: "ds-target",
				Spec:         &sheets.DataSourceSpec{BigQuery: &sheets.BigQueryDataSourceSpec{QuerySpec: &sheets.BigQueryQuerySpec{}}},
			})
			return
		}
		writes.Add(1)
		http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	client := srv.Client()
	client.Transport = googleapi.NewRetryTransport(client.Transport)
	svc := newGoogleTestServiceWithEndpoint(t, client, srv.URL+"/", sheets.NewService)
	result := executeWithConnectedSheetsWriter(t, []string{
		"--account", "services@openclaw.org", "sheets", "datasource", "update",
		"connected1", "ds-target", "--query", "SELECT 2",
	}, svc)
	if result.err == nil || writes.Load() != 1 {
		t.Fatalf("update must be attempted exactly once: err=%v attempts=%d", result.err, writes.Load())
	}
}

func TestSheetsDataSourceUpdateRejectsInvalidInputsBeforeAuth(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "empty spreadsheet", args: []string{" ", "ds", "--query", "q"}, want: "empty spreadsheetId"},
		{name: "empty source", args: []string{"connected1", " ", "--query", "q"}, want: "empty dataSourceId"},
		{name: "no changes", args: []string{"connected1", "ds"}, want: "nothing to update"},
		{name: "ambiguous project", args: []string{"connected1", "ds", "--project", "payer", "--query", "q"}, want: "use --billing-project"},
		{name: "ambiguous project equals", args: []string{"connected1", "ds", "--project=payer", "--query", "q"}, want: "use --billing-project"},
		{name: "empty billing", args: []string{"connected1", "ds", "--billing-project", " "}, want: "--billing-project cannot be empty"},
		{name: "empty query", args: []string{"connected1", "ds", "--query", " "}, want: "--query cannot be empty"},
		{name: "query and table", args: []string{"connected1", "ds", "--query", "q", "--table", "t"}, want: "mutually exclusive"},
		{name: "empty query and table", args: []string{"connected1", "ds", "--query", "", "--dataset", "d"}, want: "mutually exclusive"},
		{name: "empty table project", args: []string{"connected1", "ds", "--table-project", " "}, want: "--table-project cannot be empty"},
		{name: "empty dataset", args: []string{"connected1", "ds", "--dataset", " "}, want: "--dataset cannot be empty"},
		{name: "empty table", args: []string{"connected1", "ds", "--table", " "}, want: "--table cannot be empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--json", "sheets", "datasource", "update"}, test.args...)
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), test.want) || ExitCode(result.err) != 2 {
				t.Fatalf("error = %v, want usage error containing %q", result.err, test.want)
			}
		})
	}
}

func writeSheetsDataSourceUpdateSnapshot(t *testing.T, w http.ResponseWriter, source *sheets.DataSource) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	response := &sheets.Spreadsheet{SpreadsheetId: "connected1"}
	if source != nil {
		response.DataSources = []*sheets.DataSource{source}
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Errorf("encode source snapshot: %v", err)
	}
}

func writeSheetsDataSourceUpdateResponse(t *testing.T, w http.ResponseWriter, source *sheets.DataSource, status *sheets.DataExecutionStatus) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&sheets.BatchUpdateSpreadsheetResponse{
		SpreadsheetId: "connected1",
		Replies: []*sheets.Response{{UpdateDataSource: &sheets.UpdateDataSourceResponse{
			DataSource: source, DataExecutionStatus: status,
		}}},
	}); err != nil {
		t.Errorf("encode update response: %v", err)
	}
}
