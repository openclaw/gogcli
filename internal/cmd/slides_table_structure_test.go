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

	"google.golang.org/api/slides/v1"
)

func TestSlidesTableStructure_DryRunRequests(t *testing.T) {
	tests := []struct {
		name string
		op   string
		run  func(context.Context, *RootFlags) error
		want func(*testing.T, *slides.Request)
	}{
		{
			name: "insert rows",
			op:   "slides.table.row.insert",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableRowInsertCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 2, Count: 3, Below: true}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.InsertTableRows
				if got == nil || got.TableObjectId != "table1" || got.CellLocation.RowIndex != 2 || got.CellLocation.ColumnIndex != 0 || !got.InsertBelow || got.Number != 3 {
					t.Fatalf("unexpected insert rows request: %+v", got)
				}
			},
		},
		{
			name: "delete row",
			op:   "slides.table.row.delete",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableRowDeleteCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 1}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.DeleteTableRow
				if got == nil || got.TableObjectId != "table1" || got.CellLocation.RowIndex != 1 || got.CellLocation.ColumnIndex != 0 {
					t.Fatalf("unexpected delete row request: %+v", got)
				}
			},
		},
		{
			name: "insert columns",
			op:   "slides.table.column.insert",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableColumnInsertCmd{PresentationID: "pres1", TableObjectID: "table1", Col: 2, Count: 2, Right: true}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.InsertTableColumns
				if got == nil || got.TableObjectId != "table1" || got.CellLocation.RowIndex != 0 || got.CellLocation.ColumnIndex != 2 || !got.InsertRight || got.Number != 2 {
					t.Fatalf("unexpected insert columns request: %+v", got)
				}
			},
		},
		{
			name: "delete column",
			op:   "slides.table.column.delete",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableColumnDeleteCmd{PresentationID: "pres1", TableObjectID: "table1", Col: 3}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.DeleteTableColumn
				if got == nil || got.TableObjectId != "table1" || got.CellLocation.RowIndex != 0 || got.CellLocation.ColumnIndex != 3 {
					t.Fatalf("unexpected delete column request: %+v", got)
				}
			},
		},
		{
			name: "merge",
			op:   "slides.table.merge",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableMergeCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 1, Col: 2, RowSpan: 2, ColSpan: 3}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.MergeTableCells
				if got == nil || got.ObjectId != "table1" || got.TableRange.Location.RowIndex != 1 || got.TableRange.Location.ColumnIndex != 2 || got.TableRange.RowSpan != 2 || got.TableRange.ColumnSpan != 3 {
					t.Fatalf("unexpected merge request: %+v", got)
				}
			},
		},
		{
			name: "unmerge",
			op:   "slides.table.unmerge",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableUnmergeCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 1, Col: 2, RowSpan: 2, ColSpan: 3}).Run(ctx, flags)
			},
			want: func(t *testing.T, request *slides.Request) {
				t.Helper()
				got := request.UnmergeTableCells
				if got == nil || got.ObjectId != "table1" || got.TableRange.Location.RowIndex != 1 || got.TableRange.Location.ColumnIndex != 2 || got.TableRange.RowSpan != 2 || got.TableRange.ColumnSpan != 3 {
					t.Fatalf("unexpected unmerge request: %+v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			ctx := withSlidesTestServiceFactory(
				newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard),
				func(context.Context, string) (*slides.Service, error) {
					t.Fatal("dry-run must not create a Slides service")
					return nil, context.Canceled
				},
			)
			err := tt.run(ctx, &RootFlags{Account: "a@b.com", DryRun: true, Force: true})
			if err != nil && ExitCode(err) != 0 {
				t.Fatalf("Run: %v", err)
			}
			var got struct {
				Op      string `json:"op"`
				Request struct {
					BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
				} `json:"request"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
			}
			if got.Op != tt.op {
				t.Fatalf("op = %q, want %q", got.Op, tt.op)
			}
			if len(got.Request.BatchUpdate.Requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(got.Request.BatchUpdate.Requests))
			}
			tt.want(t, got.Request.BatchUpdate.Requests[0])
		})
	}
}

func TestSlidesTableStructure_UsesRevisionAndProviderDimensions(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	batchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/presentations/pres1"):
			_ = json.NewEncoder(w).Encode(slidesTableTestPresentation())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			batchCalled = true
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"presentationId": "pres1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newSlidesServiceFromServer(t, srv))
	cmd := &SlidesTableRowInsertCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 2, Count: 1, Below: true}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !batchCalled {
		t.Fatal("batch update was not called")
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %+v, want rev1", captured.WriteControl)
	}
}

func TestSlidesTableStructure_RejectsProviderOutOfBounds(t *testing.T) {
	batchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(slidesTableTestPresentation())
			return
		}
		batchCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"presentationId": "pres1"})
	}))
	defer srv.Close()

	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newSlidesServiceFromServer(t, srv))
	cmd := &SlidesTableMergeCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 2, Col: 1, RowSpan: 2, ColSpan: 2}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "--row-span exceeds table row count 3 from row 2") {
		t.Fatalf("unexpected error: %v", err)
	}
	if batchCalled {
		t.Fatal("out-of-bounds request reached batchUpdate")
	}
}

func TestSlidesTableStructure_LocalValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *RootFlags) error
		want string
	}{
		{name: "empty presentation", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableRowInsertCmd{TableObjectID: "table1", Row: 0, Count: 1}).Run(ctx, flags)
		}, want: "empty presentationId"},
		{name: "empty table", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableRowInsertCmd{PresentationID: "pres1", Row: 0, Count: 1}).Run(ctx, flags)
		}, want: "empty tableObjectId"},
		{name: "negative row", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableRowInsertCmd{PresentationID: "pres1", TableObjectID: "table1", Row: -1, Count: 1}).Run(ctx, flags)
		}, want: "--row must be >= 0"},
		{name: "too many", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableColumnInsertCmd{PresentationID: "pres1", TableObjectID: "table1", Col: 0, Count: 21}).Run(ctx, flags)
		}, want: "--count must be between 1 and 20"},
		{name: "negative column", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableColumnDeleteCmd{PresentationID: "pres1", TableObjectID: "table1", Col: -1}).Run(ctx, flags)
		}, want: "--col must be >= 0"},
		{name: "single-cell merge", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableMergeCmd{PresentationID: "pres1", TableObjectID: "table1", RowSpan: 1, ColSpan: 1}).Run(ctx, flags)
		}, want: "merge range must span more than one cell"},
		{name: "bad unmerge span", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableUnmergeCmd{PresentationID: "pres1", TableObjectID: "table1", RowSpan: 0, ColSpan: 1}).Run(ctx, flags)
		}, want: "--row-span must be >= 1"},
	}

	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("invalid command must not create a Slides service")
			return nil, context.Canceled
		},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(ctx, &RootFlags{Account: "a@b.com", Force: true})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func slidesTableTestPresentation() map[string]any {
	return map[string]any{
		"presentationId": "pres1",
		"revisionId":     "rev1",
		"slides": []any{map[string]any{
			"objectId": "slide1",
			"pageElements": []any{map[string]any{
				"objectId": "table1",
				"table": map[string]any{
					"rows":    3,
					"columns": 4,
				},
			}},
		}},
	}
}
