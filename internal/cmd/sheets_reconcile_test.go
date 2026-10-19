package cmd

import (
	"bytes"
	"context"
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
					_ = json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]string{"title": "Data"}, "data": []any{map[string]any{"rowData": []any{map[string]any{"values": []any{map[string]any{"effectiveValue": map[string]string{"stringValue": value}}, map[string]any{"effectiveValue": map[string]string{"stringValue": "human note"}}}}}}}}}})
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

func TestSheetsReconcileFormulaErrorIsNotLiteral(t *testing.T) {
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			writes++
			_, _ = io.WriteString(w, `{}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/values:batchGet") {
			_, _ = io.WriteString(w, `{"valueRanges":[{"values":[["#DIV/0!"]]}]}`)
			return
		}
		value := map[string]any{"errorValue": map[string]string{"type": "DIVIDE_BY_ZERO", "message": "division by zero"}}
		if writes > 0 {
			value = map[string]any{"stringValue": "#DIV/0!"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]string{"title": "Data"}, "data": []any{map[string]any{"rowData": []any{map[string]any{"values": []any{map[string]any{"effectiveValue": value}}}}}}}}})
	}))
	defer srv.Close()
	var out bytes.Buffer
	ctx := withSheetsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSheetsServiceFromServer(t, srv))
	err := runKong(t, &SheetsReconcileCmd{}, []string{"test-sheet", "--data-json", `[{"range":"Data!A1","values":[["#DIV/0!"]]}]`, "--apply"}, ctx, &RootFlags{Account: "test@example.com"})
	if err != nil || writes != 1 || !strings.Contains(out.String(), `"verified": true`) {
		t.Fatalf("err=%v writes=%d %s", err, writes, &out)
	}
}

func TestSheetsReconcileTypedGridOffsetsAndMultipleRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()["ranges"]) != 2 {
			t.Error("expected both ranges")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sheets":[{"properties":{"title":"Data"},"data":[{"startRow":2,"startColumn":1,"rowData":[{"values":[{"effectiveValue":{"numberValue":0}},{"effectiveValue":{"boolValue":false}},{"effectiveValue":{"stringValue":""}}]}]},{"startRow":5,"startColumn":3,"rowData":[{"values":[{"effectiveValue":{"numberValue":4}}]}]}]}]}`)
	}))
	defer srv.Close()
	data, err := parseSheetsBatchUpdateData(`[{"range":"Data!B3:D3","values":[[0,false,""]]},{"range":"Data!D6","values":[[4]]}]`, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	values, err := readSheetsReconcile(context.Background(), newSheetsServiceFromServer(t, srv), "test-sheet", data)
	if err != nil || len(diffSheetsReconcile(data, values)) != 0 {
		t.Fatalf("values=%v err=%v", values, err)
	}
}
