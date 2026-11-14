package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// Both people.searchContacts and otherContacts.search cap pageSize at 30 and
// return no page token, so a larger --max can never be met.
func capped30SearchHandler(endpoint string, sent *atomic.Value) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, endpoint) || r.URL.Query().Get("query") == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		sent.Store(r.URL.Query().Get("pageSize"))
		n, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if n > 30 {
			n = 30
		}
		results := make([]map[string]any, 0, n)
		for i := range n {
			results = append(results, map[string]any{"person": map[string]any{
				"resourceName": fmt.Sprintf("people/c%d", i),
				"names":        []map[string]any{{"displayName": fmt.Sprintf("Ada %d", i)}},
			}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}
}

func TestContactsSearch_RespectsThe30ResultCap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verb     []string
		endpoint string
	}{
		{name: "contacts search", verb: []string{"contacts", "search", "ada"}, endpoint: "people:searchContacts"},
		{name: "contacts other search", verb: []string{"contacts", "other", "search", "ada"}, endpoint: "otherContacts:search"},
	} {
		for _, flags := range []struct {
			args     []string
			wantWarn bool
		}{
			{args: nil, wantWarn: false},
			{args: []string{"--max", "50"}, wantWarn: true},
		} {
			t.Run(fmt.Sprintf("%s %v", tc.name, flags.args), func(t *testing.T) {
				var sent atomic.Value
				svc, closeSrv := newPeopleService(t, capped30SearchHandler(tc.endpoint, &sent))
				t.Cleanup(closeSrv)
				args := append(append([]string{"--json", "--account", "a@b.com"}, tc.verb...), flags.args...)
				res := executeWithPeopleTestServices(t, args, peopleTestServices{
					Contacts: fixedPeopleTestService(svc),
					Other:    fixedPeopleTestService(svc),
				})
				if res.err != nil {
					t.Fatalf("Execute: %v", res.err)
				}
				if got := sent.Load(); got != "30" {
					t.Fatalf("pageSize sent = %v, want 30", got)
				}
				warned := strings.Contains(res.stderr, "30")
				if warned != flags.wantWarn {
					t.Fatalf("stderr = %q, want warning about the 30-result cap: %v", res.stderr, flags.wantWarn)
				}
			})
		}
	}
}
