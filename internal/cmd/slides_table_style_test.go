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

func TestSlidesTableStyle_DryRunRequests(t *testing.T) {
	middle := "MIDDLE"
	dash := "DASH"
	weight := 2.5
	tests := []struct {
		name string
		op   string
		run  func(context.Context, *RootFlags) error
		want func(*testing.T, []*slides.Request)
	}{
		{
			name: "row size",
			op:   "slides.table.row.size",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableRowSizeCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 2, Height: 48}).Run(ctx, flags)
			},
			want: func(t *testing.T, requests []*slides.Request) {
				t.Helper()
				if len(requests) != 1 || requests[0].UpdateTableRowProperties == nil {
					t.Fatalf("unexpected requests: %+v", requests)
				}
				got := requests[0].UpdateTableRowProperties
				if got.ObjectId != "table1" || len(got.RowIndices) != 1 || got.RowIndices[0] != 2 || got.Fields != "minRowHeight" {
					t.Fatalf("unexpected row size request: %+v", got)
				}
				if got.TableRowProperties.MinRowHeight.Magnitude != 48 || got.TableRowProperties.MinRowHeight.Unit != "PT" {
					t.Fatalf("unexpected row height: %+v", got.TableRowProperties.MinRowHeight)
				}
			},
		},
		{
			name: "column size",
			op:   "slides.table.column.size",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableColumnSizeCmd{PresentationID: "pres1", TableObjectID: "table1", Col: 1, Width: 120}).Run(ctx, flags)
			},
			want: func(t *testing.T, requests []*slides.Request) {
				t.Helper()
				if len(requests) != 1 || requests[0].UpdateTableColumnProperties == nil {
					t.Fatalf("unexpected requests: %+v", requests)
				}
				got := requests[0].UpdateTableColumnProperties
				if got.ObjectId != "table1" || len(got.ColumnIndices) != 1 || got.ColumnIndices[0] != 1 || got.Fields != "columnWidth" {
					t.Fatalf("unexpected column size request: %+v", got)
				}
				if got.TableColumnProperties.ColumnWidth.Magnitude != 120 || got.TableColumnProperties.ColumnWidth.Unit != "PT" {
					t.Fatalf("unexpected column width: %+v", got.TableColumnProperties.ColumnWidth)
				}
			},
		},
		{
			name: "cell and text style",
			op:   "slides.table.cell.style",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableCellStyleCmd{
					PresentationID: "pres1",
					TableObjectID:  "table1",
					Row:            1,
					Col:            2,
					FillColor:      "#3367D6",
					ContentAlign:   &middle,
					Bold:           true,
					TextColor:      "#FFFFFF",
					Size:           18,
					Font:           "Cambria",
				}).Run(ctx, flags)
			},
			want: func(t *testing.T, requests []*slides.Request) {
				t.Helper()
				if len(requests) != 2 {
					t.Fatalf("requests = %d, want 2", len(requests))
				}
				cell := requests[0].UpdateTableCellProperties
				if cell == nil || cell.ObjectId != "table1" || cell.TableRange.Location.RowIndex != 1 || cell.TableRange.Location.ColumnIndex != 2 {
					t.Fatalf("unexpected cell style request: %+v", cell)
				}
				if cell.TableCellProperties.ContentAlignment != "MIDDLE" || cell.TableCellProperties.TableCellBackgroundFill.PropertyState != "RENDERED" {
					t.Fatalf("unexpected cell properties: %+v", cell.TableCellProperties)
				}
				text := requests[1].UpdateTextStyle
				if text == nil || text.CellLocation.RowIndex != 1 || text.CellLocation.ColumnIndex != 2 || text.TextRange.Type != slidesTableAll {
					t.Fatalf("unexpected text style request: %+v", text)
				}
				if !text.Style.Bold || text.Style.FontFamily != "Cambria" || text.Style.FontSize.Magnitude != 18 {
					t.Fatalf("unexpected text properties: %+v", text.Style)
				}
			},
		},
		{
			name: "border style",
			op:   "slides.table.border.style",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&SlidesTableBorderStyleCmd{
					PresentationID: "pres1",
					TableObjectID:  "table1",
					Row:            0,
					Col:            1,
					RowSpan:        2,
					ColSpan:        3,
					Position:       "OUTER",
					BorderColor:    "#EA4335",
					Weight:         &weight,
					Dash:           &dash,
				}).Run(ctx, flags)
			},
			want: func(t *testing.T, requests []*slides.Request) {
				t.Helper()
				if len(requests) != 1 || requests[0].UpdateTableBorderProperties == nil {
					t.Fatalf("unexpected requests: %+v", requests)
				}
				got := requests[0].UpdateTableBorderProperties
				if got.ObjectId != "table1" || got.BorderPosition != "OUTER" || got.TableRange.RowSpan != 2 || got.TableRange.ColumnSpan != 3 {
					t.Fatalf("unexpected border request: %+v", got)
				}
				if got.TableBorderProperties.Weight.Magnitude != weight || got.TableBorderProperties.DashStyle != dash {
					t.Fatalf("unexpected border properties: %+v", got.TableBorderProperties)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runSlidesTableStyleDryRun(t, tt.run)
			if got.Op != tt.op {
				t.Fatalf("op = %q, want %q", got.Op, tt.op)
			}
			tt.want(t, got.Request.BatchUpdate.Requests)
		})
	}
}

