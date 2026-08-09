package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/outfmt"
)

type docsCommentsListPage struct {
	comments      []map[string]any
	nextPageToken string
}

type docsCommentsListLocatedJSON struct {
	DocID    string              `json:"docId"`
	Tab      *docsCommentListTab `json:"tab"`
	Comments []struct {
		ID                string                          `json:"id"`
		Content           string                          `json:"content"`
		QuotedFileContent *drive.CommentQuotedFileContent `json:"quotedFileContent"`
		Location          *docsCommentLocation            `json:"location"`
	} `json:"comments"`
	NextPageToken string `json:"nextPageToken"`
}

func docsCommentFixture(id, quote string) map[string]any {
	comment := map[string]any{"id": id, "content": "note " + id, "resolved": false}
	if quote != "" {
		comment["quotedFileContent"] = map[string]any{"value": quote}
	}
	return comment
}

// newDocsCommentsListService serves comments.list pages for doc1. Page N is
// requested with pageToken "pN"; each page declares its own nextPageToken.
func newDocsCommentsListService(t *testing.T, pages ...docsCommentsListPage) *drive.Service {
	t.Helper()
	byToken := make(map[string]docsCommentsListPage, len(pages))
	for i, page := range pages {
		token := ""
		if i > 0 {
			token = fmt.Sprintf("p%d", i)
		}
		byToken[token] = page
	}
	return newDriveCommentsTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments" {
			http.NotFound(w, r)
			return
		}
		page, ok := byToken[r.URL.Query().Get("pageToken")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"comments":      page.comments,
			"nextPageToken": page.nextPageToken,
		})
	})
}

// docsCommentsListTabsDoc is a two-tab document where "shared quote" appears
// in both tabs.
func docsCommentsListTabsDoc() *docs.Document {
	return &docs.Document{
		DocumentId: "doc1",
		Tabs: []*docs.Tab{
			{
				TabProperties: &docs.TabProperties{TabId: "t.first", Title: "First"},
				DocumentTab: &docs.DocumentTab{Body: docsFindRangeDoc(
					docsFindRangeParagraph(1, "first tab quote\n"),
					docsFindRangeParagraph(17, "shared quote\n"),
				).Body},
			},
			{
				TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
				DocumentTab: &docs.DocumentTab{Body: docsFindRangeDoc(
					docsFindRangeParagraph(1, "second tab quote\n"),
					docsFindRangeParagraph(18, "shared quote\n"),
				).Body},
			},
		},
	}
}

func newCountingDocsDocumentTestService(t *testing.T, document any, fetches *int, includeTabs *string) *docs.Service {
	t.Helper()
	svc, _ := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		if fetches != nil {
			*fetches++
		}
		if includeTabs != nil {
			*includeTabs = r.URL.Query().Get("includeTabsContent")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	})
	return svc
}

func runDocsCommentsListJSON(t *testing.T, driveSvc *drive.Service, docsSvc *docs.Service, args ...string) executeTestResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	ctx := docsCommentsListTestContext(t, newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveSvc, docsSvc)
	err := runKong(t, &DocsCommentsListCmd{}, args, ctx, &RootFlags{Account: "a@b.com"})
	return executeTestResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func docsCommentsListTestContext(t *testing.T, ctx context.Context, driveSvc *drive.Service, docsSvc *docs.Service) context.Context {
	t.Helper()
	ctx = withDriveTestService(ctx, driveSvc)
	if docsSvc == nil {
		return withDocsTestServiceFactory(ctx, func(context.Context, string) (*docs.Service, error) {
			t.Fatal("comments list must not create a Docs service without --locate/--tab")
			return nil, errors.New("unexpected Docs service creation")
		})
	}
	return withDocsTestService(ctx, docsSvc)
}

func parseDocsCommentsListLocated(t *testing.T, stdout string) docsCommentsListLocatedJSON {
	t.Helper()
	var parsed docsCommentsListLocatedJSON
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	return parsed
}

func TestDocsCommentsListDefaultSkipsDocsFetch(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{
		comments: []map[string]any{docsCommentFixture("c1", "first tab quote")},
	})

	result := runDocsCommentsListJSON(t, driveSvc, nil, "doc1")
	if result.err != nil {
		t.Fatalf("list: %v", result.err)
	}
	if strings.Contains(result.stdout, "location") {
		t.Fatalf("default output must not carry location data: %s", result.stdout)
	}

	var parsed struct {
		Comments []*drive.Comment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, result.stdout)
	}
	if len(parsed.Comments) != 1 || parsed.Comments[0].Id != "c1" {
		t.Fatalf("comments = %#v", parsed.Comments)
	}
}

