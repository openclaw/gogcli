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
)

func TestSheetsReconcileApplyAndVerify(t *testing.T) {
	for _, mode := range []string{"preview", "apply", "drift", "mismatch", "dry-run"} {
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
					if writes > 0 && mode != "mismatch" {
						value = ""
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"valueRanges": []any{map[string]any{"values": [][]any{{value, "human note"}}}}})
					return
				}
				writes++
				var req sheets.BatchUpdateValuesRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.ValueInputOption != "RAW" || len(req.Data) != 1 || req.Data[0].Range != "Data!A1" || req.Data[0].Values[0][0] != "" {
					t.Errorf("unexpected update: %#v", req)
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			var out bytes.Buffer
			ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSheetsServiceFromServer(t, srv))
			args := []string{"test-sheet", "--data-json", `[{"range":"Data!A1:B1","values":[["",null]]}]`}
			if mode != "preview" {
				args = append(args, "--apply")
			}
			err := runKong(t, &SheetsReconcileCmd{}, args, ctx, &RootFlags{Account: "test@example.com", DryRun: mode == "dry-run"})
			if mode == "drift" || mode == "mismatch" {
				if err == nil {
					t.Fatal("expected failure")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantWrites := 0
			if mode == "apply" || mode == "mismatch" {
				wantWrites = 1
			}
			if writes != wantWrites {
				t.Fatalf("writes=%d want %d", writes, wantWrites)
			}
			if mode == "apply" && !strings.Contains(out.String(), `"verified": true`) {
				t.Fatal(out.String())
			}
		})
	}
}

func TestSheetsReconcileRejectsOverlappingOrUnboundedInput(t *testing.T) {
	for _, input := range []string{
		`[{"range":"Data!A1:A","values":[[1]]}]`,
		`[{"range":"Data!A1","values":[[1,2]]}]`,
		`[{"range":"Data!A1","values":[[1]]},{"range":"Data!A1","values":[[2]]}]`,
		`[{"range":"Data!A1","values":[[null]]}]`,
	} {
		data, err := parseSheetsBatchUpdateData(input, strings.NewReader(""))
		if err == nil {
			err = validateSheetsReconcileRanges(data)
		}
		if err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