func TestSlidesTableCellStyle_UsesRevisionForAtomicBatch(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(slidesTableTestPresentation())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
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
	cmd := &SlidesTableCellStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0, FillColor: "#000000", Bold: true}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured.Requests) != 2 || captured.Requests[0].UpdateTableCellProperties == nil || captured.Requests[1].UpdateTextStyle == nil {
		t.Fatalf("unexpected atomic batch: %+v", captured.Requests)
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %+v, want rev1", captured.WriteControl)
	}
}

func TestSlidesTableStyle_LocalValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *RootFlags) error
		want string
	}{
		{name: "negative row height", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableRowSizeCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Height: -1}).Run(ctx, flags)
		}, want: "--height must be >= 0"},
		{name: "column below provider minimum", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableColumnSizeCmd{PresentationID: "pres1", TableObjectID: "table1", Col: 0, Width: 31.99}).Run(ctx, flags)
		}, want: "--width must be >= 32 points"},
		{name: "cell style missing options", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableCellStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0}).Run(ctx, flags)
		}, want: "provide at least one cell or text style option"},
		{name: "cell fill conflict", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableCellStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0, FillColor: "#fff", FillTransparent: true}).Run(ctx, flags)
		}, want: "mutually exclusive"},
		{name: "text style conflict", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableCellStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0, Bold: true, NoBold: true}).Run(ctx, flags)
		}, want: "--bold and --no-bold are mutually exclusive"},
		{name: "border missing options", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableBorderStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0, RowSpan: 1, ColSpan: 1, Position: slidesTableAll}).Run(ctx, flags)
		}, want: "provide at least one border style option"},
		{name: "border color conflict", run: func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesTableBorderStyleCmd{PresentationID: "pres1", TableObjectID: "table1", Row: 0, Col: 0, RowSpan: 1, ColSpan: 1, Position: slidesTableAll, BorderColor: "#fff", Transparent: true}).Run(ctx, flags)
		}, want: "mutually exclusive"},
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

type slidesTableStyleDryRun struct {
	Op      string `json:"op"`
	Request struct {
		BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
	} `json:"request"`
}

func runSlidesTableStyleDryRun(t *testing.T, run func(context.Context, *RootFlags) error) slidesTableStyleDryRun {
	t.Helper()
	var stdout bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("dry-run must not create a Slides service")
			return nil, context.Canceled
		},
	)
	err := run(ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	if err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}
	var got slidesTableStyleDryRun
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	return got
}
