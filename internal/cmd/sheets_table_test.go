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
)

func TestSheetsTableCreateCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotReq sheets.BatchUpdateSpreadsheetRequest
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batchUpdate: %v", err)
			}
			gotBody = string(body)
			if err := json.Unmarshal(body, &gotReq); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"replies": []map[string]any{
					{
						"addTable": map[string]any{
							"table": map[string]any{
								"tableId": "tbl1",
								"name":    "Tasks",
								"range": map[string]any{
									"sheetId":          42,
									"startRowIndex":    0,
									"endRowIndex":      4,
									"startColumnIndex": 0,
									"endColumnIndex":   3,
								},
								"columnProperties": []map[string]any{
									{"columnIndex": 0, "columnName": "Task", "columnType": "TEXT"},
									{"columnIndex": 1, "columnName": "Amount", "columnType": "DOUBLE"},
									{"columnIndex": 2, "columnName": "Done", "columnType": "BOOLEAN"},
								},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	installSheetsTestService(t, srv)

	ctx := newCmdJSONContext(t)
	cmd := &SheetsTableCreateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{
			"s1",
			"Sheet1!A1:C4",
			"--name", "Tasks",
			"--columns-json", `[{"columnName":"Task"},{"columnName":"Amount","columnType":"DOUBLE"},{"columnName":"Done","columnType":"BOOLEAN"}]`,
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("create table: %v", err)
		}
	})

	if len(gotReq.Requests) != 1 || gotReq.Requests[0].AddTable == nil || gotReq.Requests[0].AddTable.Table == nil {
		t.Fatalf("expected addTable request, got %#v", gotReq.Requests)
	}
	table := gotReq.Requests[0].AddTable.Table
	if table.Name != "Tasks" {
		t.Fatalf("table name = %q", table.Name)
	}
	if table.Range == nil || table.Range.SheetId != 42 || table.Range.EndRowIndex != 4 || table.Range.EndColumnIndex != 3 {
		t.Fatalf("range = %#v", table.Range)
	}
	if len(table.ColumnProperties) != 3 {
		t.Fatalf("columns = %#v", table.ColumnProperties)
	}
	if table.ColumnProperties[0].ColumnType != "TEXT" {
		t.Fatalf("default column type = %q", table.ColumnProperties[0].ColumnType)
	}
	if table.ColumnProperties[1].ColumnType != "DOUBLE" || table.ColumnProperties[2].ColumnType != "BOOLEAN" {
		t.Fatalf("column types = %#v", table.ColumnProperties)
	}
	if !strings.Contains(gotBody, `"columnIndex":0`) {
		t.Fatalf("expected zero columnIndex to be sent, body: %s", gotBody)
	}
	if !strings.Contains(out, `"tableId": "tbl1"`) {
		t.Fatalf("missing JSON table id: %s", out)
	}
}

func TestSheetsTableColumnTypeAliasesFailFast(t *testing.T) {
	tests := map[string]string{
		"NUMBER":     "use DOUBLE",
		"CHECKBOX":   "use BOOLEAN",
		"RATING":     "use RATINGS_CHIP",
		"SMART_CHIP": "use FILES_CHIP",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseSheetsTableColumnsJSON(`[{"columnName":"Value","columnType":"` + input + `"}]`)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestSheetsTableListGetDelete(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var deletedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{
						"properties": map[string]any{"sheetId": 42, "title": "Sheet1"},
						"tables": []map[string]any{
							{
								"tableId": "tbl1",
								"name":    "Tasks",
								"range": map[string]any{
									"sheetId":          42,
									"startRowIndex":    0,
									"endRowIndex":      4,
									"startColumnIndex": 0,
									"endColumnIndex":   3,
								},
								"columnProperties": []map[string]any{
									{"columnIndex": 0, "columnName": "Task", "columnType": "TEXT"},
								},
							},
						},
					},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].DeleteTable == nil {
				t.Fatalf("expected deleteTable request, got %#v", req.Requests)
			}
			deletedID = req.Requests[0].DeleteTable.TableId
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	installSheetsTestService(t, srv)

	listOut := captureStdout(t, func() {
		if err := (&SheetsTableListCmd{SpreadsheetID: "s1"}).Run(newCmdJSONContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("list tables: %v", err)
		}
	})
	if !strings.Contains(listOut, `"a1": "Sheet1!A1:C4"`) {
		t.Fatalf("missing A1 output: %s", listOut)
	}

	getOut := captureStdout(t, func() {
		if err := (&SheetsTableGetCmd{SpreadsheetID: "s1", TableID: "Tasks"}).Run(newCmdJSONContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("get table: %v", err)
		}
	})
	if !strings.Contains(getOut, `"tableId": "tbl1"`) {
		t.Fatalf("missing table output: %s", getOut)
	}

	deleteOut := captureStdout(t, func() {
		if err := (&SheetsTableDeleteCmd{SpreadsheetID: "s1", TableID: "tbl1"}).Run(newCmdJSONContext(t), &RootFlags{Account: "a@b.com", Force: true}); err != nil {
			t.Fatalf("delete table: %v", err)
		}
	})
	if deletedID != "tbl1" {
		t.Fatalf("deleted table id = %q", deletedID)
	}
	if !strings.Contains(deleteOut, `"tableId": "tbl1"`) {
		t.Fatalf("missing delete output: %s", deleteOut)
	}
}

func TestSheetsTableAppendCmd(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	var gotRange string
	var gotInsert string
	var gotInput string
	var gotValues sheets.ValueRange

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{
						"properties": map[string]any{"sheetId": 42, "title": "Sheet1"},
						"tables": []map[string]any{
							{
								"tableId": "tbl1",
								"name":    "Tasks",
								"range": map[string]any{
									"sheetId":          42,
									"startRowIndex":    0,
									"endRowIndex":      4,
									"startColumnIndex": 0,
									"endColumnIndex":   3,
								},
								"columnProperties": []map[string]any{
									{"columnIndex": 0, "columnName": "Task", "columnType": "TEXT"},
									{"columnIndex": 1, "columnName": "Amount", "columnType": "DOUBLE"},
									{"columnIndex": 2, "columnName": "Done", "columnType": "BOOLEAN"},
								},
							},
						},
					},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			gotRange = strings.TrimSuffix(strings.TrimPrefix(path, "/spreadsheets/s1/values/"), ":append")
			gotInsert = r.URL.Query().Get("insertDataOption")
			gotInput = r.URL.Query().Get("valueInputOption")
			if err := json.NewDecoder(r.Body).Decode(&gotValues); err != nil {
				t.Fatalf("decode append: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{
					"updatedRange":   "Sheet1!A4:C4",
					"updatedRows":    1,
					"updatedColumns": 3,
					"updatedCells":   3,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	installSheetsTestService(t, srv)

	out := captureStdout(t, func() {
		cmd := &SheetsTableAppendCmd{}
		if err := runKong(t, cmd, []string{
			"s1",
			"Tasks",
			"--values-json", `[["Write docs",2,true]]`,
		}, newCmdJSONContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("append table: %v", err)
		}
	})

	if gotRange != "Sheet1!A1:C4" {
		t.Fatalf("append range = %q", gotRange)
	}
	if gotInsert != "INSERT_ROWS" {
		t.Fatalf("insertDataOption = %q", gotInsert)
	}
	if gotInput != sheetsDefaultValueInputOption {
		t.Fatalf("valueInputOption = %q", gotInput)
	}
	if len(gotValues.Values) != 1 || len(gotValues.Values[0]) != 3 {
		t.Fatalf("values = %#v", gotValues.Values)
	}
	if !strings.Contains(out, `"tableId": "tbl1"`) || !strings.Contains(out, `"updatedRange": "Sheet1!A4:C4"`) {
		t.Fatalf("missing append output: %s", out)
	}
}

func TestSheetsTableAppendRejectsTooWideRows(t *testing.T) {
	table := sheetsTableItem{
		Name: "Tasks",
		Columns: []sheetsTableColumnItem{
			{ColumnIndex: 0, ColumnName: "Task"},
			{ColumnIndex: 1, ColumnName: "Done"},
		},
	}
	err := validateSheetsTableAppendWidth(table, [][]interface{}{{"a", "b", "c"}})
	if err == nil {
		t.Fatal("expected width error")
	}
	if !strings.Contains(err.Error(), "has 3 cells") {
		t.Fatalf("error = %q", err.Error())
	}
}

func installSheetsTestService(t *testing.T, srv *httptest.Server) {
	t.Helper()

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }
}
