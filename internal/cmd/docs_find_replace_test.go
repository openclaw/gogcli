package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsFindReplace_PlainText(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docBodyWithText("This is Draft v1 of the document"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "Draft v1", "Final v2", "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --first: %v", err)
	}

	// Should be delete + insert (single occurrence replace), not ReplaceAllText.
	if len(got.Requests) != 2 {
		t.Fatalf("expected 2 requests (delete + insert), got %d", len(got.Requests))
	}
	if got.Requests[0].DeleteContentRange == nil {
		t.Fatal("first request should be DeleteContentRange")
	}
	if got.Requests[1].InsertText == nil {
		t.Fatal("second request should be InsertText")
	}
	if got.Requests[1].InsertText.Text != "Final v2" {
		t.Fatalf("expected insert text 'Final v2', got %q", got.Requests[1].InsertText.Text)
	}
}

func TestDocsFindReplace_PlainTextMatchCase(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			// "TODO" appears at index 1, "todo" at index 10 — case-sensitive should only match "TODO".
			_ = json.NewEncoder(w).Encode(docBodyWithText("TODO and todo"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "TODO", "DONE", "--match-case", "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --match-case --first: %v", err)
	}

	// Should delete "TODO" at [1,5] and insert "DONE" at 1.
	if len(got.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got.Requests))
	}
	delRange := got.Requests[0].DeleteContentRange.Range
	if delRange.StartIndex != 1 || delRange.EndIndex != 5 {
		t.Fatalf("expected delete [1,5], got [%d,%d]", delRange.StartIndex, delRange.EndIndex)
	}
}

func TestDocsFindReplace_ZeroOccurrences(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docBodyWithText("Hello world"))
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	// Zero occurrences is not an error — no batchUpdate should be sent.
	if err := runKong(t, cmd, []string{"doc1", "nonexistent", "whatever", "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --first zero occurrences: %v", err)
	}
}

func TestDocsFindReplace_ContentFile(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	// Write a temp file with replacement content.
	tmp := t.TempDir()
	contentPath := tmp + "/replace.txt"
	if err := os.WriteFile(contentPath, []byte("replacement from file"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docBodyWithText("Replace {{content}} here"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "{{content}}", "--content-file", contentPath, "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --content-file --first: %v", err)
	}

	if got.Requests[1].InsertText.Text != "replacement from file" {
		t.Fatalf("expected replacement from file, got %q", got.Requests[1].InsertText.Text)
	}
}

func TestDocsFindReplace_ContentFile_FirstPrintsResolvedReplacement(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	tmp := t.TempDir()
	contentPath := tmp + "/replace.txt"
	if err := os.WriteFile(contentPath, []byte("replacement from file"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			body := docBodyWithText("Replace {{content}} here")
			body["revisionId"] = "rev-1"
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	ctx, out := newDocsCmdOutputContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "{{content}}", "--content-file", contentPath, "--first"}, ctx, flags); err != nil {
		t.Fatalf("docs find-replace --content-file --first: %v", err)
	}

	if !strings.Contains(out.String(), "replace\treplacement from file") {
		t.Fatalf("expected plain output to include resolved replacement, got %q", out.String())
	}
}

func TestDocsFindReplace_EmptyDocID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{"", "find", "replace"}, newDocsCmdContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "empty docId") {
		t.Fatalf("expected empty docId error, got: %v", err)
	}
}

func TestDocsFindReplace_EmptySearchText(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{"doc1", "", "replace"}, newDocsCmdContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "find text cannot be empty") {
		t.Fatalf("expected find text error, got: %v", err)
	}
}

func TestDocsFindReplace_MarkdownMode(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchCalls []docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"startIndex": 0,
							"endIndex":   1,
							"sectionBreak": map[string]any{
								"sectionStyle": map[string]any{},
							},
						},
						map[string]any{
							"startIndex": 1,
							"endIndex":   30,
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"startIndex": 1,
										"endIndex":   30,
										"textRun": map[string]any{
											"content": "Hello {{placeholder}} world",
										},
									},
								},
							},
						},
					},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			batchCalls = append(batchCalls, req)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{"doc1", "{{placeholder}}", "**bold text**", "--format", "markdown", "--first"}, newDocsCmdContext(t), flags)
	if err != nil {
		t.Fatalf("docs find-replace --format markdown --first: %v", err)
	}

	if len(batchCalls) == 0 {
		t.Fatal("expected at least one batchUpdate call")
	}
	reqs := batchCalls[0].Requests
	// Should have: DeleteContentRange, InsertText, then formatting requests.
	if len(reqs) < 2 {
		t.Fatalf("expected at least 2 requests (delete + insert), got %d", len(reqs))
	}
	if reqs[0].DeleteContentRange == nil {
		t.Fatal("first request should be DeleteContentRange")
	}
	if reqs[1].InsertText == nil {
		t.Fatal("second request should be InsertText")
	}
}

