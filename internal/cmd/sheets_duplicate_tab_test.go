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

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestSheetsDuplicateTab(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		index float64
	}{
		{"default", nil, 1},
		{"first", []string{"--index", "0"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, `{"sheets":[{"properties":{"sheetId":0,"title":"Data","index":0}}]}`)
					return
				}
				var body struct {
					Requests []struct {
						DuplicateSheet map[string]any `json:"duplicateSheet"`
					} `json:"requests"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if len(body.Requests) != 1 {
					t.Errorf("requests = %v", body.Requests)
					return
				}
				got := body.Requests[0].DuplicateSheet
				if v, ok := got["sourceSheetId"]; !ok || v != float64(0) {
					t.Errorf("sourceSheetId missing or wrong: %v", got)
				}
				if v, ok := got["insertSheetIndex"]; !ok || v != tc.index {
					t.Errorf("insertSheetIndex missing or wrong: %v", got)
				}
				if got["newSheetName"] != "Backup" {
					t.Errorf("name = %v", got)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"replies": []any{map[string]any{"duplicateSheet": map[string]any{"properties": map[string]any{"sheetId": 7, "title": "Backup", "index": tc.index}}}}})
			}))
			defer srv.Close()
			var output bytes.Buffer
			ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, &output, io.Discard), newSheetsServiceFromServer(t, srv))
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			args := append([]string{"s1", "Data", "Backup"}, tc.args...)
			if err := runKong(t, &SheetsDuplicateTabCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["sheetId"] != float64(7) || result["title"] != "Backup" || result["index"] != tc.index {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestSheetsDuplicateTab_RejectsBeforeMutation(t *testing.T) {
	for _, args := range [][]string{{"s1", "missing", "Backup"}, {"s1", "Data", "Backup", "--index=2"}} {
		capture := &sheetsBatchUpdateCapture{}
		svc := newSheetsBatchUpdateTestService(t, map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Data"}}}}, capture)
		ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
		if err := runKong(t, &SheetsDuplicateTabCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err == nil || capture.Body != nil {
			t.Fatalf("expected rejection before mutation, err=%v body=%v", err, capture.Body)
		}
	}
}

func TestSheetsDuplicateTab_DryRunWithoutAuth(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(t, &SheetsDuplicateTabCmd{}, []string{"s1", "Data", "Backup"}, ctx, &RootFlags{DryRun: true, NoInput: true})
	if err != nil && ExitCode(err) != 0 {
		t.Fatal(err)
	}
}

func TestSheetsDuplicateTab_MissingReply(t *testing.T) {
	capture := &sheetsBatchUpdateCapture{}
	svc := newSheetsBatchUpdateTestService(t, map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Data"}}}}, capture)
	ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &SheetsDuplicateTabCmd{}, []string{"s1", "Data", "Backup"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "inspect the spreadsheet before retrying") {
		t.Fatalf("expected incomplete-response guidance, got %v", err)
	}
}

func TestSheetsDuplicateTab_Validation(t *testing.T) {
	for _, args := range [][]string{{"", "Data", "Backup"}, {"s1", "", "Backup"}, {"s1", "Data", ""}, {"s1", "Data", "Backup", "--index=-1"}} {
		if err := runKong(t, &SheetsDuplicateTabCmd{}, args, context.Background(), &RootFlags{}); err == nil {
			t.Fatalf("expected validation error for %v", args)
		}
	}
}
