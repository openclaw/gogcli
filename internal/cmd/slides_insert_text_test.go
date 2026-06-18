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

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

// mockSlidesBatchUpdateServer spins up an httptest.Server that captures the
// batchUpdate request body and returns a canned BatchUpdatePresentationResponse.
// Tests can inspect captured requests via the returned pointer.
func mockSlidesBatchUpdateServer(
	t *testing.T,
	captured *[]*slides.Request,
	response map[string]any,
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost {
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				*captured = req.Requests
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	return srv
}

func mockSlidesPresentationBatchUpdateServer(
	t *testing.T,
	captured *slides.BatchUpdatePresentationRequest,
	pres *slides.Presentation,
	response map[string]any,
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				*captured = req
			}
			_ = json.NewEncoder(w).Encode(response)
		case strings.Contains(r.URL.Path, "/presentations/") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(pres)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func newSlidesServiceFromServer(t *testing.T, srv *httptest.Server) *slides.Service {
	t.Helper()
	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	return svc
}

func slidesPresentationWithTableCellText(rowIndex, columnIndex int, content string) *slides.Presentation {
	rows := make([]*slides.TableRow, rowIndex+1)
	for r := range rows {
		cells := make([]*slides.TableCell, columnIndex+1)
		for c := range cells {
			cells[c] = &slides.TableCell{}
		}
		rows[r] = &slides.TableRow{TableCells: cells}
	}
	rows[rowIndex].TableCells[columnIndex].Text = &slides.TextContent{
		TextElements: []*slides.TextElement{{
			StartIndex: 0,
			EndIndex:   utf16Len(content),
			TextRun:    &slides.TextRun{Content: content},
		}},
	}
	return &slides.Presentation{
		RevisionId: "rev1",
		Slides: []*slides.Page{{
			ObjectId: "slide_1",
			PageElements: []*slides.PageElement{{
				ObjectId: "table_1",
				Table: &slides.Table{
					Rows:      int64(rowIndex + 1),
					Columns:   int64(columnIndex + 1),
					TableRows: rows,
				},
			}},
		}},
	}
}

func TestSlidesInsertText(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
		"writeControl":   map[string]any{"requiredRevisionId": "rev-123"},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "hello world",
		InsertionIndex: 3,
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "ok | revisionId=rev-123 | replies=1") {
		t.Errorf("expected plain confirmation with revisionId and replies, got: %q", out.String())
	}
	if len(captured) != 1 {
		t.Fatalf("expected 1 request in batch, got %d", len(captured))
	}
	if captured[0].InsertText == nil {
		t.Fatal("expected InsertText request")
	}
	if captured[0].InsertText.Text != "hello world" {
		t.Errorf("expected text %q, got %q", "hello world", captured[0].InsertText.Text)
	}
	if captured[0].InsertText.ObjectId != "shape_1" {
		t.Errorf("expected objectId shape_1, got %q", captured[0].InsertText.ObjectId)
	}
	if captured[0].InsertText.InsertionIndex != 3 {
		t.Errorf("expected insertionIndex 3, got %d", captured[0].InsertText.InsertionIndex)
	}
}

func TestSlidesInsertText_ReplaceEmitsDeleteThenInsert(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}, map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "replacement",
		Replace:        true,
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected 2 requests (DeleteText + InsertText), got %d", len(captured))
	}
	if captured[0].DeleteText == nil {
		t.Error("expected first request to be DeleteText")
	} else if captured[0].DeleteText.TextRange == nil || captured[0].DeleteText.TextRange.Type != "ALL" {
		t.Errorf("expected DeleteText TextRange.Type=ALL, got %+v", captured[0].DeleteText.TextRange)
	}
	if captured[1].InsertText == nil {
		t.Error("expected second request to be InsertText")
	} else if captured[1].InsertText.Text != "replacement" {
		t.Errorf("expected inserted text %q, got %q", "replacement", captured[1].InsertText.Text)
	}
}

