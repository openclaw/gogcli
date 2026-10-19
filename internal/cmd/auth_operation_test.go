package cmd

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestAuthOperationPolicyAndScopeAlternatives(t *testing.T) {
	for _, test := range []struct {
		flags   RootFlags
		allowed bool
	}{
		{RootFlags{}, true},
		{RootFlags{EnableCommands: "sheets"}, true},
		{RootFlags{EnableCommandsExact: "sheets.update"}, true},
		{RootFlags{EnableCommands: "auth"}, false},
		{RootFlags{EnableCommands: "*", DisableCommands: "sheets.update"}, false},
	} {
		if (authOperationPolicy(&test.flags, []string{"sheets", "update"}) == nil) != test.allowed {
			t.Fatalf("%+v", test)
		}
	}
	spec, ok := authOperationSpecification("sheets.datasource.refresh")
	if !ok || !spec.write || !strings.HasSuffix(spec.extraScope, "bigquery.readonly") {
		t.Fatalf("%+v", spec)
	}
	if !authOperationHasScope([]string{"https://www.googleapis.com/auth/drive"}, spec.scopes) || authOperationHasScope([]string{"https://www.googleapis.com/auth/spreadsheets.readonly"}, spec.scopes) {
		t.Fatal("invalid write-scope alternatives")
	}
}

func TestAuthOperationProbeOnlyReadsMetadata(t *testing.T) {
	requests := 0
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Query().Get("fields") != "documentId" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"documentId":"test-doc"}`)
	}))
	defer cleanup()
	var out bytes.Buffer
	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	spec, _ := authOperationSpecification("docs.cat")
	if err := probeAuthOperation(ctx, &RootFlags{Account: "test@example.com"}, spec, "test-doc"); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestAuthOperationCommandUnknownRequirements(t *testing.T) {
	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	parser, _, err := newParserWithWriters("test", &out, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	kctx, err := parser.Parse([]string{"auth", "check-operation", "gmail.search"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := AuthCheckOperationCmd{Operation: "gmail.search"}
	if err := cmd.Run(ctx, kctx, &RootFlags{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"unknown"`) || strings.Contains(out.String(), `"ready"`) {
		t.Fatal(out.String())
	}
}

func TestAuthOperationRecordedScopesAndDirectTokens(t *testing.T) {
	for _, direct := range []bool{false, true} {
		var out bytes.Buffer
		ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
		runtime, _ := app.FromContext(ctx)
		store := newMemStore()
		if err := store.SetToken("default", "test@example.com", secrets.Token{Scopes: []string{"https://www.googleapis.com/auth/spreadsheets"}, RefreshToken: "fixture-secret-must-not-appear"}); err != nil {
			t.Fatal(err)
		}
		runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
			if direct {
				t.Fatal("direct-token diagnostics must not open the keyring")
			}
			return store, nil
		}
		parser, _, err := newParserWithWriters("test", &out, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		kctx, err := parser.Parse([]string{"auth", "check-operation", "sheets.update"})
		if err != nil {
			t.Fatal(err)
		}
		flags := &RootFlags{Account: "test@example.com", Client: "default"}
		if direct {
			flags.AccessToken = "fixture-direct-token"
		}
		if err := (&AuthCheckOperationCmd{Operation: "sheets.update"}).Run(ctx, kctx, flags); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "fixture-") {
			t.Fatal("diagnostic leaked a token")
		}
		if strings.Contains(out.String(), "recorded_present") == direct {
			t.Fatalf("incorrect token metadata: %s", &out)
		}
		if !strings.Contains(out.String(), `"name": "execution"`) || !strings.Contains(out.String(), `"not_checked"`) {
			t.Fatal(out.String())
		}
	}
}