func TestDocsCommentsListLocateSharesOneDocumentFetch(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
		docsCommentFixture("c2", "second tab quote"),
		docsCommentFixture("c3", "deleted quote"),
		docsCommentFixture("c4", ""),
	}})

	fetches := 0
	includeTabs := ""
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), &fetches, &includeTabs)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--locate")
	if result.err != nil {
		t.Fatalf("list --locate: %v", result.err)
	}
	if fetches != 1 {
		t.Fatalf("documents.get calls = %d, want 1", fetches)
	}
	if includeTabs != "true" {
		t.Fatalf("includeTabsContent = %q, want true", includeTabs)
	}

	parsed := parseDocsCommentsListLocated(t, result.stdout)
	if len(parsed.Comments) != 4 {
		t.Fatalf("comments = %d, want 4 (orphans reported, not dropped)", len(parsed.Comments))
	}
	if parsed.Tab != nil {
		t.Fatalf("tab = %#v, want none without --tab", parsed.Tab)
	}

	byID := map[string]*docsCommentLocation{}
	for _, comment := range parsed.Comments {
		if comment.Location == nil {
			t.Fatalf("comment %s has no location", comment.ID)
		}
		byID[comment.ID] = comment.Location
	}
	if got := byID["c1"]; got.Orphaned || len(got.Matches) != 1 || got.Matches[0].TabID != "t.first" {
		t.Fatalf("c1 location = %#v", got)
	}
	if got := byID["c2"]; got.Orphaned || len(got.Matches) != 1 || got.Matches[0].TabID != "t.second" {
		t.Fatalf("c2 location = %#v", got)
	}
	if got := byID["c3"]; !got.Orphaned || len(got.Matches) != 0 {
		t.Fatalf("c3 location = %#v, want orphaned with no matches", got)
	}
	if got := byID["c4"]; !got.Orphaned || len(got.Matches) != 0 {
		t.Fatalf("c4 (unquoted) location = %#v, want orphaned with no matches", got)
	}
}

func TestDocsCommentsListTabFiltersAndImpliesLocate(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
		docsCommentFixture("c2", "second tab quote"),
		docsCommentFixture("c3", "deleted quote"),
		docsCommentFixture("c5", "shared quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--tab", "Second")
	if result.err != nil {
		t.Fatalf("list --tab: %v", result.err)
	}

	parsed := parseDocsCommentsListLocated(t, result.stdout)
	if parsed.Tab == nil || parsed.Tab.ID != "t.second" || parsed.Tab.Title != "Second" {
		t.Fatalf("tab = %#v, want the resolved second tab", parsed.Tab)
	}
	if len(parsed.Comments) != 2 {
		t.Fatalf("comments = %d, want c2 and c5 only", len(parsed.Comments))
	}
	for _, comment := range parsed.Comments {
		if comment.ID != "c2" && comment.ID != "c5" {
			t.Fatalf("unexpected comment %s (orphans and other tabs must be filtered out)", comment.ID)
		}
		if comment.Location == nil {
			t.Fatalf("--tab must imply --locate, comment %s has no location", comment.ID)
		}
	}
}

func TestDocsCommentsListTabKeepsCrossTabMatches(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c5", "shared quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--tab", "t.second")
	if result.err != nil {
		t.Fatalf("list --tab: %v", result.err)
	}

	parsed := parseDocsCommentsListLocated(t, result.stdout)
	if len(parsed.Comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(parsed.Comments))
	}
	matches := parsed.Comments[0].Location.Matches
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want both tabs so callers can spot ambiguity", matches)
	}
	if matches[0].TabID != "t.first" || matches[1].TabID != "t.second" {
		t.Fatalf("matches = %#v, want one per tab", matches)
	}
}

func TestDocsCommentsListTabScansPagesForMatches(t *testing.T) {
	driveSvc := newDocsCommentsListService(t,
		docsCommentsListPage{
			comments:      []map[string]any{docsCommentFixture("c1", "first tab quote")},
			nextPageToken: "p1",
		},
		docsCommentsListPage{
			comments: []map[string]any{docsCommentFixture("c2", "second tab quote")},
		},
	)

	fetches := 0
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), &fetches, nil)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--tab", "Second")
	if result.err != nil {
		t.Fatalf("list --tab: %v", result.err)
	}
	if fetches != 1 {
		t.Fatalf("documents.get calls = %d, want 1 across the page scan", fetches)
	}

	parsed := parseDocsCommentsListLocated(t, result.stdout)
	if len(parsed.Comments) != 1 || parsed.Comments[0].ID != "c2" {
		t.Fatalf("comments = %#v, want the match from the second page", parsed.Comments)
	}
}

func TestDocsCommentsListTabFailEmptyExits(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
		docsCommentFixture("c3", "deleted quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--tab", "Second", "--fail-empty")
	var exitErr *ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code != emptyResultsExitCode {
		t.Fatalf("err = %#v, want empty-results exit %d", result.err, emptyResultsExitCode)
	}

	parsed := parseDocsCommentsListLocated(t, result.stdout)
	if len(parsed.Comments) != 0 {
		t.Fatalf("comments = %#v, want none", parsed.Comments)
	}
}

