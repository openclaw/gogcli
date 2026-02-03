package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOrgunitsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationUnits": []map[string]any{
				{"name": "Sales", "orgUnitPath": "/Sales", "orgUnitId": "ou-1"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &OrgunitsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Sales") {
		t.Fatalf("unexpected output: %s", out)
	}
}