func TestDocsFindReplace_MarkdownNoMatch(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"startIndex": 0,
							"endIndex":   1,
							"sectionBreak": map[string]any{
								"sectionStyle": map[string]any{},
							},
						},
						map[string]any{
							"startIndex": 1,
							"endIndex":   12,
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"startIndex": 1,
										"endIndex":   12,
										"textRun": map[string]any{
											"content": "Hello world",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	// No match should succeed with 0 replacements, not error.
	if err := runKong(t, cmd, []string{"doc1", "nonexistent", "**bold**", "--format", "markdown", "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace markdown --first no match: %v", err)
	}
}

func TestDocsFindReplace_MarkdownReplaceAll_DoesNotLoopOnSelfMatch(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchCalls []docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			body := docBodyWithText("foo and foo")
			body["revisionId"] = "rev-1"
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			batchCalls = append(batchCalls, req)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "foo", "**foo**", "--format", "markdown"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --format markdown: %v", err)
	}

	if len(batchCalls) != 2 {
		t.Fatalf("expected 2 markdown replacements from the original snapshot, got %d", len(batchCalls))
	}
}

func TestDocsFindReplace_MarkdownWithImage(t *testing.T) {
	origDocs := newDocsService
	origToken := imgPlaceholderToken
	t.Cleanup(func() { newDocsService = origDocs; imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	var batchCalls []docs.BatchUpdateDocumentRequest
	callCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			callCount++
			if callCount == 1 {
				// First GET: return doc with the placeholder text.
				_ = json.NewEncoder(w).Encode(docBodyWithText("Replace {{img}} here"))
			} else {
				// Second GET (read-back after insert): doc now has <<IMG_test_0>> placeholder.
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": "doc1",
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"startIndex":   0,
								"endIndex":     1,
								"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
							},
							map[string]any{
								"startIndex": 1,
								"endIndex":   30,
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"startIndex": 1,
											"endIndex":   30,
											"textRun":    map[string]any{"content": "Replace <<IMG_test_0>> here\n"},
										},
									},
								},
							},
						},
					},
				})
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			batchCalls = append(batchCalls, req)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "{{img}}",
		"![Screenshot](https://example.com/image.png)",
		"--format", "markdown", "--first",
	}, newDocsCmdContext(t), flags)
	if err != nil {
		t.Fatalf("docs find-replace markdown --first with image: %v", err)
	}

	// Should have three batchUpdate calls:
	// 1. Delete placeholder + insert text (with <<IMG_test_0>>)
	// 2. Delete <<IMG_test_0>> + InsertInlineImage
	// 3. Cleanup ReplaceAllText (belt and suspenders)
	if len(batchCalls) < 2 {
		t.Fatalf("expected at least 2 batchUpdate calls, got %d", len(batchCalls))
	}

	// Second call should contain InsertInlineImage.
	imgCall := batchCalls[1]
	foundImage := false
	for _, req := range imgCall.Requests {
		if req.InsertInlineImage != nil {
			foundImage = true
			if req.InsertInlineImage.Uri != "https://example.com/image.png" {
				t.Fatalf("expected image URL https://example.com/image.png, got %q", req.InsertInlineImage.Uri)
			}
		}
	}
	if !foundImage {
		t.Fatal("expected InsertInlineImage request in second batchUpdate call")
	}
}

func TestDocsFindReplace_MarkdownImageFailure_CleansUpPlaceholders(t *testing.T) {
	origDocs := newDocsService
	origToken := imgPlaceholderToken
	t.Cleanup(func() { newDocsService = origDocs; imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	var batchCalls []docs.BatchUpdateDocumentRequest
	getCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCount++
			if getCount == 1 {
				// First GET: doc with placeholder to replace.
				_ = json.NewEncoder(w).Encode(docBodyWithText("Replace {{img}} here"))
			} else {
				// Second GET (read-back): text now has <<IMG_test_0>>.
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": "doc1",
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"startIndex": 0, "endIndex": 1,
								"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
							},
							map[string]any{
								"startIndex": 1, "endIndex": 25,
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"startIndex": 1, "endIndex": 25,
											"textRun": map[string]any{"content": "Replace <<IMG_test_0>> here\n"},
										},
									},
								},
							},
						},
					},
				})
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			batchCalls = append(batchCalls, req)

			// Fail the second batchUpdate (the image insertion) with a 500.
			if len(batchCalls) == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    500,
						"message": "simulated backend error",
						"status":  "INTERNAL",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "{{img}}",
		"![Screenshot](https://example.com/image.png)",
		"--format", "markdown", "--first",
	}, newDocsCmdContext(t), flags)

	// The command should return an error (image insertion failed).
	if err == nil {
		t.Fatal("expected error from failed image insertion")
	}

	// But there must be a third batchUpdate call: the cleanup that removes <<IMG_test_0>>.
	if len(batchCalls) < 3 {
		t.Fatalf("expected 3 batchUpdate calls (text insert, image fail, cleanup), got %d", len(batchCalls))
	}

	// Third call should be a ReplaceAllText that strips <<IMG_test_0>>.
	cleanupCall := batchCalls[2]
	foundCleanup := false
	for _, req := range cleanupCall.Requests {
		if req.ReplaceAllText != nil &&
			req.ReplaceAllText.ContainsText != nil &&
			req.ReplaceAllText.ContainsText.Text == "<<IMG_test_0>>" &&
			req.ReplaceAllText.ReplaceText == "" {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Fatal("expected cleanup ReplaceAllText request to remove <<IMG_test_0>> placeholder")
	}
}

