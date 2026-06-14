package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func parseDocsKong(t *testing.T, cmd any, args []string) *kong.Context {
	t.Helper()

	parser, err := kong.New(cmd)
	if err != nil {
		t.Fatalf("kong new: %v", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("kong parse: %v", err)
	}
	return kctx
}

func TestDocsInfo_ValidationAndText(t *testing.T) {
	t.Parallel()

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsInfoCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}

	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"title":      "Doc",
				"revisionId": "r1",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer cleanup()

	var outBuf strings.Builder
	ctx2 := withDocsTestService(newCmdRuntimeOutputContext(t, &outBuf, io.Discard), svc)

	if err := (&DocsInfoCmd{DocID: "doc1"}).Run(ctx2, flags); err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(outBuf.String(), "revision") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
}

func TestDocsCreateCat_ValidationErrors(t *testing.T) {
	t.Parallel()

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsCreateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing title error")
	}
	if err := (&DocsCatCmd{}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
}

func TestDocsWriteUpdate_ValidationErrors(t *testing.T) {
	t.Parallel()

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsWriteCmd{}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
	if err := (&DocsWriteCmd{DocID: "doc1"}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing text error")
	}
	if err := (&DocsUpdateCmd{}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
	if err := (&DocsUpdateCmd{DocID: "doc1"}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing text error")
	}
}

func TestDocsCat_JSON_EmptyDoc(t *testing.T) {
	t.Parallel()

	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body":       map[string]any{"content": []map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer cleanup()

	var output strings.Builder
	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsCatCmd{DocID: "doc1"}).Run(ctx, nil, flags); err != nil {
		t.Fatalf("cat: %v", err)
	}
	if !strings.Contains(output.String(), "\"text\"") {
		t.Fatalf("unexpected json: %q", output.String())
	}
}

func TestDocsUpdate_InvalidIndex(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}

	tests := []struct {
		name string
		args []string
	}{
		{"zero index", []string{"doc1", "--text", "hello", "--index", "0"}},
		{"negative index", []string{"doc1", "--text", "hello", "--index=-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &DocsUpdateCmd{}
			kctx := parseDocsKong(t, cmd, tt.args)
			err := cmd.Run(ctx, kctx, flags)
			if err == nil {
				t.Fatalf("expected invalid --index error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), "invalid --index") {
				t.Fatalf("expected 'invalid --index' error, got: %v", err)
			}
		})
	}
}
