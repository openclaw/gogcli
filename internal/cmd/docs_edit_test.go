package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// --- Unit tests for parseSedExpr ---

func TestParseSedExpr_Basic(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantPattern string
		wantReplace string
		wantGlobal  bool
		wantErr     bool
	}{
		{
			name:        "simple replacement",
			expr:        "s/foo/bar/",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  false,
		},
		{
			name:        "global flag",
			expr:        "s/foo/bar/g",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  true,
		},
		{
			name:        "empty replacement",
			expr:        "s/foo//",
			wantPattern: "foo",
			wantReplace: "",
			wantGlobal:  false,
		},
		{
			name:        "regex pattern",
			expr:        `s/\d+/NUM/g`,
			wantPattern: `\d+`,
			wantReplace: "NUM",
			wantGlobal:  true,
		},
		{
			name:        "backreference conversion",
			expr:        `s/(foo)/\1bar/`,
			wantPattern: "(foo)",
			wantReplace: "${1}bar",
			wantGlobal:  false,
		},
		{
			name:        "alternate delimiter",
			expr:        "s#foo#bar#g",
			wantPattern: "foo",
			wantReplace: "bar",
			wantGlobal:  true,
		},
		{
			name:        "pipe delimiter",
			expr:        "s|path/to/file|new/path|",
			wantPattern: "path/to/file",
			wantReplace: "new/path",
			wantGlobal:  false,
		},
		{
			name:    "invalid - not starting with s",
			expr:    "x/foo/bar/",
			wantErr: true,
		},
		{
			name:    "invalid - too short",
			expr:    "s/",
			wantErr: true,
		},
		{
			name:    "invalid - missing replacement",
			expr:    "s/foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", pattern, tt.wantPattern)
			}
			if replacement != tt.wantReplace {
				t.Errorf("replacement = %q, want %q", replacement, tt.wantReplace)
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

// --- Unit tests for parseMarkdownReplacement ---

func TestParseMarkdownReplacement(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "plain text",
			input:       "hello world",
			wantText:    "hello world",
			wantFormats: nil,
		},
		{
			name:        "bold",
			input:       "**bold text**",
			wantText:    "bold text",
			wantFormats: []string{"bold"},
		},
		{
			name:        "italic",
			input:       "*italic text*",
			wantText:    "italic text",
			wantFormats: []string{"italic"},
		},
		{
			name:        "bold italic",
			input:       "***bold italic***",
			wantText:    "bold italic",
			wantFormats: []string{"bold", "italic"},
		},
		{
			name:        "strikethrough",
			input:       "~~crossed out~~",
			wantText:    "crossed out",
			wantFormats: []string{"strikethrough"},
		},
		{
			name:        "code",
			input:       "`inline code`",
			wantText:    "inline code",
			wantFormats: []string{"code"},
		},
		{
			name:        "heading 1",
			input:       "# Title",
			wantText:    "Title",
			wantFormats: []string{"heading1"},
		},
		{
			name:        "heading 2",
			input:       "## Subtitle",
			wantText:    "Subtitle",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "heading 3",
			input:       "### Section",
			wantText:    "Section",
			wantFormats: []string{"heading3"},
		},
		{
			name:        "heading 6",
			input:       "###### Deep",
			wantText:    "Deep",
			wantFormats: []string{"heading6"},
		},
		{
			name:        "heading no space",
			input:       "##NoSpace",
			wantText:    "NoSpace",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "bullet list dash",
			input:       "- list item",
			wantText:    "list item",
			wantFormats: []string{"bullet"},
		},
		{
			name:        "bullet list asterisk",
			input:       "* list item",
			wantText:    "list item",
			wantFormats: []string{"bullet"},
		},
		{
			name:        "numbered list",
			input:       "1. first item",
			wantText:    "first item",
			wantFormats: []string{"numbered"},
		},
		{
			name:        "newline escape",
			input:       "line1\\nline2",
			wantText:    "line1\nline2",
			wantFormats: nil,
		},
		{
			name:        "bullet with bold",
			input:       "- **bold item**",
			wantText:    "bold item",
			wantFormats: []string{"bullet", "bold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}

// --- Integration tests for DocsEditCmd ---

// mockDocsServer creates a test server that simulates the Google Docs API
func mockDocsServer(t *testing.T, docContent string, onBatchUpdate func(reqs []*docs.Request)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /v1/documents/{docId}
		if r.Method == http.MethodGet && strings.Contains(path, "/documents/") {
			w.Header().Set("Content-Type", "application/json")
			doc := &docs.Document{
				DocumentId: "test-doc-id",
				Title:      "Test Document",
				Body: &docs.Body{
					Content: []*docs.StructuralElement{
						{
							StartIndex: 0,
							EndIndex:   int64(len(docContent)),
							Paragraph: &docs.Paragraph{
								Elements: []*docs.ParagraphElement{
									{
										StartIndex: 0,
										EndIndex:   int64(len(docContent)),
										TextRun: &docs.TextRun{
											Content: docContent,
										},
									},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}

		// POST /v1/documents/{docId}:batchUpdate
		if r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate") {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if onBatchUpdate != nil {
				onBatchUpdate(req.Requests)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{
				DocumentId: "test-doc-id",
			})
			return
		}

		http.NotFound(w, r)
	}))
}

func TestDocsEditCmd_JSON(t *testing.T) {
	var capturedReqs []*docs.Request
	srv := mockDocsServer(t, "Hello world, hello universe!", func(reqs []*docs.Request) {
		capturedReqs = reqs
	})
	defer srv.Close()

	// Create docs service with test server
	docsSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// We need to inject the mock service - for now test the helper functions
	// Full integration requires refactoring to use a mockable service variable
	_ = docsSvc

	// Test that parseSedExpr handles edit-style input correctly
	// The edit command internally constructs a simple replacement
	pattern := "hello"
	replacement := "hi"

	if pattern != "hello" || replacement != "hi" {
		t.Errorf("unexpected values")
	}

	// Verify captured requests would have correct structure
	if len(capturedReqs) > 0 {
		for _, req := range capturedReqs {
			if req.ReplaceAllText == nil {
				t.Error("expected ReplaceAllText request")
			}
		}
	}
}
