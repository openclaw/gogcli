package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	datatransfer "google.golang.org/api/admin/datatransfer/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestTransferListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{
				{"id": "t1", "oldOwnerUserId": "old@example.com", "newOwnerUserId": "new@example.com", "overallTransferStatusCode": "completed"},
			},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "t1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestTransferCreateCmd(t *testing.T) {
	var gotOld string
	var gotApp string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			OldOwnerUserId           string `json:"oldOwnerUserId"`
			ApplicationDataTransfers []struct {
				ApplicationId string `json:"applicationId"`
			} `json:"applicationDataTransfers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotOld = payload.OldOwnerUserId
		if len(payload.ApplicationDataTransfers) > 0 {
			gotApp = payload.ApplicationDataTransfers[0].ApplicationId
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "t2",
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{OldOwner: "old@example.com", NewOwner: "new@example.com", Application: "4350700"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotOld != "old@example.com" {
		t.Fatalf("unexpected old owner: %q", gotOld)
	}
	if gotApp != "4350700" {
		t.Fatalf("unexpected app id: %s", gotApp)
	}
	if !strings.Contains(out, "Created transfer") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubDataTransfer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newDataTransferService
	svc, err := datatransfer.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new datatransfer service: %v", err)
	}
	newDataTransferService = func(context.Context, string) (*datatransfer.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newDataTransferService = orig
		srv.Close()
	})
	return srv
}

func TestTransferListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{
				{
					"id":                        "t1",
					"oldOwnerUserId":            "old@example.com",
					"newOwnerUserId":            "new@example.com",
					"overallTransferStatusCode": "completed",
					"applicationDataTransfers":  []map[string]any{{"applicationId": "123"}},
				},
			},
			"nextPageToken": "npt",
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{}

	ctx := testContextTransfer(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result struct {
		DataTransfers []struct {
			ID                        string `json:"id"`
			OldOwnerUserId            string `json:"oldOwnerUserId"`
			NewOwnerUserId            string `json:"newOwnerUserId"`
			OverallTransferStatusCode string `json:"overallTransferStatusCode"`
		} `json:"dataTransfers"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(result.DataTransfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(result.DataTransfers))
	}
	if result.DataTransfers[0].ID != "t1" {
		t.Fatalf("expected transfer ID t1, got %q", result.DataTransfers[0].ID)
	}
	if result.NextPageToken != "npt" {
		t.Fatalf("expected next page token npt, got %q", result.NextPageToken)
	}
}

func TestTransferListCmd_WithFilters(t *testing.T) {
	var gotOldOwner, gotNewOwner, gotStatus string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		gotOldOwner = r.URL.Query().Get("oldOwnerUserId")
		gotNewOwner = r.URL.Query().Get("newOwnerUserId")
		gotStatus = r.URL.Query().Get("status")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{
		OldOwner: "old@example.com",
		NewOwner: "new@example.com",
		Status:   "completed",
		Max:      50,
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			_ = cmd.Run(testContextTransfer(t), flags)
		})
	})

	if gotOldOwner != "old@example.com" {
		t.Fatalf("expected old owner filter, got %q", gotOldOwner)
	}
	if gotNewOwner != "new@example.com" {
		t.Fatalf("expected new owner filter, got %q", gotNewOwner)
	}
	if gotStatus != "completed" {
		t.Fatalf("expected status filter, got %q", gotStatus)
	}
}

func TestTransferListCmd_EmptyList(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "No transfers found") {
		t.Fatalf("expected 'No transfers found' in stderr, got %q", stderr)
	}
}