func TestSlidesInsertText_CellLocation(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	row, col := int64(0), int64(2)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "cell text",
		Row:            &row,
		Col:            &col,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 || captured[0].InsertText == nil {
		t.Fatalf("expected single InsertText request, got %+v", captured)
	}
	insert := captured[0].InsertText
	if insert.ObjectId != "table_1" || insert.Text != "cell text" {
		t.Fatalf("unexpected insert text request: %+v", insert)
	}
	if insert.CellLocation == nil || insert.CellLocation.RowIndex != 0 || insert.CellLocation.ColumnIndex != 2 {
		t.Fatalf("unexpected cell location: %+v", insert.CellLocation)
	}
}

func TestSlidesInsertText_CellReplaceEmitsCellDeleteThenInsert(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	pres := slidesPresentationWithTableCellText(1, 0, "old value\n")
	srv := mockSlidesPresentationBatchUpdateServer(
		t,
		&captured,
		pres,
		map[string]any{
			"presentationId": "pres1",
			"replies":        []any{map[string]any{}, map[string]any{}},
		},
	)
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	row, col := int64(1), int64(0)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "replacement",
		Replace:        true,
		Row:            &row,
		Col:            &col,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured.Requests) != 2 {
		t.Fatalf("expected DeleteText + InsertText, got %+v", captured.Requests)
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %+v, want required revision rev1", captured.WriteControl)
	}
	if captured.Requests[0].DeleteText == nil || captured.Requests[0].DeleteText.CellLocation == nil {
		t.Fatalf("expected cell-targeted DeleteText, got %+v", captured.Requests[0])
	}
	if captured.Requests[0].DeleteText.CellLocation.RowIndex != 1 || captured.Requests[0].DeleteText.CellLocation.ColumnIndex != 0 {
		t.Fatalf("unexpected delete cell location: %+v", captured.Requests[0].DeleteText.CellLocation)
	}
	if captured.Requests[1].InsertText == nil || captured.Requests[1].InsertText.CellLocation == nil {
		t.Fatalf("expected cell-targeted InsertText, got %+v", captured.Requests[1])
	}
	if captured.Requests[1].InsertText.CellLocation.RowIndex != 1 || captured.Requests[1].InsertText.CellLocation.ColumnIndex != 0 {
		t.Fatalf("unexpected insert cell location: %+v", captured.Requests[1].InsertText.CellLocation)
	}
}

func TestSlidesInsertText_CellReplaceSkipsDeleteForBlankCell(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	pres := slidesPresentationWithTableCellText(0, 2, "\n")
	srv := mockSlidesPresentationBatchUpdateServer(
		t,
		&captured,
		pres,
		map[string]any{
			"presentationId": "pres1",
			"replies":        []any{map[string]any{}},
		},
	)
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	row, col := int64(0), int64(2)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "first text",
		Replace:        true,
		Row:            &row,
		Col:            &col,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured.Requests) != 1 || captured.Requests[0].InsertText == nil {
		t.Fatalf("expected only InsertText for blank cell, got %+v", captured.Requests)
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %+v, want required revision rev1", captured.WriteControl)
	}
	if captured.Requests[0].InsertText.CellLocation == nil ||
		captured.Requests[0].InsertText.CellLocation.RowIndex != 0 ||
		captured.Requests[0].InsertText.CellLocation.ColumnIndex != 2 {
		t.Fatalf("unexpected insert cell location: %+v", captured.Requests[0].InsertText.CellLocation)
	}
	if captured.Requests[0].InsertText.Text != "first text" {
		t.Fatalf("inserted text = %q", captured.Requests[0].InsertText.Text)
	}
}

func TestSlidesInsertText_CellReplaceBlankWithEmptyIsNoOp(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	pres := slidesPresentationWithTableCellText(0, 0, "\n")
	srv := mockSlidesPresentationBatchUpdateServer(t, &captured, pres, map[string]any{})
	defer srv.Close()

	var stdout bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), newSlidesServiceFromServer(t, srv))
	row, col := int64(0), int64(0)
	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "",
		Replace:        true,
		Row:            &row,
		Col:            &col,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured.Requests) != 0 {
		t.Fatalf("blank clear should not send batchUpdate, got %+v", captured.Requests)
	}
	if !strings.Contains(stdout.String(), `"replies": []`) {
		t.Fatalf("unexpected no-op output: %s", stdout.String())
	}
}

