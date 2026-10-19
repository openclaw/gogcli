package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsa1"
)

func TestSheetsUpsertReorderedRowsPreservesHumanColumns(t *testing.T) {
	r, _ := sheetsa1.Parse("Data!B3:D9")
	current := [][]any{{"id", "note", "status"}, {"002", "human note", "old"}, {"001", "another note", "keep"}}
	rows := []map[string]any{{"id": "001", "status": "new"}, {"id": "003", "status": "added"}}
	plan, err := planSheetsUpsert(r, current, rows, "id", []string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Matched != 1 || plan.Inserted != 1 || len(plan.Updates) != 3 {
		t.Fatalf("%+v", plan)
	}
	for i, want := range []string{"Data!D5", "Data!B6", "Data!D6"} {
		if plan.Updates[i].Range != want {
			t.Fatalf("update %d: %s want %s", i, plan.Updates[i].Range, want)
		}
	}
	for _, change := range plan.Updates {
		if strings.Contains(change.Range, "!C") {
			t.Fatal("human column overwritten")
		}
	}
}

func TestSheetsUpsertRejectsAmbiguousKeysAndColumns(t *testing.T) {
	r, _ := sheetsa1.Parse("Data!A1:C2")
	for _, test := range []struct {
		current [][]any
		rows    []map[string]any
		columns []string
	}{
		{[][]any{{"id", "value"}, {"x", "1"}, {"x", "2"}}, []map[string]any{{"id": "x"}}, []string{"value"}},
		{[][]any{{"id", "value"}, {"x", "1"}}, []map[string]any{{"id": "x"}, {"id": "x"}}, []string{"value"}},
		{[][]any{{"id", "value"}, {"x", "1"}}, []map[string]any{{"id": "y"}}, []string{"value"}},
		{[][]any{{"id", "value"}}, []map[string]any{{"id": "x", "note": "oops"}}, []string{"value"}},
		{[][]any{{"id", "value"}, {float64(1), "v"}}, []map[string]any{{"id": "1"}}, []string{"value"}},
	} {
		if _, err := planSheetsUpsert(r, test.current, test.rows, "id", test.columns); err == nil {
			t.Fatalf("accepted %+v", test)
		}
	}
}

func TestSheetsUpsertDryRunAndDrift(t *testing.T) {
	for _, mode := range []string{"apply", "dry-run", "drift"} {
		t.Run(mode, func(t *testing.T) {
			reads, writes := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					reads++
					value := "old"
					if mode == "drift" && reads == 2 {
						value = "human edit"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"values": [][]any{{"id", "note", "value"}, {"001", "human", value}}})
					return
				}
				writes++
				var req sheets.BatchUpdateValuesRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if len(req.Data) != 1 || req.Data[0].Range != "Data!C2" || req.ValueInputOption != "RAW" {
					t.Errorf("bad request: %+v", req)
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			var out bytes.Buffer
			ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSheetsServiceFromServer(t, srv))
			err := runKong(t, &SheetsUpsertCmd{}, []string{"test-sheet", "Data!A1:C10", "--key", "id", "--columns", "value", "--rows-json", `[{"id":"001","value":"new"}]`}, ctx, &RootFlags{Account: "test@example.com", DryRun: mode == "dry-run"})
			if mode == "drift" {
				if err == nil {
					t.Fatal("expected drift failure")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if (writes == 1) != (mode == "apply") {
				t.Fatalf("writes=%d", writes)
			}
		})
	}
}

func TestSheetsUpsertWritesExplicitFormulaLikeLiteral(t *testing.T) {
	r, _ := sheetsa1.Parse("Data!A1:B2")
	plan, err := planSheetsUpsert(r, [][]any{{"id", "value"}, {"001", "=1+1"}}, []map[string]any{{"id": "001", "value": "=1+1"}}, "id", []string{"value"})
	if err != nil || len(plan.Updates) != 1 {
		t.Fatalf("%+v %v", plan, err)
	}
}