func TestDocsCommentsListTabUnknown(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	result := runDocsCommentsListJSON(t, driveSvc, docsSvc, "doc1", "--tab", "Missing")
	if result.err == nil || !strings.Contains(result.err.Error(), "tab not found") {
		t.Fatalf("err = %v, want a tab-not-found error", result.err)
	}
	var exitErr *ExitError
	if errors.As(result.err, &exitErr) {
		t.Fatalf("err = %#v, want a plain error, not an exit code", result.err)
	}
}

func TestDocsCommentsListTabRejectsExplicitEmptyValues(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{})

	for _, args := range [][]string{
		{"doc1", "--tab", " "},
		{"doc1", "--tab-id", " "},
	} {
		result := runDocsCommentsListJSON(t, driveSvc, nil, args...)
		if result.err == nil || !strings.Contains(result.err.Error(), "--tab requires a non-empty tab title or ID") {
			t.Fatalf("args %v: err = %v, want explicit-empty tab rejection", args, result.err)
		}
	}
}

func TestDocsCommentsListLocatePlainTable(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
		docsCommentFixture("c3", "deleted quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	var stdout, stderr bytes.Buffer
	ctx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &stdout, &stderr), outfmt.Mode{Plain: true})
	ctx = docsCommentsListTestContext(t, ctx, driveSvc, docsSvc)
	if err := runKong(t, &DocsCommentsListCmd{}, []string{"doc1", "--locate"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list --locate --plain: %v", err)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %#v, want header plus two comments", lines)
	}
	if !strings.HasSuffix(lines[0], "\tTAB") {
		t.Fatalf("header = %q, want a trailing TAB column", lines[0])
	}
	if !strings.HasSuffix(lines[1], "\tFirst") {
		t.Fatalf("row = %q, want the resolved tab title", lines[1])
	}
	if !strings.HasSuffix(lines[2], "\t(orphaned)") {
		t.Fatalf("row = %q, want the orphan marker", lines[2])
	}
}

func TestDocsCommentsListLocateDocumentWithoutTabs(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "untabbed quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsFindRangeDoc(docsFindRangeParagraph(1, "untabbed quote\n")), nil, nil)

	var stdout, stderr bytes.Buffer
	ctx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &stdout, &stderr), outfmt.Mode{Plain: true})
	ctx = docsCommentsListTestContext(t, ctx, driveSvc, docsSvc)
	if err := runKong(t, &DocsCommentsListCmd{}, []string{"doc1", "--locate"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list --locate: %v", err)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want header plus one comment", lines)
	}
	// A document without tabs resolves to an empty tab ID, so the column has
	// nothing to name - but the comment is located, not orphaned.
	if !strings.HasSuffix(lines[1], "\t-") {
		t.Fatalf("row = %q, want the no-tab placeholder", lines[1])
	}
}

func TestDocsCommentsListTabEmptyMessageNamesTab(t *testing.T) {
	driveSvc := newDocsCommentsListService(t, docsCommentsListPage{comments: []map[string]any{
		docsCommentFixture("c1", "first tab quote"),
	}})
	docsSvc := newCountingDocsDocumentTestService(t, docsCommentsListTabsDoc(), nil, nil)

	var stdout, stderr bytes.Buffer
	ctx := docsCommentsListTestContext(t, newCmdRuntimeOutputContext(t, &stdout, &stderr), driveSvc, docsSvc)
	if err := runKong(t, &DocsCommentsListCmd{}, []string{"doc1", "--tab", "t.second"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list --tab: %v", err)
	}

	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	// The message names the tab by title even when --tab was given as an ID.
	if got := stderr.String(); !strings.Contains(got, `No comments located in tab "Second"`) {
		t.Fatalf("stderr = %q, want the resolved tab title", got)
	}
}

func TestDriveCommentWithLocationMarshalJSON(t *testing.T) {
	// drive.Comment declares MarshalJSON on a value receiver; without the
	// override on driveCommentWithLocation the promoted method would silently
	// drop the location key.
	item := &driveCommentWithLocation{
		Comment: &drive.Comment{
			Id:                "c1",
			Content:           "note",
			QuotedFileContent: &drive.CommentQuotedFileContent{Value: "quote"},
		},
		Location: &docsCommentLocation{Matches: []docsedit.TextRange{{StartIndex: 1, EndIndex: 6, TabID: "t.first"}}},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed struct {
		ID                string                          `json:"id"`
		Content           string                          `json:"content"`
		QuotedFileContent *drive.CommentQuotedFileContent `json:"quotedFileContent"`
		Location          *docsCommentLocation            `json:"location"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if parsed.ID != "c1" || parsed.Content != "note" || parsed.QuotedFileContent == nil {
		t.Fatalf("comment fields lost: %s", data)
	}
	if parsed.Location == nil || len(parsed.Location.Matches) != 1 || parsed.Location.Matches[0].TabID != "t.first" {
		t.Fatalf("location lost: %s", data)
	}
}
