package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestSheetsConditionalAddBuildsRule(t *testing.T) {
	ctx, flags, requests, rawRequests, cleanup := newSheetsAdvancedTestContext(t, sheetsAdvancedTestState{})
	defer cleanup()

	if err := runKong(t, &SheetsConditionalAddCmd{}, []string{
		"s1", "Sheet1!A2:C10",
		"--type", "text-eq",
		"--expr", "A",
		"--format-json", `{"backgroundColor":{"red":1,"green":0.8}}`,
		"--format-fields", "backgroundColor",
	}, ctx, flags); err != nil {
		t.Fatalf("conditional add: %v", err)
	}

	if len(*requests) != 1 || len((*requests)[0].Requests) != 1 {
		t.Fatalf("requests = %#v", *requests)
	}
	add := (*requests)[0].Requests[0].AddConditionalFormatRule
	if add == nil || add.Rule == nil || add.Rule.BooleanRule == nil {
		t.Fatalf("missing addConditionalFormatRule: %#v", (*requests)[0].Requests[0])
	}
	if add.Index != 0 {
		t.Fatalf("index = %d, want 0", add.Index)
	}
	condition := add.Rule.BooleanRule.Condition
	if condition.Type != "TEXT_EQ" || len(condition.Values) != 1 || condition.Values[0].UserEnteredValue != "A" {
		t.Fatalf("condition = %#v", condition)
	}
	if got := add.Rule.Ranges[0]; got.SheetId != 0 || got.StartRowIndex != 1 || got.EndRowIndex != 10 || got.StartColumnIndex != 0 || got.EndColumnIndex != 3 {
		t.Fatalf("range = %#v", got)
	}
	if add.Rule.BooleanRule.Format == nil || add.Rule.BooleanRule.Format.BackgroundColor == nil {
		t.Fatalf("missing format: %#v", add.Rule.BooleanRule.Format)
	}
	if !strings.Contains((*rawRequests)[0], `"sheetId":0`) {
		t.Fatalf("request did not force-send zero sheetId: %s", (*rawRequests)[0])
	}
	if !strings.Contains((*rawRequests)[0], `"backgroundColor"`) {
		t.Fatalf("request missing backgroundColor: %s", (*rawRequests)[0])
	}
}

func TestSheetsConditionalClearAllDeletesReverseAndRequiresForce(t *testing.T) {
	ctx, flags, requests, _, cleanup := newSheetsAdvancedTestContext(t, sheetsAdvancedTestState{
		ConditionalRules: 2,
	})
	defer cleanup()

	err := runKong(t, &SheetsConditionalClearCmd{}, []string{"s1", "--sheet", "Sheet1", "--all"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected force error, got %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("clear ran without force: %#v", *requests)
	}

	forceFlags := *flags
	forceFlags.Force = true
	if err := runKong(t, &SheetsConditionalClearCmd{}, []string{"s1", "--sheet", "Sheet1", "--all"}, ctx, &forceFlags); err != nil {
		t.Fatalf("conditional clear: %v", err)
	}
	if len(*requests) != 1 || len((*requests)[0].Requests) != 2 {
		t.Fatalf("requests = %#v", *requests)
	}
	first := (*requests)[0].Requests[0].DeleteConditionalFormatRule
	second := (*requests)[0].Requests[1].DeleteConditionalFormatRule
	if first == nil || second == nil {
		t.Fatalf("missing deleteConditionalFormatRule: %#v", (*requests)[0].Requests)
	}
	if first.Index != 1 || second.Index != 0 {
		t.Fatalf("delete indexes = %d,%d; want 1,0", first.Index, second.Index)
	}
}

func TestSheetsBandingSetListAndClear(t *testing.T) {
	ctx, flags, requests, _, cleanup := newSheetsAdvancedTestContext(t, sheetsAdvancedTestState{
		BandedRangeID: 777,
	})
	defer cleanup()

	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsBandingSetCmd{}, []string{"s1", "Sheet1!A1:C5"}, ctx, flags); err != nil {
			t.Fatalf("banding set: %v", err)
		}
	})
	if !strings.Contains(out, `"bandedRangeId": 777`) {
		t.Fatalf("missing banded range id output: %s", out)
	}
	if len(*requests) != 1 || (*requests)[0].Requests[0].AddBanding == nil {
		t.Fatalf("missing addBanding request: %#v", *requests)
	}
	add := (*requests)[0].Requests[0].AddBanding.BandedRange
	if add.Range.SheetId != 0 || add.Range.StartRowIndex != 0 || add.Range.EndRowIndex != 5 || add.Range.EndColumnIndex != 3 {
		t.Fatalf("add range = %#v", add.Range)
	}
	if add.RowProperties == nil || add.RowProperties.FirstBandColorStyle == nil || add.RowProperties.SecondBandColorStyle == nil {
		t.Fatalf("missing default row banding properties: %#v", add.RowProperties)
	}

	listOut := captureStdout(t, func() {
		if err := runKong(t, &SheetsBandingListCmd{}, []string{"s1"}, ctx, flags); err != nil {
			t.Fatalf("banding list: %v", err)
		}
	})
	if !strings.Contains(listOut, `"bandedRangeId": 777`) || !strings.Contains(listOut, `"a1": "Sheet1!A1:C5"`) {
		t.Fatalf("missing banding list output: %s", listOut)
	}

	err := runKong(t, &SheetsBandingClearCmd{}, []string{"s1", "--id", "777"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected force error, got %v", err)
	}

	forceFlags := *flags
	forceFlags.Force = true
	if err := runKong(t, &SheetsBandingClearCmd{}, []string{"s1", "--id", "777"}, ctx, &forceFlags); err != nil {
		t.Fatalf("banding clear: %v", err)
	}
	if len(*requests) != 2 || (*requests)[1].Requests[0].DeleteBanding == nil {
		t.Fatalf("missing deleteBanding request: %#v", *requests)
	}
	if (*requests)[1].Requests[0].DeleteBanding.BandedRangeId != 777 {
		t.Fatalf("delete banding id = %d", (*requests)[1].Requests[0].DeleteBanding.BandedRangeId)
	}
}

