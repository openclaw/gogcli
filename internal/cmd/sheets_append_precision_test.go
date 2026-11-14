package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSheetsAppendPreservesJSONNumbers(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"updates":{"updatedRows":1}}`)
	}))
	defer srv.Close()
	result := executeWithSheetsTestService(t, []string{
		"--home=" + t.TempDir(), "--account=synthetic@example.invalid", "--json",
		"sheets", "append", "--input=RAW", "--values-json=[[9007199254740993,1.234567890123456789]]",
		"--", "synthetic-sheet", "A:B",
	}, newSheetsServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !strings.Contains(body, "9007199254740993") || !strings.Contains(body, "1.234567890123456789") {
		t.Fatalf("input numbers changed in request: %s", body)
	}
}
