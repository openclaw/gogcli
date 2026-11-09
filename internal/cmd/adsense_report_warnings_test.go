package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A truncated report (totalMatchedRows above the rows returned) and the API's
// own warnings reach JSON output; table and --plain output must not drop them.
func TestExecute_AdSenseReportsQuery_TableShowsTruncationAndWarnings(t *testing.T) {
	svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reports:generate")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"headers": []map[string]any{
				{"name": "DATE", "type": "DIMENSION"},
				{"name": "ESTIMATED_EARNINGS", "type": "METRIC_CURRENCY"},
			},
			"rows": []map[string]any{
				{"cells": []map[string]any{{"value": "2026-02-01"}, {"value": "1.23"}}},
				{"cells": []map[string]any{{"value": "2026-02-02"}, {"value": "4.56"}}},
			},
			"totalMatchedRows": "250000",
			"warnings":         []string{"Some requested metrics are not available for this date range."},
		})
	}))
	for _, mode := range []string{"--plain", ""} {
		args := []string{"--account", "a@b.com", "adsense", "reports", "query", "pub-123", "--metrics", "estimated_earnings"}
		if mode != "" {
			args = append([]string{mode}, args...)
		}
		result := executeWithAdSenseTestService(t, args, svc)
		if result.err != nil {
			t.Fatalf("%q: Execute: %v", mode, result.err)
		}
		if !strings.Contains(result.stdout, "2026-02-02") {
			t.Fatalf("%q: rows missing from stdout: %q", mode, result.stdout)
		}
		if strings.Contains(result.stdout, "250000") || strings.Contains(result.stdout, "not available") {
			t.Fatalf("%q: notes must stay off stdout: %q", mode, result.stdout)
		}
		if !strings.Contains(result.stderr, "2 of 250000") {
			t.Fatalf("%q: want truncation note on stderr, got %q", mode, result.stderr)
		}
		if !strings.Contains(result.stderr, "Some requested metrics are not available") {
			t.Fatalf("%q: want API warning on stderr, got %q", mode, result.stderr)
		}
	}
}