func TestSlidesInsertText_CellReplaceRejectsMissingCell(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	pres := slidesPresentationWithTableCellText(0, 0, "\n")
	srv := mockSlidesPresentationBatchUpdateServer(t, &captured, pres, map[string]any{})
	defer srv.Close()

	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newSlidesServiceFromServer(t, srv))
	row, col := int64(1), int64(0)
	err := (&SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "x",
		Replace:        true,
		Row:            &row,
		Col:            &col,
	}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "table cell table_1[1,0] not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Requests) != 0 {
		t.Fatalf("missing cell should not send batchUpdate, got %+v", captured.Requests)
	}
}

func TestSlidesInsertText_ReplaceEmptyClearsOnly(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "",
		Replace:        true,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected 1 DeleteText request, got %d", len(captured))
	}
	if captured[0].DeleteText == nil {
		t.Fatalf("expected DeleteText request, got %+v", captured[0])
	}
}

func TestSlidesInsertText_StdinDash(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	const piped = "from-stdin content\nline 2\n"
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(
		newCmdRuntimeIOContext(t, strings.NewReader(piped), io.Discard, io.Discard),
		svc,
	)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "-",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 || captured[0].InsertText == nil {
		t.Fatalf("expected single InsertText request, got %+v", captured)
	}
	if captured[0].InsertText.Text != piped {
		t.Errorf("expected piped text %q, got %q", piped, captured[0].InsertText.Text)
	}
}

func TestSlidesInsertText_DryRunNoAPICall(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "dry",
		Replace:        true,
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	if !got.DryRun {
		t.Fatalf("expected dry_run=true, got %#v", got)
	}
	body := got.Request.BatchUpdate
	if len(body.Requests) != 2 {
		t.Fatalf("expected 2 requests in dry-run body, got %d", len(body.Requests))
	}
	if body.Requests[0].DeleteText == nil || body.Requests[1].InsertText == nil {
		t.Errorf("expected DeleteText then InsertText in dry-run body, got %+v", body.Requests)
	}
}

func TestSlidesInsertText_DryRunCellLocationIncludesZeroIndexes(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	row, col := int64(0), int64(0)
	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "table_1",
		Text:           "dry",
		Row:            &row,
		Col:            &col,
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, out.String())
	}
	insert := got.Request.BatchUpdate.Requests[0].InsertText
	if insert == nil || insert.CellLocation == nil || insert.CellLocation.RowIndex != 0 || insert.CellLocation.ColumnIndex != 0 {
		t.Fatalf("dry-run output should include zero cell indexes, got: %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesInsertText_InvalidInsertionIndexIsUsageErrorBeforeDryRun(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "dry",
		InsertionIndex: -1,
	}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected insertion-index error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestSlidesInsertText_InvalidCellFlags(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)
	zero, neg := int64(0), int64(-1)
	cases := []struct {
		name string
		cmd  SlidesInsertTextCmd
		want string
	}{
		{name: "row without col", cmd: SlidesInsertTextCmd{PresentationID: "pres1", ObjectID: "table_1", Text: "x", Row: &zero}, want: "--row and --col"},
		{name: "col without row", cmd: SlidesInsertTextCmd{PresentationID: "pres1", ObjectID: "table_1", Text: "x", Col: &zero}, want: "--row and --col"},
		{name: "negative row", cmd: SlidesInsertTextCmd{PresentationID: "pres1", ObjectID: "table_1", Text: "x", Row: &neg, Col: &zero}, want: "--row must be >= 0"},
		{name: "negative col", cmd: SlidesInsertTextCmd{PresentationID: "pres1", ObjectID: "table_1", Text: "x", Row: &zero, Col: &neg}, want: "--col must be >= 0"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Run(ctx, flags)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestSlidesInsertText_EmptyTextWithoutReplace(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Fatalf("expected empty text error, got: %v", err)
	}
}

func TestSlidesInsertText_EmptyObjectID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "",
		Text:           "something",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty objectId") {
		t.Fatalf("expected empty objectId error, got: %v", err)
	}
}
