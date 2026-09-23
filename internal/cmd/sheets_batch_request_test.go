package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func sheetsBatchRequestTestRuntime(t *testing.T, handler http.HandlerFunc) *app.Runtime {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := sheetsBatchRequestBaseURL
	sheetsBatchRequestBaseURL = server.URL + "/v4"
	t.Cleanup(func() { sheetsBatchRequestBaseURL = previous })
	return &app.Runtime{Services: app.Services{
		SheetsHTTP: func(ctx context.Context, account string) (*http.Client, error) {
			if account != "test@example.com" {
				t.Errorf("account = %q", account)
			}
			return &http.Client{Transport: googleapi.NewRetryTransport(http.DefaultTransport)}, nil
		},
	}}
}

func TestSheetsBatchRequestPreservesWireValues(t *testing.T) {
	setTestConfigHome(t)
	requests := `[{"updateCells":{"range":{"sheetId":0,"startRowIndex":0},"rows":[{"values":[{"userEnteredFormat":{"textFormat":{"bold":false}},"note":"","userEnteredValue":null}]}],"fields":"userEnteredFormat,note,userEnteredValue"}},{"unknownFutureRequest":{"largeInteger":9007199254740993}}]`
	response := `{"spreadsheetId":"sheet-1","replies":[{},{}],"futureField":9007199254740993}`
	calls := 0
	runtime := sheetsBatchRequestTestRuntime(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v4/spreadsheets/sheet-1:batchUpdate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			t.Error(err)
		}
		if want := `{"requests":` + requests + `}`; compact.String() != want {
			t.Errorf("wire payload = %s, want %s", compact.String(), want)
		}
		_, _ = fmt.Fprint(w, response)
	})
	result := executeWithTestRuntime(t, []string{
		"--account", "test@example.com", "--force", "--json", "sheets", "batch-request", "sheet-1", "--requests-json", requests,
	}, runtime)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if calls != 1 || !strings.Contains(result.stdout, "9007199254740993") || !strings.Contains(result.stdout, `"replies"`) {
		t.Fatalf("calls = %d; output = %s", calls, result.stdout)
	}
}

func TestParseSheetsBatchRequests(t *testing.T) {
	valid := `[{"addSheet":{"properties":{"title":"New"}}}]`
	file := filepath.Join(t.TempDir(), "requests.json")
	if err := os.WriteFile(file, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{valid, "@" + file, "@-"} {
		requests, err := parseSheetsBatchRequests(source, strings.NewReader(valid))
		if err != nil || len(requests) != 1 {
			t.Fatalf("source %q: requests = %v, error = %v", source, requests, err)
		}
	}
	for _, source := range []string{"", "null", "[]", "{}", "[null]", "[false]", "[1]", `["request"]`, "[[]]", "[{}] {}", "["} {
		t.Run(source, func(t *testing.T) {
			if _, err := parseSheetsBatchRequests(source, strings.NewReader("")); ExitCode(err) != 2 {
				t.Fatalf("expected usage error, got %v", err)
			}
		})
	}
}

func TestSheetsBatchRequestDryRunAndWriteGuards(t *testing.T) {
	setTestConfigHome(t)
	runtime := &app.Runtime{Services: app.Services{
		SheetsHTTP: func(context.Context, string) (*http.Client, error) {
			t.Fatal("must not construct authenticated client")
			return nil, errRuntimeServiceRequired
		},
	}}
	args := []string{"--account", "test@example.com", "--json", "sheets", "batch-request", "sheet-1", "--requests-json", `[{"deleteSheet":{"sheetId":0}}]`}
	for _, test := range []struct {
		name  string
		flags []string
		want  int
	}{
		{name: "dry run", flags: []string{"--dry-run"}, want: 0},
		{name: "noninteractive", flags: []string{"--no-input"}, want: 2},
		{name: "readonly", flags: []string{"--readonly", "--force"}, want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := executeWithTestRuntime(t, append(test.flags, args...), runtime)
			if got := ExitCode(result.err); got != test.want {
				t.Fatalf("exit = %d, want %d; error = %v", got, test.want, result.err)
			}
			if test.name == "dry run" && (!strings.Contains(result.stdout, `"sheetId": 0`) || !strings.Contains(result.stdout, `"deleteSheet"`)) {
				t.Fatalf("dry-run payload missing: %s", result.stdout)
			}
		})
	}
}

func TestSheetsBatchRequestDoesNotReplayServerFailure(t *testing.T) {
	setTestConfigHome(t)
	calls := 0
	runtime := sheetsBatchRequestTestRuntime(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"error":{"code":503,"message":"response lost after applying batch"}}`)
	})
	result := executeWithTestRuntime(t, []string{
		"--account", "test@example.com", "--force", "sheets", "batch-request", "sheet-1", "--requests-json", `[{"insertDimension":{}}]`,
	}, runtime)
	if ExitCode(result.err) != 8 || calls != 1 || result.stdout != "" {
		t.Fatalf("error = %v, calls = %d, stdout = %q", result.err, calls, result.stdout)
	}
}

func TestSheetsBatchRequestRuntimePolicy(t *testing.T) {
	setTestConfigHome(t)
	for _, test := range []struct {
		name  string
		flags []string
		allow bool
	}{
		{name: "unrestricted", allow: true},
		{name: "parent", flags: []string{"--enable-commands", "sheets"}},
		{name: "parent with deny", flags: []string{"--enable-commands", "sheets", "--disable-commands", "sheets.delete-tab"}},
		{name: "deny only", flags: []string{"--disable-commands", "sheets.delete-tab"}},
		{name: "wildcard with deny", flags: []string{"--enable-commands", "*", "--disable-commands", "sheets.delete-tab"}},
		{name: "wildcard unrestricted", flags: []string{"--enable-commands", "*"}, allow: true},
		{name: "explicit", flags: []string{"--enable-commands", "sheets.batch-request", "--disable-commands", "sheets.delete-tab"}, allow: true},
		{name: "exact", flags: []string{"--enable-commands-exact", "sheets.batch-request"}, allow: true},
		{name: "parent deny", flags: []string{"--enable-commands", "sheets.batch-request", "--disable-commands", "sheets"}},
		{name: "direct deny", flags: []string{"--enable-commands", "sheets.batch-request", "--disable-commands", "sheets.batch-request"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `[{"addSheet":{}}]`
			if !test.allow {
				source = "@/does-not-exist-policy-must-run-before-input"
			}
			args := append([]string(nil), test.flags...)
			args = append(args, "--dry-run", "sheets", "batch-request", "sheet-1", "--requests-json", source)
			result := executeWithTestRuntime(t, args, nil)
			if test.allow {
				if result.err != nil {
					t.Fatal(result.err)
				}
			} else if ExitCode(result.err) != 2 || strings.Contains(result.err.Error(), "read --requests-json") {
				t.Fatalf("expected policy rejection before file access, got %v", result.err)
			}
		})
	}
}

func TestSheetsBatchRequestDoesNotFollowRedirects(t *testing.T) {
	setTestConfigHome(t)
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			runtime := sheetsBatchRequestTestRuntime(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v4/spreadsheets/sheet-1:batchUpdate" {
					t.Errorf("followed redirect: %s", r.URL.Path)
				}
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(code)
			})
			result := executeWithTestRuntime(t, []string{
				"--account", "test@example.com", "--force", "sheets", "batch-request", "sheet-1", "--requests-json", `[{"addSheet":{}}]`,
			}, runtime)
			if result.err == nil || calls != 1 || result.stdout != "" {
				t.Fatalf("error = %v, calls = %d, stdout = %q", result.err, calls, result.stdout)
			}
		})
	}
}