func TestTransferListCmd_WithPagination(t *testing.T) {
	var gotPageToken string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{
				{"id": "t1"},
			},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{Page: "page123"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextTransfer(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageToken != "page123" {
		t.Fatalf("expected page token 'page123', got %q", gotPageToken)
	}
}

func TestTransferGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers/t123") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                        "t123",
			"oldOwnerUserId":            "old@example.com",
			"newOwnerUserId":            "new@example.com",
			"overallTransferStatusCode": "completed",
			"applicationDataTransfers":  []map[string]any{{"applicationId": "456"}},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferGetCmd{TransferID: "t123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "t123") {
		t.Fatalf("expected transfer ID in output, got %q", out)
	}
	if !strings.Contains(out, "old@example.com") {
		t.Fatalf("expected old owner in output, got %q", out)
	}
	if !strings.Contains(out, "new@example.com") {
		t.Fatalf("expected new owner in output, got %q", out)
	}
	if !strings.Contains(out, "completed") {
		t.Fatalf("expected status in output, got %q", out)
	}
}

func TestTransferGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers/t456") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                        "t456",
			"oldOwnerUserId":            "old@example.com",
			"newOwnerUserId":            "new@example.com",
			"overallTransferStatusCode": "inProgress",
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferGetCmd{TransferID: "t456"}

	ctx := testContextTransfer(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result struct {
		ID                        string `json:"id"`
		OldOwnerUserId            string `json:"oldOwnerUserId"`
		NewOwnerUserId            string `json:"newOwnerUserId"`
		OverallTransferStatusCode string `json:"overallTransferStatusCode"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if result.ID != "t456" {
		t.Fatalf("expected ID t456, got %q", result.ID)
	}
	if result.OverallTransferStatusCode != "inProgress" {
		t.Fatalf("expected status inProgress, got %q", result.OverallTransferStatusCode)
	}
}

func TestTransferGetCmd_EmptyID(t *testing.T) {
	stubDataTransfer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferGetCmd{TransferID: "  "}

	err := cmd.Run(testContextTransfer(t), flags)
	if err == nil {
		t.Fatal("expected error for empty transfer ID")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected 'required' in error, got %v", err)
	}
}

func TestTransferCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                        "t789",
			"oldOwnerUserId":            "old@example.com",
			"newOwnerUserId":            "new@example.com",
			"overallTransferStatusCode": "pending",
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{
		OldOwner:    "old@example.com",
		NewOwner:    "new@example.com",
		Application: "12345",
	}

	ctx := testContextTransfer(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if result.ID != "t789" {
		t.Fatalf("expected ID t789, got %q", result.ID)
	}
}

func TestTransferCreateCmd_MissingOwners(t *testing.T) {
	stubDataTransfer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{
		OldOwner:    "",
		NewOwner:    "new@example.com",
		Application: "12345",
	}

	err := cmd.Run(testContextTransfer(t), flags)
	if err == nil {
		t.Fatal("expected error for missing old owner")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected 'required' in error, got %v", err)
	}
}

func TestTransferCreateCmd_InvalidApplication(t *testing.T) {
	stubDataTransfer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{
		OldOwner:    "old@example.com",
		NewOwner:    "new@example.com",
		Application: "not-a-number",
	}

	err := cmd.Run(testContextTransfer(t), flags)
	if err == nil {
		t.Fatal("expected error for invalid application ID")
	}
	if !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("expected 'numeric' in error, got %v", err)
	}
}

func TestTransferCreateCmd_WithJSONParams(t *testing.T) {
	var gotParams []map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			ApplicationDataTransfers []struct {
				ApplicationTransferParams []map[string]any `json:"applicationTransferParams"`
			} `json:"applicationDataTransfers"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.ApplicationDataTransfers) > 0 {
			gotParams = payload.ApplicationDataTransfers[0].ApplicationTransferParams
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1"})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{
		OldOwner:    "old@example.com",
		NewOwner:    "new@example.com",
		Application: "12345",
		Parameters:  `{"key1": ["value1", "value2"]}`,
	}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(gotParams) != 1 {
		t.Fatalf("expected 1 param, got %d", len(gotParams))
	}
}

func TestTransferCreateCmd_WithKeyValueParams(t *testing.T) {
	var gotParams []map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			ApplicationDataTransfers []struct {
				ApplicationTransferParams []map[string]any `json:"applicationTransferParams"`
			} `json:"applicationDataTransfers"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.ApplicationDataTransfers) > 0 {
			gotParams = payload.ApplicationDataTransfers[0].ApplicationTransferParams
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1"})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{
		OldOwner:    "old@example.com",
		NewOwner:    "new@example.com",
		Application: "12345",
		Parameters:  "key1=value1,key2=value2",
	}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(gotParams) != 2 {
		t.Fatalf("expected 2 params, got %d", len(gotParams))
	}
}

func TestTransferApplicationsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/applications") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"applications": []map[string]any{
				{"id": "100", "name": "Google Drive", "transferParams": []map[string]any{{"key": "param1"}}},
				{"id": "200", "name": "Google Calendar", "transferParams": []map[string]any{}},
			},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferApplicationsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Google Drive") {
		t.Fatalf("expected Google Drive in output, got %q", out)
	}
	if !strings.Contains(out, "Google Calendar") {
		t.Fatalf("expected Google Calendar in output, got %q", out)
	}
}

func TestTransferApplicationsCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/applications") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"applications": []map[string]any{
				{"id": "100", "name": "Google Drive"},
			},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferApplicationsCmd{}

	ctx := testContextTransfer(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result struct {
		Applications []struct {
			ID   int64  `json:"id,string"`
			Name string `json:"name"`
		} `json:"applications"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(result.Applications) != 1 {
		t.Fatalf("expected 1 application, got %d", len(result.Applications))
	}
	if result.Applications[0].Name != "Google Drive" {
		t.Fatalf("expected Google Drive, got %q", result.Applications[0].Name)
	}
}

func TestTransferApplicationsCmd_EmptyList(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/applications") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"applications": []map[string]any{},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferApplicationsCmd{}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdoutTransfer(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "No applications found") {
		t.Fatalf("expected 'No applications found' in stderr, got %q", stderr)
	}
}

