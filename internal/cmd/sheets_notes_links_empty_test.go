package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A range with no notes or links must still be a JSON array, so scripts can
// use `jq '.[]'` or `length` on it, with and without --results-only.
func TestSheetsNotesAndLinks_EmptyRangeJSONIsEmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sheets": []map[string]any{{
				"properties": map[string]any{"title": "Sheet1"},
				"data": []map[string]any{{
					"rowData": []map[string]any{{"values": []map[string]any{{"formattedValue": "plain"}}}},
				}},
			}},
		})
	}))
	defer srv.Close()
	svc := newSheetsServiceFromServer(t, srv)

	for _, tc := range []struct {
		command string
		key     string
	}{
		{command: "notes", key: "notes"},
		{command: "links", key: "links"},
	} {
		envelope := executeWithSheetsTestService(t, []string{"--json", "--account", "a@b.com", "sheets", tc.command, "s1", "Sheet1!A1:A1"}, svc)
		if envelope.err != nil {
			t.Fatalf("%s: %v", tc.command, envelope.err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal([]byte(envelope.stdout), &got); err != nil {
			t.Fatalf("%s json: %v (%q)", tc.command, err, envelope.stdout)
		}
		if string(got[tc.key]) != "[]" {
			t.Fatalf("%s: %q = %s, want []", tc.command, tc.key, got[tc.key])
		}

		unwrapped := executeWithSheetsTestService(t, []string{"--json", "--results-only", "--account", "a@b.com", "sheets", tc.command, "s1", "Sheet1!A1:A1"}, svc)
		if unwrapped.err != nil {
			t.Fatalf("%s --results-only: %v", tc.command, unwrapped.err)
		}
		if strings.TrimSpace(unwrapped.stdout) != "[]" {
			t.Fatalf("%s --results-only = %s, want []", tc.command, strings.TrimSpace(unwrapped.stdout))
		}
	}
}