type sheetsAdvancedTestState struct {
	ConditionalRules int
	BandedRangeID    int64
}

func newSheetsAdvancedTestContext(t *testing.T, state sheetsAdvancedTestState) (context.Context, *RootFlags, *[]sheets.BatchUpdateSpreadsheetRequest, *[]string, func()) {
	t.Helper()

	origNew := newSheetsService
	requests := []sheets.BatchUpdateSpreadsheetRequest{}
	rawRequests := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			writeSheetsAdvancedMetadata(t, w, state)
		case path == "/spreadsheets/s1:batchUpdate" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batchUpdate: %v", err)
			}
			rawRequests = append(rawRequests, string(body))
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			requests = append(requests, req)
			writeSheetsAdvancedBatchReply(t, w, req, state)
		default:
			http.NotFound(w, r)
		}
	}))

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "a@b.com"}
	cleanup := func() {
		newSheetsService = origNew
		srv.Close()
	}
	return ctx, flags, &requests, &rawRequests, cleanup
}

func writeSheetsAdvancedMetadata(t *testing.T, w http.ResponseWriter, state sheetsAdvancedTestState) {
	t.Helper()
	rules := make([]map[string]any, 0, state.ConditionalRules)
	for i := 0; i < state.ConditionalRules; i++ {
		rules = append(rules, map[string]any{
			"booleanRule": map[string]any{
				"condition": map[string]any{
					"type":   "TEXT_EQ",
					"values": []map[string]any{{"userEnteredValue": "A"}},
				},
			},
			"ranges": []map[string]any{{
				"sheetId":          0,
				"startRowIndex":    1,
				"endRowIndex":      5,
				"startColumnIndex": 0,
				"endColumnIndex":   3,
			}},
		})
	}
	banded := []map[string]any{}
	if state.BandedRangeID != 0 {
		banded = append(banded, map[string]any{
			"bandedRangeId": state.BandedRangeID,
			"range": map[string]any{
				"sheetId":          0,
				"startRowIndex":    0,
				"endRowIndex":      5,
				"startColumnIndex": 0,
				"endColumnIndex":   3,
			},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"spreadsheetId": "s1",
		"sheets": []map[string]any{{
			"properties":         map[string]any{"sheetId": 0, "title": "Sheet1"},
			"conditionalFormats": rules,
			"bandedRanges":       banded,
		}},
	})
}

func writeSheetsAdvancedBatchReply(t *testing.T, w http.ResponseWriter, req sheets.BatchUpdateSpreadsheetRequest, state sheetsAdvancedTestState) {
	t.Helper()
	replies := make([]map[string]any, 0, len(req.Requests))
	for _, r := range req.Requests {
		switch {
		case r.AddBanding != nil:
			replies = append(replies, map[string]any{
				"addBanding": map[string]any{
					"bandedRange": map[string]any{"bandedRangeId": state.BandedRangeID},
				},
			})
		default:
			replies = append(replies, map[string]any{})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"spreadsheetId": "s1",
		"replies":       replies,
	})
}