func TestDocsFindReplace_MarkdownImageSuccess_StillCleansUp(t *testing.T) {
	origDocs := newDocsService
	origToken := imgPlaceholderToken
	t.Cleanup(func() { newDocsService = origDocs; imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	var batchCalls []docs.BatchUpdateDocumentRequest
	getCount := 0
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCount++
			if getCount == 1 {
				_ = json.NewEncoder(w).Encode(docBodyWithText("Replace {{img}} here"))
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": "doc1",
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"startIndex": 0, "endIndex": 1,
								"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
							},
							map[string]any{
								"startIndex": 1, "endIndex": 20,
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"startIndex": 1, "endIndex": 20,
											"textRun": map[string]any{"content": "Replace <<IMG_test_0>> here\n"},
										},
									},
								},
							},
						},
					},
				})
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			batchCalls = append(batchCalls, req)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "{{img}}",
		"![Screenshot](https://example.com/image.png)",
		"--format", "markdown", "--first",
	}, newDocsCmdContext(t), flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 calls: text insert, image insert, cleanup (belt and suspenders).
	if len(batchCalls) < 3 {
		t.Fatalf("expected 3 batchUpdate calls (text, image, cleanup), got %d", len(batchCalls))
	}

	cleanupCall := batchCalls[2]
	foundCleanup := false
	for _, req := range cleanupCall.Requests {
		if req.ReplaceAllText != nil &&
			req.ReplaceAllText.ContainsText != nil &&
			req.ReplaceAllText.ContainsText.Text == "<<IMG_test_0>>" &&
			req.ReplaceAllText.ReplaceText == "" {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Fatal("expected cleanup ReplaceAllText even on successful image insertion")
	}
}

func TestDocsFindReplace_PinsRevisionId(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	body := docBodyWithText("Replace me here")
	body["revisionId"] = "rev-abc-123"

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "me", "you", "--first"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --first: %v", err)
	}

	if got.WriteControl == nil || got.WriteControl.RequiredRevisionId != "rev-abc-123" {
		t.Fatalf("expected WriteControl.RequiredRevisionId=rev-abc-123, got %+v", got.WriteControl)
	}
}

func TestFindTextInDoc(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 0,
					EndIndex:   1,
				},
				{
					StartIndex: 1,
					EndIndex:   25,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								EndIndex:   25,
								TextRun: &docs.TextRun{
									Content: "Hello world, hello again",
								},
							},
						},
					},
				},
			},
		},
	}

	// Case-insensitive: should find both "Hello" and "hello".
	start, end, total := findTextInDoc(doc, "hello", false)
	if total != 2 {
		t.Fatalf("expected 2 matches, got %d", total)
	}
	if start != 1 || end != 6 {
		t.Fatalf("expected first match at [1,6], got [%d,%d]", start, end)
	}

	// Case-sensitive: should find only "hello" (lowercase).
	start, end, total = findTextInDoc(doc, "hello", true)
	if total != 1 {
		t.Fatalf("expected 1 case-sensitive match, got %d", total)
	}
	if start != 14 || end != 19 {
		t.Fatalf("expected match at [14,19], got [%d,%d]", start, end)
	}

	// No match.
	_, _, total = findTextInDoc(doc, "xyz", false)
	if total != 0 {
		t.Fatalf("expected 0 matches, got %d", total)
	}
}

func TestFindTextInDoc_TableCell(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								TableCells: []*docs.TableCell{
									{
										Content: []*docs.StructuralElement{
											{
												Paragraph: &docs.Paragraph{
													Elements: []*docs.ParagraphElement{
														{
															StartIndex: 40,
															TextRun:    &docs.TextRun{Content: "before needle after"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	start, end, total := findTextInDoc(doc, "needle", true)
	if total != 1 {
		t.Fatalf("expected 1 match, got %d", total)
	}
	if start != 47 || end != 53 {
		t.Fatalf("expected match at [47,53], got [%d,%d]", start, end)
	}
}
