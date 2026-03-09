package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// newSedTestServer creates a mock Google Docs API server that handles
// document GET and batchUpdate requests.
func newSedTestServer(t *testing.T, doc *docs.Document) (*docs.Service, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
		if r.Method == http.MethodPost {
			// batchUpdate — return empty success with replies
			body, _ := io.ReadAll(r.Body)
			var req docs.BatchUpdateDocumentRequest
			_ = json.Unmarshal(body, &req)

			resp := &docs.BatchUpdateDocumentResponse{}
			for range req.Requests {
				resp.Replies = append(resp.Replies, &docs.Response{})
			}
			// Check for ReplaceAllText and return occurrences
			for i, rr := range req.Requests {
				if rr.ReplaceAllText != nil {
					resp.Replies[i] = &docs.Response{
						ReplaceAllText: &docs.ReplaceAllTextResponse{OccurrencesChanged: 1},
					}
				}
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	require.NoError(t, err)
	return docSvc, srv.Close
}

func sedTestUI() *ui.UI {
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	return u
}

// mockDocsService sets newDocsService to return the given service and restores on cleanup.
func mockDocsService(t *testing.T, svc *docs.Service) {
	t.Helper()
	orig := newDocsService
	newDocsService = func(context.Context, string) (*docs.Service, error) { return svc, nil }
	t.Cleanup(func() { newDocsService = orig })
}

func TestRunDeleteCommand(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{SectionBreak: &docs.SectionBreak{}, StartIndex: 0, EndIndex: 1},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "keep this\n"}},
				},
			}, StartIndex: 1, EndIndex: 11},
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "delete me\n"}},
				},
			}, StartIndex: 11, EndIndex: 22},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, err := parseDCommand("d/delete/")
	require.NoError(t, err)

	err = cmd.runDeleteCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunDeleteCommand_NoMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "some text\n"}},
				},
			}, StartIndex: 1, EndIndex: 11},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseDCommand("d/nonexistent/")
	err := cmd.runDeleteCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunInsertAroundMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target line\n"}},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()

	// Test append (after)
	expr, _ := parseAICommand("a/target/new text/", 'a')
	err := cmd.runAppendCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)

	// Test insert (before)
	expr, _ = parseAICommand("i/target/before text/", 'i')
	err = cmd.runInsertCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunInsertAroundMatch_NoMatch(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "text\n"}},
				},
			}, StartIndex: 1, EndIndex: 6},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseAICommand("a/nope/text/", 'a')
	err := cmd.runAppendCommand(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err) // no matches, but not an error
}

func TestRunTransliterate(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello\n"}},
				},
			}, StartIndex: 1, EndIndex: 7},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseYCommand("y/helo/HELO/")
	err := cmd.runTransliterate(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_DeleteCommand(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target\n"}},
				},
			}, StartIndex: 1, EndIndex: 8},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr, _ := parseDCommand("d/target/")
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_NativeReplace(t *testing.T) {
	doc := &docs.Document{DocumentId: "test-doc"}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "foo", replacement: "bar", global: true}
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunSingle_Manual(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello world\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()
	expr := sedExpr{pattern: "hello", replacement: "**goodbye**"}
	err := cmd.runSingle(context.Background(), u, "", "test-doc", expr)
	assert.NoError(t, err)
}

func TestRunBatch_Mixed(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "hello world\n"}, StartIndex: 1, EndIndex: 13},
				},
			}, StartIndex: 1, EndIndex: 13},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()

	exprs := []sedExpr{
		{pattern: "foo", replacement: "bar", global: true}, // native
		{pattern: "baz", replacement: "qux", global: true}, // native (batched)
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc", exprs)
	assert.NoError(t, err)
}

func TestRunBatch_WithDelete(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "target\n"}},
				},
			}, StartIndex: 1, EndIndex: 8},
		}},
	}
	svc, cleanup := newSedTestServer(t, doc)
	defer cleanup()
	mockDocsService(t, svc)

	cmd := &DocsSedCmd{}
	u := sedTestUI()

	exprs := []sedExpr{
		{command: 'd', pattern: "target"},
		{pattern: "foo", replacement: "bar", global: true},
	}
	err := cmd.runBatch(context.Background(), u, "", "test-doc", exprs)
	assert.NoError(t, err)
}