func TestParseTransferParams_EmptyInput(t *testing.T) {
	params, err := parseTransferParams("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params != nil {
		t.Fatalf("expected nil params, got %v", params)
	}
}

func TestParseTransferParams_SimpleJSON(t *testing.T) {
	params, err := parseTransferParams(`{"key1": "value1"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Key != "key1" {
		t.Fatalf("expected key1, got %q", params[0].Key)
	}
}

func TestParseTransferParams_ArrayJSON(t *testing.T) {
	params, err := parseTransferParams(`{"key1": ["v1", "v2"]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if len(params[0].Value) != 2 {
		t.Fatalf("expected 2 values, got %d", len(params[0].Value))
	}
}

func TestParseTransferParams_KeyValuePairs(t *testing.T) {
	params, err := parseTransferParams("key1=value1,key2=value2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
}

func TestParseTransferParams_PipeDelimitedValues(t *testing.T) {
	params, err := parseTransferParams("key1=v1|v2|v3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if len(params[0].Value) != 3 {
		t.Fatalf("expected 3 values, got %d", len(params[0].Value))
	}
}

func TestParseTransferParams_InvalidKeyValue(t *testing.T) {
	_, err := parseTransferParams("invalid-no-equals")
	if err == nil {
		t.Fatal("expected error for invalid key=value")
	}
	if !strings.Contains(err.Error(), "expected key=value") {
		t.Fatalf("expected 'expected key=value' in error, got %v", err)
	}
}

func TestParseTransferParams_EmptyKey(t *testing.T) {
	_, err := parseTransferParams("=value")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("expected 'empty key' in error, got %v", err)
	}
}

func TestParseTransferParams_EmptyValue(t *testing.T) {
	_, err := parseTransferParams("key=")
	if err == nil {
		t.Fatal("expected error for empty value")
	}
	if !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("expected 'empty value' in error, got %v", err)
	}
}

func TestTransferParamsFromMap(t *testing.T) {
	paramsMap := map[string][]string{
		"key1": {"value1"},
		"key2": {"v2a", "v2b"},
	}
	params := transferParamsFromMap(paramsMap)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
}

func testContextTransfer(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func testContextWithStdoutTransfer(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}
