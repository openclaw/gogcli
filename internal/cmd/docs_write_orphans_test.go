package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

func TestDocsWriteCheckOrphansValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "plain", args: []string{"doc1", "--text", "text", "--replace", "--check-orphans"}},
		{name: "markdown without replace", args: []string{"doc1", "--text", "text", "--markdown", "--check-orphans"}},
		{name: "append", args: []string{"doc1", "--text", "text", "--markdown", "--append", "--check-orphans"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runKong(t, &DocsWriteCmd{}, tt.args, newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com", DryRun: true})
			if err == nil || !strings.Contains(err.Error(), "--check-orphans requires --replace --markdown") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestDocsWriteCheckOrphansDryRunSkipsServices(t *testing.T) {
	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("dry-run must not create Drive service")
		return nil, errors.New("unexpected Drive service creation")
	}
	docsFactory := func(context.Context, string) (*docs.Service, error) {
		t.Fatal("dry-run must not create Docs service")
		return nil, errors.New("unexpected Docs service creation")
	}

	var stdout, stderr bytes.Buffer
	ctx := withDocsTestServiceFactory(
		withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory),
		docsFactory,
	)
	runErr := runKong(t, &DocsWriteCmd{},
		[]string{"doc1", "--text", "replacement", "--markdown", "--replace", "--check-orphans"},
		ctx,
		&RootFlags{Account: "a@b.com", DryRun: true},
	)
	var exitErr *ExitError
	if !errors.As(runErr, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("err = %#v, want dry-run exit 0", runErr)
	}
	var got struct {
		Request struct {
			CheckOrphans bool `json:"check_orphans"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if !got.Request.CheckOrphans {
		t.Fatalf("dry-run request = %#v, want check_orphans", got.Request)
	}
}

func TestFindDocsWriteMarkdownOrphansFiltersAndMatchesRenderedText(t *testing.T) {
	doc := docsFindRangeDoc(
		docsFindRangeParagraph(1, "Tom & Jerry\n"),
		docsFindRangeParagraph(13, "kept phrase\n"),
	)
	comments := []*drive.Comment{
		{
			Id:      "c1",
			Content: "entity quote",
			Author:  &drive.User{DisplayName: "Alice", EmailAddress: "alice@example.com"},
			QuotedFileContent: &drive.CommentQuotedFileContent{
				Value: "Tom &amp; Jerry",
			},
		},
		{
			Id:       "c2",
			Resolved: true,
			QuotedFileContent: &drive.CommentQuotedFileContent{
				Value: "Tom &amp; Jerry",
			},
		},
		{Id: "c3", Content: "unanchored"},
		{
			Id: "c4",
			QuotedFileContent: &drive.CommentQuotedFileContent{
				Value: "already orphaned",
			},
		},
		{
			Id: "c5",
			QuotedFileContent: &drive.CommentQuotedFileContent{
				Value: "kept phrase",
			},
		},
	}
	driveSvc, docsSvc := newDocsWriteOrphanServices(t, doc, comments)

	orphans, tabID, err := findDocsWriteMarkdownOrphans(
		context.Background(),
		driveSvc,
		docsSvc,
		"doc1",
		prepareMarkdown("# Replacement\n\nkept phrase"),
		"",
		true,
	)
	if err != nil {
		t.Fatalf("find orphans: %v", err)
	}
	if tabID != "" {
		t.Fatalf("tabID = %q, want whole document", tabID)
	}
	if len(orphans) != 1 || orphans[0].CommentID != "c1" || orphans[0].AuthorEmail != "alice@example.com" {
		t.Fatalf("orphans = %#v, want c1", orphans)
	}
}

func TestFindDocsWriteMarkdownOrphansScopesToTargetTab(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc1",
		Tabs: []*docs.Tab{
			{
				TabProperties: &docs.TabProperties{TabId: "t.first", Title: "First"},
				DocumentTab:   &docs.DocumentTab{Body: docsFindRangeDoc(docsFindRangeParagraph(1, "outside quote\n")).Body},
			},
			{
				TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
				DocumentTab:   &docs.DocumentTab{Body: docsFindRangeDoc(docsFindRangeParagraph(1, "inside quote\n")).Body},
			},
		},
	}
	comments := []*drive.Comment{
		{Id: "outside", QuotedFileContent: &drive.CommentQuotedFileContent{Value: "outside quote"}},
		{Id: "inside", QuotedFileContent: &drive.CommentQuotedFileContent{Value: "inside quote"}},
	}
	driveSvc, docsSvc := newDocsWriteOrphanServices(t, doc, comments)

	orphans, tabID, err := findDocsWriteMarkdownOrphans(
		context.Background(),
		driveSvc,
		docsSvc,
		"doc1",
		prepareMarkdown("replacement"),
		"Second",
		false,
	)
	if err != nil {
		t.Fatalf("find orphans: %v", err)
	}
	if tabID != "t.second" {
		t.Fatalf("tabID = %q, want t.second", tabID)
	}
	if len(orphans) != 1 || orphans[0].CommentID != "inside" {
		t.Fatalf("orphans = %#v, want inside only", orphans)
	}
}

func TestDocsWriteMarkdownDocumentIncludesTableCellText(t *testing.T) {
	doc := docsWriteMarkdownDocument(prepareMarkdown("| Name | Note |\n| --- | --- |\n| Ada | **cell phrase** |"))
	locator := DocsCommentsLocateCmd{NormalizeWhitespace: true}
	matches := locator.findQuoteMatchesInDoc(doc, "cell phrase", "")
	if len(matches) != 1 || !matches[0].InTable {
		t.Fatalf("matches = %#v, want one table match", matches)
	}
}

func TestDocsWriteMarkdownDocumentStripsHeadingAnchors(t *testing.T) {
	doc := docsWriteMarkdownDocument(prepareMarkdown("## Files {#attachments}"))
	locator := DocsCommentsLocateCmd{NormalizeWhitespace: true}
	if matches := locator.findQuoteMatchesInDoc(doc, "{#attachments}", ""); len(matches) != 0 {
		t.Fatalf("matches = %#v, want explicit heading anchor stripped", matches)
	}
	if matches := locator.findQuoteMatchesInDoc(doc, "Files", ""); len(matches) != 1 {
		t.Fatalf("matches = %#v, want rendered heading text", matches)
	}
}

func TestWriteDocsWriteOrphanResultJSONAndPlain(t *testing.T) {
	orphans := []docsWriteOrphanComment{
		{
			CommentID: "c1",
			Author:    "Alice",
			Content:   strings.Repeat("comment ", 20),
			Quote:     strings.Repeat("quoted ", 20),
		},
	}

	var jsonOut, jsonStderr bytes.Buffer
	jsonErr := writeDocsWriteOrphanResult(newCmdRuntimeJSONOutputContext(t, &jsonOut, &jsonStderr), "doc1", "t.second", orphans)
	var exitErr *ExitError
	if !errors.As(jsonErr, &exitErr) || exitErr.Code != exitCodeOrphaned {
		t.Fatalf("json err = %#v, want exit %d", jsonErr, exitCodeOrphaned)
	}
	var result docsWriteOrphanResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut.String())
	}
	if result.DocumentID != "doc1" || result.TabID != "t.second" || len(result.WouldOrphan) != 1 {
		t.Fatalf("result = %#v", result)
	}

	var stdout, stderr bytes.Buffer
	plainErr := writeDocsWriteOrphanResult(newCmdOutputContext(t, &stdout, &stderr), "doc1", "", orphans)
	if !errors.As(plainErr, &exitErr) || exitErr.Code != exitCodeOrphaned {
		t.Fatalf("plain err = %#v, want exit %d", plainErr, exitCodeOrphaned)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"blocked: 1", "Alice", "c1"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func newDocsWriteOrphanServices(t *testing.T, doc *docs.Document, comments []*drive.Comment) (*drive.Service, *docs.Service) {
	t.Helper()
	driveSvc, _ := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&drive.CommentList{Comments: comments})
	}))
	docsSvc, _ := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/documents/doc1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	return driveSvc, docsSvc
}
