package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestSheetsImportLiteralTextAndHeaderMapping(t *testing.T) {
	var got sheets.BatchUpdateValuesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if !strings.HasSuffix(r.URL.Path, "/values/Data!A1:C1") {
				t.Errorf("wrong header range: %s", r.URL.Path)
			}
			_, _ = io.WriteString(w, `{"values":[["id","human note","value"]]}`)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"totalUpdatedCells":2}`)
	}))
	defer srv.Close()
	f := filepath.Join(t.TempDir(), "input.csv")
	if err := os.WriteFile(f, []byte("value,id\n\"=SUM(A1:A2)\",001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSheetsServiceFromServer(t, srv))
	err := runKong(t, &SheetsImportCmd{}, []string{"sheet-test", `Data\!A1:C10`, "--file", f, "--match-headers"}, ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ValueInputOption != "RAW" || len(got.Data) != 2 || got.Data[0].Range != "Data!C2:C2" || got.Data[1].Values[0][0] != "001" || got.Data[0].Values[0][0] != "=SUM(A1:A2)" {
		t.Fatalf("unexpected request: %#v", got)
	}
}

func TestSheetsImportDelimitedAndInvalidInput(t *testing.T) {
	rows, err := readSheetsDelimited(strings.NewReader("\ufeffid\ttext\r\n001\t\"line1\nline2\"\r\n"), "tsv")
	if err != nil || rows[1][0] != "001" || rows[1][1] != "line1\nline2" {
		t.Fatalf("%v %v", rows, err)
	}
	for _, input := range []string{"", "a,b\n1\n", "\"unterminated"} {
		if _, err := readSheetsDelimited(strings.NewReader(input), "csv"); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestSheetsImportDryRunOffline(t *testing.T) {
	f := filepath.Join(t.TempDir(), "input.tsv")
	if err := os.WriteFile(f, []byte("001\t=1+2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ctx := withSheetsTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("unexpected API call")
		return nil, errRuntimeServiceRequired
	})
	err := runKong(t, &SheetsImportCmd{}, []string{"test-sheet", "Data!B2:C2", "--file", f, "--format", "tsv"}, ctx, &RootFlags{DryRun: true})
	if ExitCode(err) != 0 || !strings.Contains(out.String(), "001") {
		t.Fatalf("%v %s", err, &out)
	}
}
